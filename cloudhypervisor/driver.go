// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cloudhypervisor

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/ccheshirecat/nomad-driver-ch/cloudinit"
	domain "github.com/ccheshirecat/nomad-driver-ch/internal/shared"
)

const (
	// Cloud Hypervisor states mapped to our domain states
	CHStateRunning  = "running"
	CHStateShutdown = "shutdown"
	CHStateShutoff  = "shutoff"
	CHStateCrashed  = "crashed"
	CHStateUnknown  = "unknown"

	// Default timeouts and intervals
	defaultShutdownTimeout = 30 * time.Second
	defaultStartupTimeout  = 60 * time.Second
)

// VMProcess represents a running VM process with its metadata
type VMProcess struct {
	Name         string
	Pid          int
	APISocket    string
	LogFile      string
	WorkDir      string
	TapName      string
	MAC          string
	IP           string
	VirtiofsdPIDs []int
	Config       *VMConfig
	StartedAt    time.Time
}

// Driver implements the Virtualizer interface for Cloud Hypervisor
type Driver struct {
	logger        hclog.Logger
	config        *domain.CloudHypervisor
	networkConfig *domain.Network
	dataDir       string

	// Registry of running VMs
	mu        sync.RWMutex
	processes map[string]*VMProcess

	// HTTP client for Unix socket communication
	httpClient *http.Client

	// Cloud-init controller
	ci CloudInit

	// IP allocation state
	allocatedIPs map[string]bool // IP -> allocated
	ipCounter    int
}

// CloudInit interface for generating cloud-init ISOs
type CloudInit interface {
	Apply(ci *cloudinit.Config, path string) error
}

// VMConfig represents the JSON structure for CH vm.create API
type VMConfig struct {
	CPUs     CPUConfig      `json:"cpus"`
	Memory   MemoryConfig   `json:"memory"`
	Payload  *PayloadConfig `json:"payload,omitempty"`
	Disks    []DiskConfig   `json:"disks,omitempty"`
	Net      []NetConfig    `json:"net,omitempty"`
	RNG      *RNGConfig     `json:"rng,omitempty"`
	Vsock    *VsockConfig   `json:"vsock,omitempty"`
	FS       []FSConfig     `json:"fs,omitempty"`
	Platform *PlatformConfig `json:"platform,omitempty"`
	Devices  []DeviceConfig `json:"devices,omitempty"`
	Console  ConsoleConfig  `json:"console"`
	Serial   SerialConfig   `json:"serial"`
}

type CPUConfig struct {
	BootVCPUs uint     `json:"boot_vcpus"`
	MaxVCPUs  uint     `json:"max_vcpus"`
	Features  []string `json:"features,omitempty"`
}

type MemoryConfig struct {
	Size           string `json:"size"`
	Shared         bool   `json:"shared,omitempty"`
	Hugepages      bool   `json:"hugepages,omitempty"`
	HotplugMethod  string `json:"hotplug_method,omitempty"`
	HotplugSize    string `json:"hotplug_size,omitempty"`
}

type PayloadConfig struct {
	Kernel   string `json:"kernel"`
	Cmdline  string `json:"cmdline"`
	Initramfs string `json:"initramfs,omitempty"`
}

type DiskConfig struct {
	Path     string `json:"path"`
	Readonly bool   `json:"readonly,omitempty"`
	Serial   string `json:"serial,omitempty"`
}

type NetConfig struct {
	Tap string `json:"tap"`
	MAC string `json:"mac"`
	IP  string `json:"ip,omitempty"`
	Mask string `json:"mask,omitempty"`
}

type RNGConfig struct {
	Src string `json:"src"`
}

type VsockConfig struct {
	CID    uint   `json:"cid"`
	Socket string `json:"socket"`
}

type FSConfig struct {
	Tag       string `json:"tag"`
	Socket    string `json:"socket"`
	NumQueues uint   `json:"num_queues,omitempty"`
	QueueSize uint   `json:"queue_size,omitempty"`
}

type PlatformConfig struct {
	NumPCISegments    uint   `json:"num_pci_segments,omitempty"`
	IOMMUSegments     []uint `json:"iommu_segments,omitempty"`
	IOMMUAddressWidth uint   `json:"iommu_address_width,omitempty"`
}

type DeviceConfig struct {
	Path        string `json:"path"`
	ID          string `json:"id,omitempty"`
	IOMMU       bool   `json:"iommu,omitempty"`
	PCISegment  uint   `json:"pci_segment,omitempty"`
}

type ConsoleConfig struct {
	Mode string `json:"mode"`
}

type SerialConfig struct {
	Mode string `json:"mode"`
	File string `json:"file,omitempty"`
}

// VMInfo represents the response from CH vm.info API
type VMInfo struct {
	State  string `json:"state"`
	Memory struct {
		ActualSize   uint64 `json:"actual_size"`
		LastUpdate   uint64 `json:"last_update_ts"`
	} `json:"memory"`
	Balloons []interface{} `json:"balloons"`
	Block    []interface{} `json:"block"`
	Net      []interface{} `json:"net"`
}

// New creates a new Cloud Hypervisor driver
func New(ctx context.Context, logger hclog.Logger, config *domain.CloudHypervisor, netConfig *domain.Network, dataDir string) *Driver {
	d := &Driver{
		logger:        logger.Named("cloud-hypervisor"),
		config:        config,
		networkConfig: netConfig,
		dataDir:       dataDir,
		processes:     make(map[string]*VMProcess),
		allocatedIPs:  make(map[string]bool),
		ipCounter:     100, // Start from .100
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					// This will be overridden per request with the actual socket path
					return nil, fmt.Errorf("socket path not set")
				},
			},
			Timeout: 30 * time.Second,
		},
	}

	go d.monitorCtx(ctx)
	return d
}

// monitorCtx handles context cancellation cleanup
func (d *Driver) monitorCtx(ctx context.Context) {
	<-ctx.Done()
	d.logger.Info("shutting down cloud hypervisor driver")

	d.mu.Lock()
	defer d.mu.Unlock()

	// Cleanup all running processes
	for name, proc := range d.processes {
		d.logger.Warn("forcefully stopping VM on shutdown", "vm", name)
		d.cleanupProcess(proc)
	}
}

// Start validates the Cloud Hypervisor installation and initializes the driver
func (d *Driver) Start(dataDir string) error {
	if dataDir != "" {
		d.dataDir = dataDir
	}

	// Validate CH binaries exist and are executable
	if err := d.validateBinaries(); err != nil {
		return fmt.Errorf("cloud hypervisor binary validation failed: %w", err)
	}

	// Initialize cloud-init controller
	ci, err := cloudinit.NewController(d.logger.Named("cloud-init"))
	if err != nil {
		return fmt.Errorf("failed to create cloud-init controller: %w", err)
	}
	d.ci = ci

	// Ensure data directory exists
	if err := os.MkdirAll(d.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	d.logger.Info("cloud hypervisor driver started successfully",
		"data_dir", d.dataDir,
		"ch_binary", d.config.Bin)

	return nil
}

// validateBinaries checks that required binaries are available
func (d *Driver) validateBinaries() error {
	binaries := map[string]string{
		"cloud-hypervisor": d.config.Bin,
		"ch-remote":        d.config.RemoteBin,
	}

	// virtiofsd is optional
	if d.config.VirtiofsdBin != "" {
		binaries["virtiofsd"] = d.config.VirtiofsdBin
	}

	for name, path := range binaries {
		if path == "" {
			continue // Optional binary
		}

		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("%s binary not found at %s: %w", name, path, err)
		}

		// Check if executable
		if err := exec.Command(path, "--version").Run(); err != nil {
			d.logger.Warn("binary version check failed", "binary", name, "path", path, "error", err)
		}
	}

	return nil
}

// CreateDomain creates and starts a new VM
func (d *Driver) CreateDomain(config *domain.Config) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Check if VM already exists
	if _, exists := d.processes[config.Name]; exists {
		return fmt.Errorf("VM %s already exists", config.Name)
	}

	d.logger.Info("creating VM", "name", config.Name)

	// Create working directory for this VM
	workDir := filepath.Join(d.dataDir, config.Name)
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return fmt.Errorf("failed to create work directory: %w", err)
	}

	// Build VM process info
	proc := &VMProcess{
		Name:      config.Name,
		WorkDir:   workDir,
		APISocket: filepath.Join(workDir, "api.sock"),
		LogFile:   filepath.Join(workDir, "vmm.log"),
		StartedAt: time.Now(),
	}

	// Allocate IP address
	ip, err := d.allocateIP()
	if err != nil {
		return fmt.Errorf("failed to allocate IP: %w", err)
	}
	proc.IP = ip

	// Generate MAC address deterministically
	proc.MAC = d.generateMAC(config.Name)
	proc.TapName = d.networkConfig.TAPPrefix + config.Name

	// Create cloud-init ISO
	if err := d.createCloudInit(config, proc, workDir); err != nil {
		d.deallocateIP(ip)
		return fmt.Errorf("failed to create cloud-init: %w", err)
	}

	// Setup networking (create TAP interface)
	if err := d.setupNetworking(proc); err != nil {
		d.deallocateIP(ip)
		return fmt.Errorf("failed to setup networking: %w", err)
	}

	// Start virtiofsd processes for mounts
	if err := d.startVirtiofsd(config, proc); err != nil {
		d.deallocateIP(ip)
		d.cleanupNetworking(proc)
		return fmt.Errorf("failed to start virtiofsd: %w", err)
	}

	// Build CH VM configuration
	vmConfig, err := d.buildVMConfig(config, proc)
	if err != nil {
		d.deallocateIP(ip)
		d.cleanupNetworking(proc)
		d.stopVirtiofsd(proc)
		return fmt.Errorf("failed to build VM config: %w", err)
	}
	proc.Config = vmConfig

	// Start Cloud Hypervisor process
	if err := d.startCHProcess(proc); err != nil {
		d.deallocateIP(ip)
		d.cleanupNetworking(proc)
		d.stopVirtiofsd(proc)
		return fmt.Errorf("failed to start CH process: %w", err)
	}

	// Create and boot VM via REST API
	if err := d.createAndBootVM(proc); err != nil {
		d.cleanupProcess(proc)
		d.deallocateIP(ip)
		return fmt.Errorf("failed to create/boot VM: %w", err)
	}

	// Register the process
	d.processes[config.Name] = proc

	d.logger.Info("VM created successfully",
		"name", config.Name,
		"ip", proc.IP,
		"mac", proc.MAC,
		"tap", proc.TapName)

	return nil
}

// StopDomain gracefully stops a VM
func (d *Driver) StopDomain(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	proc, exists := d.processes[name]
	if !exists {
		return fmt.Errorf("VM %s not found", name)
	}

	d.logger.Info("stopping VM", "name", name)

	// Try graceful shutdown via REST API first
	if err := d.shutdownVM(proc); err != nil {
		d.logger.Warn("graceful shutdown failed, forcing stop", "vm", name, "error", err)
		// Force kill the process
		if proc.Pid > 0 {
			if process, err := os.FindProcess(proc.Pid); err == nil {
				process.Kill()
			}
		}
	}

	return nil
}

// DestroyDomain stops and removes a VM completely
func (d *Driver) DestroyDomain(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	proc, exists := d.processes[name]
	if !exists {
		return fmt.Errorf("VM %s not found", name)
	}

	d.logger.Info("destroying VM", "name", name)

	// Stop the VM first
	d.shutdownVM(proc)

	// Cleanup everything
	d.cleanupProcess(proc)

	// Deallocate IP
	d.deallocateIP(proc.IP)

	// Remove from registry
	delete(d.processes, name)

	d.logger.Info("VM destroyed", "name", name)
	return nil
}

// GetInfo returns information about the Cloud Hypervisor host
func (d *Driver) GetInfo() (domain.VirtualizerInfo, error) {
	info := domain.VirtualizerInfo{
		Model: "cloud-hypervisor",
	}

	// Get CH version
	if version, err := d.getCHVersion(); err == nil {
		if v, err := strconv.ParseUint(version, 10, 32); err == nil {
			info.EmulatorVersion = uint32(v)
		}
	}

	// Count running VMs
	d.mu.RLock()
	info.RunningDomains = uint(len(d.processes))
	d.mu.RUnlock()

	// TODO: Get actual host memory/CPU info if needed
	// For now, leave as defaults (0 values)

	return info, nil
}

// getCHVersion extracts version from cloud-hypervisor --version
func (d *Driver) getCHVersion() (string, error) {
	cmd := exec.Command(d.config.Bin, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Parse "cloud-hypervisor v48.0.0" -> "48"
	versionStr := strings.TrimSpace(string(output))
	parts := strings.Fields(versionStr)
	if len(parts) >= 2 {
		version := strings.TrimPrefix(parts[1], "v")
		// Extract major version number
		if dotIndex := strings.Index(version, "."); dotIndex > 0 {
			return version[:dotIndex], nil
		}
		return version, nil
	}

	return "0", nil
}

// GetNetworkInterfaces returns network interface information for a VM
func (d *Driver) GetNetworkInterfaces(name string) ([]domain.NetworkInterface, error) {
	d.mu.RLock()
	proc, exists := d.processes[name]
	d.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("VM %s not found", name)
	}

	// Return interface info from our stored configuration
	interfaces := []domain.NetworkInterface{
		{
			NetworkName: d.networkConfig.Bridge,
			DeviceName:  proc.TapName,
			MAC:         proc.MAC,
			Model:       "virtio",
			Driver:      "virtio-net",
		},
	}

	// Parse IP address if available
	if proc.IP != "" {
		if addr, err := netip.ParseAddr(proc.IP); err == nil {
			interfaces[0].Addrs = []netip.Addr{addr}
		}
	}

	return interfaces, nil
}

// GetDomain returns information about a specific VM
func (d *Driver) GetDomain(name string) (*domain.Info, error) {
	d.mu.RLock()
	proc, exists := d.processes[name]
	d.mu.RUnlock()

	if !exists {
		return nil, nil // VM not found
	}

	// Query VM info via REST API
	info, err := d.getVMInfo(proc)
	if err != nil {
		// If REST API fails, check if process is still running
		if proc.Pid > 0 {
			if process, err := os.FindProcess(proc.Pid); err == nil {
				if err := process.Signal(syscall.Signal(0)); err == nil {
					// Process exists, assume running
					return &domain.Info{
						State: CHStateRunning,
					}, nil
				}
			}
		}
		// Process not found, VM is stopped
		return &domain.Info{
			State: CHStateShutoff,
		}, nil
	}

	// Map CH state to domain state
	domainState := mapCHState(info.State)

	return &domain.Info{
		State:     domainState,
		Memory:    info.Memory.ActualSize,
		MaxMemory: info.Memory.ActualSize, // CH doesn't distinguish
		CPUTime:   0, // TODO: extract if available
	}, nil
}

// Helper functions below...

func (d *Driver) allocateIP() (string, error) {
	// Simple IP allocation from pool
	// TODO: Make this more robust with persistence
	for i := 100; i <= 200; i++ {
		ip := fmt.Sprintf("194.31.143.%d", i)
		if !d.allocatedIPs[ip] {
			d.allocatedIPs[ip] = true
			return ip, nil
		}
	}
	return "", fmt.Errorf("no available IPs in pool")
}

func (d *Driver) deallocateIP(ip string) {
	delete(d.allocatedIPs, ip)
}

func (d *Driver) generateMAC(vmName string) string {
	// Generate deterministic MAC address based on VM name
	// Use a simple hash-based approach
	hash := 0
	for _, c := range vmName {
		hash = hash*31 + int(c)
	}

	// Generate MAC with 52:54:00 prefix (QEMU/KVM range)
	return fmt.Sprintf("52:54:00:%02x:%02x:%02x",
		byte(hash>>16), byte(hash>>8), byte(hash))
}

func mapCHState(chState string) string {
	switch strings.ToLower(chState) {
	case "running":
		return CHStateRunning
	case "shutdown":
		return CHStateShutdown
	case "shutoff":
		return CHStateShutoff
	case "crashed":
		return CHStateCrashed
	default:
		return CHStateUnknown
	}
}

func parseIPAddr(ipStr string) (net.IP, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ipStr)
	}
	return ip, nil
}

// Additional methods will be implemented in separate files to keep this manageable
// These include:
// - createCloudInit()
// - setupNetworking()
// - startVirtiofsd()
// - buildVMConfig()
// - startCHProcess()
// - createAndBootVM()
// - shutdownVM()
// - getVMInfo()
// - cleanupProcess()
// - etc.
// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cloudhypervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/ccheshirecat/nomad-driver-ch/cloudinit"
	domain "github.com/ccheshirecat/nomad-driver-ch/internal/shared"
)

// createCloudInit generates cloud-init ISO for VM
func (d *Driver) createCloudInit(config *domain.Config, proc *VMProcess, workDir string) error {
	// Generate cloud-init commands for virtio-fs mounts
	bootCMDs := []string{}

	// Add mount commands for each mount (replacing 9p logic)
	for _, mount := range config.Mounts {
		bootCMDs = append(bootCMDs,
			fmt.Sprintf("mkdir -p %s", mount.Destination),
			fmt.Sprintf("mount -t virtiofs %s %s", mount.Tag, mount.Destination),
		)
	}

	// Add any additional boot commands from config
	bootCMDs = append(bootCMDs, config.BOOTCMDs...)

	// Create network configuration for static IP
	networkConfig := fmt.Sprintf(`
network:
  version: 2
  ethernets:
    eth0:
      addresses:
        - %s/24
      gateway4: %s
      nameservers:
        addresses:
          - 8.8.8.8
          - 1.1.1.1
`, proc.IP, d.networkConfig.Gateway)

	// Build cloud-init config
	ciConfig := &cloudinit.Config{
		MetaData: cloudinit.MetaData{
			InstanceID:    config.Name,
			LocalHostname: config.HostName,
		},
		VendorData: cloudinit.VendorData{
			Password:  config.Password,
			SSHKey:    config.SSHKey,
			BootCMD:   bootCMDs,
			RunCMD:    config.CMDs,
			Files:     convertFiles(config.Files),
		},
		UserData: config.CIUserData,
	}

	// If no custom user data, add network config
	if config.CIUserData == "" {
		ciConfig.UserData = fmt.Sprintf("#cloud-config\n%s", networkConfig)
	}

	// Create ISO
	isoPath := filepath.Join(workDir, config.Name+".iso")
	if err := d.ci.Apply(ciConfig, isoPath); err != nil {
		return fmt.Errorf("failed to create cloud-init ISO: %w", err)
	}

	d.logger.Debug("cloud-init ISO created", "path", isoPath)
	return nil
}

// convertFiles converts domain.File to cloudinit.File
func convertFiles(domainFiles []domain.File) []cloudinit.File {
	files := make([]cloudinit.File, len(domainFiles))
	for i, df := range domainFiles {
		files[i] = cloudinit.File{
			Path:        df.Path,
			Content:     df.Content,
			Permissions: df.Permissions,
			Encoding:    df.Encoding,
			Owner:       df.Owner,
			Group:       df.Group,
		}
	}
	return files
}

// setupNetworking creates TAP interface and attaches to bridge
func (d *Driver) setupNetworking(proc *VMProcess) error {
	// Create TAP interface
	cmd := exec.Command("ip", "tuntap", "add", "dev", proc.TapName, "mode", "tap")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create tap interface: %w", err)
	}

	// Set TAP interface up
	cmd = exec.Command("ip", "link", "set", "dev", proc.TapName, "up")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to bring up tap interface: %w", err)
	}

	// Add TAP to bridge
	cmd = exec.Command("ip", "link", "set", "dev", proc.TapName, "master", d.networkConfig.Bridge)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add tap to bridge: %w", err)
	}

	d.logger.Debug("networking setup complete",
		"tap", proc.TapName,
		"bridge", d.networkConfig.Bridge,
		"ip", proc.IP)

	return nil
}

// cleanupNetworking removes TAP interface
func (d *Driver) cleanupNetworking(proc *VMProcess) {
	if proc.TapName != "" {
		cmd := exec.Command("ip", "link", "delete", "dev", proc.TapName)
		if err := cmd.Run(); err != nil {
			d.logger.Warn("failed to cleanup tap interface", "tap", proc.TapName, "error", err)
		}
	}
}

// startVirtiofsd starts virtiofsd processes for each mount
func (d *Driver) startVirtiofsd(config *domain.Config, proc *VMProcess) error {
	if d.config.VirtiofsdBin == "" {
		// virtiofsd not available, skip mounts
		d.logger.Debug("virtiofsd not configured, skipping mounts")
		return nil
	}

	for _, mount := range config.Mounts {
		socketPath := filepath.Join(proc.WorkDir, mount.Tag+".sock")

		// Remove existing socket
		os.Remove(socketPath)

		// Start virtiofsd
		cmd := exec.Command(d.config.VirtiofsdBin,
			"--socket-path", socketPath,
			"--shared-dir", mount.Source,
			"--cache", "auto",
			"--sandbox", "chroot",
		)

		// Start the process
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("failed to start virtiofsd for %s: %w", mount.Tag, err)
		}

		proc.VirtiofsdPIDs = append(proc.VirtiofsdPIDs, cmd.Process.Pid)

		d.logger.Debug("virtiofsd started",
			"tag", mount.Tag,
			"socket", socketPath,
			"source", mount.Source,
			"pid", cmd.Process.Pid)
	}

	return nil
}

// stopVirtiofsd stops all virtiofsd processes
func (d *Driver) stopVirtiofsd(proc *VMProcess) {
	for _, pid := range proc.VirtiofsdPIDs {
		if process, err := os.FindProcess(pid); err == nil {
			process.Kill()
			d.logger.Debug("stopped virtiofsd", "pid", pid)
		}
	}
	proc.VirtiofsdPIDs = nil
}

// buildVMConfig constructs the VM configuration for CH API
func (d *Driver) buildVMConfig(config *domain.Config, proc *VMProcess) (*VMConfig, error) {
	vmConfig := &VMConfig{
		CPUs: CPUConfig{
			BootVCPUs: config.CPUs,
			MaxVCPUs:  config.CPUs, // TODO: support max_vcpus from task config
		},
		Memory: MemoryConfig{
			Size:   fmt.Sprintf("%dM", config.Memory),
			Shared: true, // Required for virtio-fs
		},
		Console: ConsoleConfig{Mode: "null"}, // Disable console
		Serial:  SerialConfig{Mode: "file", File: filepath.Join(proc.WorkDir, "serial.log")},
	}

	// Set kernel/initramfs/cmdline
	if config.BaseImage != "" {
		// Check if we have kernel specified
		kernel := d.config.DefaultKernel
		initramfs := d.config.DefaultInitramfs
		cmdline := "console=hvc0 root=/dev/vda1 rw"

		// TODO: Extract kernel/initramfs/cmdline from task config when available

		if kernel != "" {
			vmConfig.Payload = &PayloadConfig{
				Kernel:   kernel,
				Cmdline:  cmdline,
			}
			if initramfs != "" {
				vmConfig.Payload.Initramfs = initramfs
			}
		}
	}

	// Add disks
	vmConfig.Disks = []DiskConfig{
		{
			Path:     config.BaseImage,
			Readonly: false,
		},
	}

	// Add cloud-init ISO as readonly disk
	isoPath := filepath.Join(proc.WorkDir, config.Name+".iso")
	vmConfig.Disks = append(vmConfig.Disks, DiskConfig{
		Path:     isoPath,
		Readonly: true,
	})

	// Add network interface
	vmConfig.Net = []NetConfig{
		{
			Tap: proc.TapName,
			MAC: proc.MAC,
		},
	}

	// Add RNG
	vmConfig.RNG = &RNGConfig{
		Src: "/dev/urandom",
	}

	// Add virtio-fs mounts
	for _, mount := range config.Mounts {
		socketPath := filepath.Join(proc.WorkDir, mount.Tag+".sock")
		vmConfig.FS = append(vmConfig.FS, FSConfig{
			Tag:       mount.Tag,
			Socket:    socketPath,
			NumQueues: 1, // TODO: make configurable
			QueueSize: 1024,
		})
	}

	// TODO: Add support for additional devices, vsock, etc. from task config

	return vmConfig, nil
}

// startCHProcess starts the Cloud Hypervisor daemon process
func (d *Driver) startCHProcess(proc *VMProcess) error {
	// Prepare log file
	logFile, err := os.Create(proc.LogFile)
	if err != nil {
		return fmt.Errorf("failed to create log file: %w", err)
	}
	defer logFile.Close()

	// Start CH process
	args := []string{
		"--api-socket", proc.APISocket,
	}

	// Add logging
	if d.config.LogFile != "" {
		args = append(args, "--log-file", proc.LogFile)
	}

	// Add seccomp
	if d.config.Seccomp != "" {
		args = append(args, "--seccomp", d.config.Seccomp)
	}

	cmd := exec.Command(d.config.Bin, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start cloud-hypervisor: %w", err)
	}

	proc.Pid = cmd.Process.Pid

	// Wait for API socket to become available
	if err := d.waitForAPISocket(proc.APISocket, defaultStartupTimeout); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("CH API socket not ready: %w", err)
	}

	d.logger.Debug("cloud-hypervisor process started",
		"pid", proc.Pid,
		"api_socket", proc.APISocket)

	return nil
}

// waitForAPISocket waits for CH API socket to become available
func (d *Driver) waitForAPISocket(socketPath string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if _, err := os.Stat(socketPath); err == nil {
			// Socket exists, try to connect
			conn, err := net.Dial("unix", socketPath)
			if err == nil {
				conn.Close()
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for API socket")
}

// createAndBootVM creates and boots the VM via REST API
func (d *Driver) createAndBootVM(proc *VMProcess) error {
	// Create VM
	if err := d.vmCreate(proc); err != nil {
		return fmt.Errorf("failed to create VM: %w", err)
	}

	// Boot VM
	if err := d.vmBoot(proc); err != nil {
		return fmt.Errorf("failed to boot VM: %w", err)
	}

	// Wait for VM to be running
	if err := d.waitForVMState(proc, CHStateRunning, defaultStartupTimeout); err != nil {
		return fmt.Errorf("VM failed to reach running state: %w", err)
	}

	return nil
}

// vmCreate calls CH vm.create API
func (d *Driver) vmCreate(proc *VMProcess) error {
	body, err := json.Marshal(proc.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal VM config: %w", err)
	}

	resp, err := d.httpRequest(proc.APISocket, "PUT", "/api/v1/vm.create", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("VM create failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// vmBoot calls CH vm.boot API
func (d *Driver) vmBoot(proc *VMProcess) error {
	resp, err := d.httpRequest(proc.APISocket, "PUT", "/api/v1/vm.boot", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("VM boot failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// shutdownVM calls CH vm.shutdown API
func (d *Driver) shutdownVM(proc *VMProcess) error {
	resp, err := d.httpRequest(proc.APISocket, "PUT", "/api/v1/vm.shutdown", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("VM shutdown failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Wait for shutdown with timeout
	return d.waitForVMState(proc, CHStateShutoff, defaultShutdownTimeout)
}

// getVMInfo calls CH vm.info API
func (d *Driver) getVMInfo(proc *VMProcess) (*VMInfo, error) {
	resp, err := d.httpRequest(proc.APISocket, "GET", "/api/v1/vm.info", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("VM info failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var info VMInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode VM info: %w", err)
	}

	return &info, nil
}

// waitForVMState waits for VM to reach the specified state
func (d *Driver) waitForVMState(proc *VMProcess, targetState string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		info, err := d.getVMInfo(proc)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		if mapCHState(info.State) == targetState {
			return nil
		}

		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("timeout waiting for VM state %s", targetState)
}

// httpRequest performs HTTP request to CH API via Unix socket
func (d *Driver) httpRequest(socketPath, method, path string, body []byte) (*http.Response, error) {
	// Create a custom transport for this specific socket
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, "http://localhost"+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	return client.Do(req)
}

// cleanupProcess cleans up all resources associated with a VM process
func (d *Driver) cleanupProcess(proc *VMProcess) {
	// Stop virtiofsd processes
	d.stopVirtiofsd(proc)

	// Kill CH process
	if proc.Pid > 0 {
		if process, err := os.FindProcess(proc.Pid); err == nil {
			process.Kill()
		}
	}

	// Cleanup networking
	d.cleanupNetworking(proc)

	// Remove working directory
	if proc.WorkDir != "" {
		os.RemoveAll(proc.WorkDir)
	}

	d.logger.Debug("process cleanup complete", "vm", proc.Name)
}
// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	domain "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/libvirt"
	virtnet "github.com/hashicorp/nomad-driver-virt/libvirt/net"
	"github.com/hashicorp/nomad-driver-virt/virt/image_tools"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/client/lib/idset"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/client/lib/numalib/hw"
	"github.com/hashicorp/nomad/client/taskenv"
	"github.com/hashicorp/nomad/drivers/shared/eventer"
	"github.com/hashicorp/nomad/plugins/base"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

const (
	pluginName = "nomad-driver-virt"

	// pluginVersion allows the client to identify and use newer versions of
	// an installed plugin
	pluginVersion = "v0.1.0"

	// fingerprintPeriod is the interval at which the plugin will send
	// fingerprint responses
	fingerprintPeriod = 30 * time.Second

	// taskHandleVersion is the version of task handle which this plugin sets
	// and understands how to decode
	// this is used to allow modification and migration of the task schema
	// used by the plugin
	taskHandleVersion = 1

	defaultDataDir     = "/var/lib/virt"
	dataDirPermissions = 777

	envVariblesFilePath        = "/etc/profile.d/virt.sh" //Only valid for linux OS
	envVariblesFilePermissions = "777"
)

var (
	// pluginInfo describes the plugin
	pluginInfo = &base.PluginInfoResponse{
		Type:              base.PluginTypeDriver,
		PluginApiVersions: []string{drivers.ApiVersion010},
		PluginVersion:     pluginVersion,
		Name:              pluginName,
	}

	ErrExistingTaks    = errors.New("task is already running")
	ErrStartingLibvirt = errors.New("unable to start libvirt")
	ErrImageNotFound   = errors.New("disk image not found at path")
	ErrTaskCrashed     = errors.New("task has crashed")
)

// TaskState is the runtime state which is encoded in the handle returned to
// Nomad client.
// This information is needed to rebuild the task state and handler during
// recovery.
type TaskState struct {
	TaskConfig *drivers.TaskConfig
	StartedAt  time.Time

	// NetTeardown is the specification used to delete all the network
	// configuration associated to a VM.
	NetTeardown *net.TeardownSpec
}

// Net is the interface that defines the virtualization network sub-system. It
// should be the only link from the main driver and compute functionality, into
// the network. This helps encapsulate the logic making future development
// easier, even allowing for this code to be moved into its own application if
// desired.
type Net interface {

	// Fingerprint interrogates the host system and populates the attribute
	// mapping with relevant network information. Any errors performing this
	// should be logged by the implementor, but not considered terminal, which
	// explains the lack of error response. Each entry should use
	// FingerprintAttributeKeyPrefix as a base.
	Fingerprint(map[string]*structs.Attribute)

	// Init performs any initialization work needed by the network sub-system
	// prior to being used by the driver. This will be called when the plugin
	// is set up by Nomad and should be expected to run multiple times during
	// a Nomad client's lifecycle. It should therefore be idempotent. Any error
	// returned is considered fatal to the plugin.
	Init() error

	// VMStartedBuild performs any network configuration required once the
	// driver has successfully started a VM. Any error returned will be
	// considered terminal to the start of the VM and therefore halt any
	// further progress and result in the task being restarted.
	VMStartedBuild(*net.VMStartedBuildRequest) (*net.VMStartedBuildResponse, error)

	// VMTerminatedTeardown performs all the network teardown required to clean
	// the host and any systems of configuration specific to the task. If an
	// error is encountered, Nomad will retry the stop/kill process, so all
	// implementations must be able to support this and not enter death spirals
	// when an error occurs.
	VMTerminatedTeardown(*net.VMTerminatedTeardownRequest) (*net.VMTerminatedTeardownResponse, error)
}

type Virtualizer interface {
	Start(string) error
	CreateDomain(config *domain.Config) error
	StopDomain(name string) error
	DestroyDomain(name string) error
	GetInfo() (domain.VirtualizerInfo, error)
}

type DomainGetter interface {
	GetDomain(name string) (*domain.Info, error)
}

type ImageHandler interface {
	GetImageFormat(basePath string) (string, error)
	CreateThinCopy(basePath string, destination string, sizeM int64) error
}

type VirtDriverPlugin struct {
	eventer        *eventer.Eventer
	virtualizer    Virtualizer
	taskGetter     DomainGetter
	config         *Config
	nomadConfig    *base.ClientDriverConfig
	tasks          *taskStore
	signalShutdown context.CancelFunc
	logger         hclog.Logger
	dataDir        string
	// networkController is the backend controller interface for the network
	// subsystem.
	networkController Net
	// networkInit indicates whether the network subsystem has had its init
	// function called. While the function should be idempotent, this helps
	// avoid unnecessary calls and work.
	networkInit  atomic.Bool
	imageHandler ImageHandler
}

// NewPlugin returns a new driver plugin
func NewPlugin(logger hclog.Logger) drivers.DriverPlugin {
	ctx, cancel := context.WithCancel(context.Background())
	logger = logger.Named(pluginName)

	v := libvirt.New(ctx, logger)

	// Should we check if extentions and kernel modules are there?
	// grep -E 'svm|vmx' /proc/cpuinfo
	// lsmod | grep kvm -> kvm_intel, kvm_amd, nvme-tcp
	return &VirtDriverPlugin{
		eventer:           eventer.NewEventer(ctx, logger),
		config:            &Config{},
		tasks:             newTaskStore(),
		signalShutdown:    cancel,
		logger:            logger,
		networkInit:       atomic.Bool{},
		imageHandler:      image_tools.NewHandler(logger),
		virtualizer:       v,
		taskGetter:        v,
		networkController: virtnet.NewController(logger, v),
	}
}

// PluginInfo returns information describing the plugin.
func (d *VirtDriverPlugin) PluginInfo() (*base.PluginInfoResponse, error) {
	return pluginInfo, nil
}

// ConfigSchema returns the plugin configuration schema.
func (d *VirtDriverPlugin) ConfigSchema() (*hclspec.Spec, error) {
	return configSpec, nil
}

// SetConfig is called by the client to pass the configuration for the plugin.
func (d *VirtDriverPlugin) SetConfig(cfg *base.Config) error {
	var config Config
	if len(cfg.PluginConfig) != 0 {
		if err := base.MsgPackDecode(cfg.PluginConfig, &config); err != nil {
			return err
		}
	}

	// Save the configuration to the plugin
	d.config = &config

	// Save the Nomad agent configuration
	if cfg.AgentConfig != nil {
		d.nomadConfig = cfg.AgentConfig.Driver
	}

	if d.config.DataDir != "" {
		d.dataDir = config.DataDir
	} else {
		d.dataDir = defaultDataDir
	}

	err := createDataDirectory(d.dataDir)
	if err != nil {
		return fmt.Errorf("virt: unable to create data dir: %w", err)
	}

	err = d.virtualizer.Start(d.dataDir)
	if err != nil {
		return err
	}

	if !d.networkInit.Load() {
		if err := d.networkController.Init(); err != nil {
			return fmt.Errorf("virt: failed to init network controller: %w", err)
		} else {
			d.networkInit.Store(true)
		}
	}

	return nil
}

// TaskConfigSchema returns the HCL schema for the configuration of a task.
func (d *VirtDriverPlugin) TaskConfigSchema() (*hclspec.Spec, error) {
	return taskConfigSpec, nil
}

// Capabilities returns the features supported by the driver.
func (d *VirtDriverPlugin) Capabilities() (*drivers.Capabilities, error) {
	return capabilities, nil
}

// Fingerprint returns a channel that will be used to send health information
// and other driver specific node attributes.
func (d *VirtDriverPlugin) Fingerprint(ctx context.Context) (<-chan *drivers.Fingerprint, error) {
	ch := make(chan *drivers.Fingerprint)

	go d.handleFingerprint(ctx, ch)

	return ch, nil
}

// handleFingerprint manages the channel and the flow of fingerprint data.
func (d *VirtDriverPlugin) handleFingerprint(ctx context.Context, ch chan<- *drivers.Fingerprint) {
	defer close(ch)

	// Nomad expects the initial fingerprint to be sent immediately
	ch <- d.buildFingerprint()

	ticker := time.NewTicker(fingerprintPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ch <- d.buildFingerprint()
		}
	}
}

// buildFingerprint returns the driver's fingerprint data
func (d *VirtDriverPlugin) buildFingerprint() *drivers.Fingerprint {
	virtInfo, err := d.virtualizer.GetInfo()
	if err != nil {
		return &drivers.Fingerprint{
			Attributes:        map[string]*structs.Attribute{},
			Health:            drivers.HealthStateUndetected,
			HealthDescription: "",
		}
	}
	attrs := map[string]*structs.Attribute{}

	attrs["driver.virt"] = structs.NewBoolAttribute(true)
	attrs["driver.virt.libvirt.version"] = structs.NewIntAttribute(int64(virtInfo.LibvirtVersion), "")
	attrs["driver.virt.emulator.version"] = structs.NewIntAttribute(int64(virtInfo.EmulatorVersion), "")
	attrs["driver.virt.active"] = structs.NewIntAttribute(int64(virtInfo.RunningDomains), "")
	attrs["driver.virt.inactive"] = structs.NewIntAttribute(int64(virtInfo.InactiveDomains), "bytes")

	d.networkController.Fingerprint(attrs)

	fp := &drivers.Fingerprint{
		Attributes:        attrs,
		Health:            drivers.HealthStateHealthy,
		HealthDescription: drivers.DriverHealthy,
	}

	return fp
}

// WaitTask function is expected to return a channel that will send an *ExitResult when the task
// exits or close the channel when the context is canceled. It is also expected that calling
// WaitTask on an exited task will immediately send an *ExitResult on the returned channel.
// A call to WaitTask after StopTask is valid and should be handled.
// If WaitTask is called after DestroyTask, it should return drivers.ErrTaskNotFound as
// no task state should exist after DestroyTask is called.
func (d *VirtDriverPlugin) WaitTask(ctx context.Context, taskID string) (<-chan *drivers.ExitResult, error) {
	exitChannel := make(chan *drivers.ExitResult, 1)

	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return nil, drivers.ErrTaskNotFound
	}

	go func(ctx context.Context, handle *taskHandle, exitCh chan *drivers.ExitResult) {
		defer close(exitCh)
		d.logger.Info("monitoring task", "task_id", handle.name)

		handle.monitor(ctx, exitCh)

	}(ctx, handle, exitChannel)

	return exitChannel, nil
}

// StopTask function is expected to stop a running task by sending the given signal to it.
// If the task does not stop during the given timeout, the driver must forcefully kill the task.
// StopTask does not clean up resources of the task or remove it from the driver's internal state.
func (d *VirtDriverPlugin) StopTask(taskID string, timeout time.Duration, signal string) error {
	d.logger.Info("stopping task", "task_id", taskID)

	_, ok := d.tasks.Get(taskID)
	if !ok {
		d.logger.Warn("task to stop not found", "task_id", taskID)
		return nil
	}

	err := d.virtualizer.StopDomain(domainNameFromTaskID(taskID))
	if err != nil {
		return fmt.Errorf("virt: unable to stop task %s: %w", taskID, err)
	}

	return nil
}

// DestroyTask function cleans up and removes a task that has terminated.
// If force is set to true, the driver must destroy the task even if it is still running.
func (d *VirtDriverPlugin) DestroyTask(taskID string, force bool) error {
	d.logger.Info("destroying task", "task_id", taskID)

	handle, ok := d.tasks.Get(taskID)
	if !ok {
		d.logger.Warn("task to destroy not found", "task_id", taskID)
		return nil
	}

	taskName := domainNameFromTaskID(taskID)
	if handle.IsRunning() && !force {
		return errors.New("cannot destroy a running task")
	}

	err := d.virtualizer.DestroyDomain(taskName)
	if err != nil {
		return fmt.Errorf("virt: unable to destroy task %s: %w", taskID, err)
	}

	// Build our network request to send now that the VM has been destroyed.
	netTeardownReq := net.VMTerminatedTeardownRequest{
		TeardownSpec: handle.netTeardown,
	}
	if _, err := d.networkController.VMTerminatedTeardown(&netTeardownReq); err != nil {
		return fmt.Errorf("virt: failed to destroy task network: %w", err)
	}

	d.tasks.Delete(taskName)

	return nil
}

// InspectTask returns detailed status information for the referenced taskID.
func (d *VirtDriverPlugin) InspectTask(taskID string) (*drivers.TaskStatus, error) {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return nil, drivers.ErrTaskNotFound
	}

	return handle.TaskStatus(), nil
}

// TaskStats returns a channel which the driver should send stats to at the given interval.
func (d *VirtDriverPlugin) TaskStats(ctx context.Context, taskID string, interval time.Duration) (<-chan *drivers.TaskResourceUsage, error) {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return nil, drivers.ErrTaskNotFound
	}

	statsChannel := make(chan *drivers.TaskResourceUsage)

	go d.publishStats(ctx, interval, statsChannel, handle)

	return statsChannel, nil
}

func (d *VirtDriverPlugin) publishStats(ctx context.Context, interval time.Duration,
	sch chan<- *drivers.TaskResourceUsage, handle *taskHandle) {
	defer close(sch)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			stats, err := handle.GetStats()
			if err != nil {
				d.logger.Error("error while reading stats from the task", "task", handle.name, "error", err)
			}

			d.logger.Trace("publishing stats", "values", fmt.Sprintf("%+v", stats.ResourceUsage.MemoryStats))
			sch <- stats

		case <-ctx.Done():
			return
		}
	}
}

// TaskEvents returns a channel that the plugin can use to emit task related events.
func (d *VirtDriverPlugin) TaskEvents(ctx context.Context) (<-chan *drivers.TaskEvent, error) {
	return d.eventer.TaskEvents(ctx)
}

// SignalTask forwards a signal to a task.
// This is an optional capability, currently not supported by the virt driver.
func (d *VirtDriverPlugin) SignalTask(taskID string, signal string) error {
	return errors.New("This driver does not support signaling")
}

// ExecTask returns the result of executing the given command inside a task.
// This is an optional capability, currently not supported by the virt driver.
func (d *VirtDriverPlugin) ExecTask(taskID string, cmd []string, timeout time.Duration) (*drivers.ExecTaskResult, error) {
	return nil, errors.New("This driver does not support exec")
}

// domainNameFromTaskConfig creates a name to be used for the vms, using the
// last 8 chars of the taskName which should be unique per task and the task name
// to help the operator identify the vms.
//
// The struct of the task ID is "allocID/taskName/UniqueID".
func domainNameFromTaskID(taskID string) string {
	ids := strings.Split(taskID, "/")
	return strings.Join(ids[1:], "-")
}

func createDataDirectory(path string) error {
	err := os.MkdirAll(path, dataDirPermissions)
	if err != nil {
		return err
	}

	return nil
}

func createAllocFileMounts(task *drivers.TaskConfig) []domain.MountFileConfig {
	mounts := []domain.MountFileConfig{
		{
			Source:      task.TaskDir().SharedAllocDir,
			Tag:         "allocDir",
			Destination: task.Env[taskenv.AllocDir],
			ReadOnly:    true,
		},
		{
			Source:      task.TaskDir().LocalDir,
			Tag:         "localDir",
			Destination: task.Env[taskenv.TaskLocalDir],
			ReadOnly:    true,
		},
		{
			Source:      task.TaskDir().SecretsDir,
			Tag:         "secretsDir",
			Destination: task.Env[taskenv.SecretsDir],
			ReadOnly:    true,
		},
	}

	return mounts
}

// To create the alloc env vars, they are all writtent into a scripy in
// /etc/profile.d/virt.sh where the OS will take care of executing it at start.
func createEnvsFile(envs map[string]string) domain.File {
	con := []string{}

	for k, v := range envs {
		con = append(con, fmt.Sprintf("export %s=%s", k, v))
	}

	return domain.File{
		Encoding:    "b64",
		Path:        envVariblesFilePath,
		Permissions: envVariblesFilePermissions,
		Content:     base64.StdEncoding.EncodeToString([]byte(strings.Join(con, "\n\t"))),
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func addCMDsForMounts(mounts []domain.MountFileConfig) []string {
	cmds := []string{}
	for _, m := range mounts {
		c := []string{
			fmt.Sprintf("mkdir -p %s", m.Destination),
			fmt.Sprintf("mountpoint -q %s || mount -t 9p -o trans=virtio %s %s", m.Destination, m.Tag, m.Destination),
		}

		cmds = append(cmds, c...)
	}

	return cmds
}

func setUpTaskState(vmState string) drivers.TaskState {
	switch vmState {
	case libvirt.DomainRunning:
		return drivers.TaskStateRunning
	case libvirt.DomainShutdown, libvirt.DomainShutOff, libvirt.DomainCrashed:
		return drivers.TaskStateExited
	default:
		return drivers.TaskStateUnknown
	}
}

func buildHostname(taskName string) string {
	return fmt.Sprintf("nomad-%s", taskName)
}

// StartTask returns a task handle and a driver network if necessary.
func (d *VirtDriverPlugin) StartTask(cfg *drivers.TaskConfig) (*drivers.TaskHandle, *drivers.DriverNetwork, error) {
	if _, ok := d.tasks.Get(cfg.ID); ok {
		return nil, nil, fmt.Errorf("task with ID %q already started", cfg.ID)
	}

	var driverConfig TaskConfig
	if err := cfg.DecodeDriverConfig(&driverConfig); err != nil {
		return nil, nil, fmt.Errorf("virt: failed to decode driver config: %v", err)
	}

	d.logger.Error("starting task", "driver_cfg", hclog.Fmt("%+v\n", cfg.Resources))
	d.logger.Error("starting task", "driver_cfg", hclog.Fmt("%+v\n", cfg.Resources.LinuxResources))
	d.logger.Error("starting task", "driver_cfg", hclog.Fmt("%+v\n", cfg.Resources.NomadResources))

	taskName := domainNameFromTaskID(cfg.ID)

	d.logger.Info("starting task", "name", taskName)

	// The process to have directories mounted on the VM consists on two steps,
	// one is declaring them as backing storage in the VM and the second is to
	// create the directory inside the VM and executing the mount using 9P.
	// These commands are added here to execute at bootime.
	allocFSMounts := createAllocFileMounts(cfg)
	bootCMDs := addCMDsForMounts(allocFSMounts)

	var osVariant *domain.OSVariant
	if driverConfig.OS != nil {
		osVariant = &domain.OSVariant{
			Machine: driverConfig.OS.Machine,
			Arch:    driverConfig.OS.Arch,
		}
	}

	hostname := buildHostname(taskName)
	if driverConfig.Hostname != "" {
		hostname = driverConfig.Hostname
	}

	// The alloc directory and plugin data directory are assumed to be allowed
	// paths to load images from.
	allowedPaths := append(d.config.ImagePaths, d.dataDir, cfg.AllocDir)

	diskImagePath := driverConfig.ImagePath

	if !fileExists(diskImagePath) {

		// Assuming the image was downloaded using artifacts and will be placed
		// somewhere in the alloc's filesystem.
		diskImagePath = filepath.Join(cfg.TaskDir().Dir, diskImagePath)
		if !fileExists(diskImagePath) {
			return nil, nil, fmt.Errorf("virt: %s, %w", cfg.AllocID, ErrImageNotFound)
		}
	}

	diskFormat, err := d.imageHandler.GetImageFormat(diskImagePath)
	if err != nil {
		return nil, nil, fmt.Errorf("virt: unable to get disk format %s: %w", cfg.AllocID, err)
	}

	if driverConfig.UseThinCopy {
		copyPath := filepath.Join(d.dataDir, taskName+".img")
		d.logger.Info("creating thin copy at", "path", copyPath) // TODO: Put back at info

		if err := d.imageHandler.CreateThinCopy(diskImagePath, copyPath,
			cfg.Resources.NomadResources.Memory.MemoryMB); err != nil {
			return nil, nil, fmt.Errorf("virt: unable to create thin copy for %s: %w",
				taskName, err)
		}

		diskImagePath = copyPath
		diskFormat = "qcow2"
	}

	cpuSet := idset.Parse[hw.CoreID](cfg.Resources.LinuxResources.CpusetCpus)

	dc := &domain.Config{
		RemoveConfigFiles: true,
		Name:              taskName,
		Memory:            uint(cfg.Resources.NomadResources.Memory.MemoryMB),
		CPUs:              uint(cpuSet.Size()),
		CPUset:            cfg.Resources.LinuxResources.CpusetCpus,
		OsVariant:         osVariant,
		BaseImage:         diskImagePath,
		DiskFmt:           diskFormat,
		PrimaryDiskSize:   driverConfig.PrimaryDiskSize,
		HostName:          hostname,
		Mounts:            allocFSMounts,
		CMDs:              driverConfig.CMDs,
		BOOTCMDs:          bootCMDs,
		CIUserData:        driverConfig.UserData,
		Password:          driverConfig.DefaultUserPassword,
		SSHKey:            driverConfig.DefaultUserSSHKey,
		Files:             []domain.File{createEnvsFile(cfg.Env)},
		NetworkInterfaces: driverConfig.NetworkInterfacesConfig,
	}

	if err := dc.Validate(allowedPaths); err != nil {
		return nil, nil, fmt.Errorf("virt: invalid configuration %s: %w", cfg.AllocID, err)
	}

	if err := d.virtualizer.CreateDomain(dc); err != nil {
		return nil, nil, fmt.Errorf("virt: failed to start task %s: %w", cfg.AllocID, err)
	}

	h := &taskHandle{
		taskConfig: cfg,
		procState:  drivers.TaskStateRunning,
		startedAt:  time.Now().Round(time.Millisecond),
		logger:     d.logger.Named("handle").With(cfg.AllocID),
		taskGetter: d.taskGetter,
		name:       taskName,
	}

	// Build our network request to send now that the VM has been started. The
	// response will contain our teardown spec, which gets stored in the task
	// handle, so we can easily perform deletions.
	netBuildReq := net.VMStartedBuildRequest{
		DomainName: hostname,
		NetConfig:  &driverConfig.NetworkInterfacesConfig,
		Resources:  cfg.Resources,
	}

	netBuildResp, err := d.networkController.VMStartedBuild(&netBuildReq)
	if err != nil {
		return nil, nil, fmt.Errorf("virt: failed to build task network: %w", err)
	}

	h.netTeardown = netBuildResp.TeardownSpec

	d.logger.Info("task started successfully", "taskName", taskName)

	// Generate our driver state and send this to Nomad. It stores critical
	// information the driver will need to recover from failure and reattach
	// to running VMs.
	driverState := TaskState{
		NetTeardown: netBuildResp.TeardownSpec,
		StartedAt:   h.startedAt,
		TaskConfig:  cfg,
	}

	handle := drivers.NewTaskHandle(taskHandleVersion)
	handle.Config = cfg

	if err := handle.SetDriverState(&driverState); err != nil {
		return nil, nil, fmt.Errorf("virt: failed to set driver state for %s: %v",
			cfg.AllocID, err)
	}

	d.tasks.Set(cfg.ID, h)

	return handle, nil, nil
}

// RecoverTask recreates the in-memory state of a task from a TaskHandle.
func (d *VirtDriverPlugin) RecoverTask(handle *drivers.TaskHandle) error {
	if handle == nil {
		return errors.New("virt: handle cannot be nil")
	}

	if _, ok := d.tasks.Get(handle.Config.ID); ok {
		return nil
	}

	var taskState TaskState
	if err := handle.GetDriverState(&taskState); err != nil {
		return fmt.Errorf("virt: failed to decode task state from handle %s: %v",
			handle.Config.ID, err)
	}

	var driverConfig TaskConfig
	if err := taskState.TaskConfig.DecodeDriverConfig(&driverConfig); err != nil {
		return fmt.Errorf("virt: failed to decode driver config %s: %v", handle.Config.ID, err)
	}

	h := &taskHandle{
		name:        domainNameFromTaskID(handle.Config.ID),
		logger:      d.logger.Named("handle").With(handle.Config.AllocID),
		taskConfig:  taskState.TaskConfig,
		startedAt:   taskState.StartedAt,
		taskGetter:  d.taskGetter,
		netTeardown: taskState.NetTeardown,
	}

	vm, err := h.taskGetter.GetDomain(h.name)
	if err != nil {
		d.logger.Warn("Recovery restart failed, unknown task state", "task", handle.Config.ID)
		return fmt.Errorf("virt: failed to recover task %s: %v", handle.Config.ID, err)
	}

	if vm == nil {
		return drivers.ErrTaskNotFound
	}

	h.procState = setUpTaskState(vm.State)

	d.tasks.Set(handle.Config.ID, h)

	return nil
}

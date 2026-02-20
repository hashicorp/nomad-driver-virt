// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package plugin

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hashicorp/nomad-driver-virt/cloudinit"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/providers"
	"github.com/hashicorp/nomad-driver-virt/virt"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/client/lib/idset"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/client/lib/numalib/hw"
	"github.com/hashicorp/nomad/client/taskenv"
	"github.com/hashicorp/nomad/drivers/shared/eventer"
	"github.com/hashicorp/nomad/plugins/base"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/drivers/fsisolation"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

const (
	pluginName = "virt"

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

	// capabilities indicates what optional features this driver supports
	// this should be set according to the target run time.
	capabilities = &drivers.Capabilities{
		// The plugin's capabilities signal Nomad which extra functionalities
		// are supported. For a list of available options check the docs page:
		// https://godoc.org/github.com/hashicorp/nomad/plugins/drivers#Capabilities
		SendSignals:          false,
		Exec:                 false,
		DisableLogCollection: true,
		FSIsolation:          fsisolation.Image,

		// NetIsolationModes details that this driver only supports the network
		// isolation of host.
		NetIsolationModes: []drivers.NetIsolationMode{
			drivers.NetIsolationModeHost,
		},

		// MustInitiateNetwork is set to false, indicating the driver does not
		// implement and thus satisfy the Nomad drivers.DriverNetworkManager
		// interface.
		MustInitiateNetwork: false,

		// MountConfigs is currently not supported, although the plumbing is
		// ready to handle this.
		MountConfigs: drivers.MountConfigSupportNone,
	}

	ErrExistingTask    = errors.New("task is already running")
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

type VirtDriverPlugin struct {
	eventer        *eventer.Eventer
	providers      providers.Providers
	config         *virt.Config
	nomadConfig    *base.ClientDriverConfig
	tasks          *taskStore
	signalShutdown context.CancelFunc
	logger         hclog.Logger
	dataDir        string
	ci             cloudinit.CloudInit
}

// NewPlugin returns a new driver plugin
func NewPlugin(logger hclog.Logger) drivers.DriverPlugin {
	ctx, cancel := context.WithCancel(context.Background())
	logger = logger.Named(pluginName)

	// Should we check if extentions and kernel modules are there?
	// grep -E 'svm|vmx' /proc/cpuinfo
	// lsmod | grep kvm -> kvm_intel, kvm_amd, nvme-tcp
	return &VirtDriverPlugin{
		providers:      providers.New(ctx, logger),
		eventer:        eventer.NewEventer(ctx, logger),
		config:         &virt.Config{},
		tasks:          newTaskStore(),
		signalShutdown: cancel,
		logger:         logger,
	}
}

// PluginInfo returns information describing the plugin.
func (d *VirtDriverPlugin) PluginInfo() (*base.PluginInfoResponse, error) {
	return pluginInfo, nil
}

// ConfigSchema returns the plugin configuration schema.
func (d *VirtDriverPlugin) ConfigSchema() (*hclspec.Spec, error) {
	return virt.ConfigSpec(), nil
}

// SetConfig is called by the client to pass the configuration for the plugin.
func (d *VirtDriverPlugin) SetConfig(cfg *base.Config) error {
	var config virt.Config
	if len(cfg.PluginConfig) != 0 {
		if err := base.MsgPackDecode(cfg.PluginConfig, &config); err != nil {
			return err
		}
	}

	// Save the configuration to the plugin
	d.config = &config

	// Apply any required configuration updates
	d.config.Compat()

	// Save the Nomad agent configuration
	if cfg.AgentConfig != nil {
		d.nomadConfig = cfg.AgentConfig.Driver
	}

	if err := d.providers.Setup(d.config); err != nil {
		return fmt.Errorf("virt: failed to setup providers: %w", err)
	}

	if d.ci == nil {
		var err error
		if d.ci, err = cloudinit.NewController(d.logger); err != nil {
			return fmt.Errorf("virt: unable to create cloudinit controller: %w", err)
		}
	}

	return nil
}

// TaskConfigSchema returns the HCL schema for the configuration of a task.
func (d *VirtDriverPlugin) TaskConfigSchema() (*hclspec.Spec, error) {
	return virt.TaskConfigSpec(), nil
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
	fp, err := d.providers.Fingerprint()
	if err != nil {
		d.logger.Error("failed to generate fingerprint", "error", err)
		return &drivers.Fingerprint{
			Attributes:        map[string]*structs.Attribute{},
			Health:            drivers.HealthStateUndetected,
			HealthDescription: "",
		}
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

		handle.monitor(ctx, 0, exitCh)

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

	vmname := vmNameFromTaskID(taskID)
	virtualizer, err := d.providers.GetProviderForVM(vmname)
	if err != nil {
		return fmt.Errorf("virt: unable to stop task %s: %w", taskID, err)
	}

	if err := virtualizer.StopVM(vmname); err != nil {
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

	if handle.IsRunning() && !force {
		return fmt.Errorf("virt: cannot destroy running task %s", taskID)
	}

	vmname := vmNameFromTaskID(taskID)
	virtualizer, err := d.providers.GetProviderForVM(vmname)
	if err != nil {
		return fmt.Errorf("virt: unable to destroy task %s: %w", taskID, err)
	}
	network, err := virtualizer.Networking()
	if err != nil {
		return fmt.Errorf("virt: unable to destroy task %s: %w", taskID, err)
	}

	if err := virtualizer.DestroyVM(vmname); err != nil {
		return fmt.Errorf("virt: unable to destroy task %s: %w", taskID, err)
	}

	// Build our network request to send now that the VM has been destroyed.
	netTeardownReq := net.VMTerminatedTeardownRequest{
		TeardownSpec: handle.netTeardown,
	}
	if _, err := network.VMTerminatedTeardown(&netTeardownReq); err != nil {
		return fmt.Errorf("virt: failed to destroy task network: %w", err)
	}

	d.tasks.Delete(vmname)

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

// vmNameFromTaskConfig creates a name to be used for the vms, using the
// last 8 chars of the taskName which should be unique per task and the task name
// to help the operator identify the vms.
//
// The struct of the task ID is "allocID/taskName/UniqueID".
func vmNameFromTaskID(taskID string) string {
	ids := strings.Split(taskID, "/")
	return strings.Join(ids[1:], "-")
}

func createAllocFileMounts(task *drivers.TaskConfig) []vm.MountFileConfig {
	mounts := []vm.MountFileConfig{
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

// To create the alloc env vars, they are all writtent into a script in
// /etc/profile.d/virt.sh where the OS will take care of executing it at start.
func createEnvsFile(envs map[string]string) vm.File {
	con := []string{}

	for k, v := range envs {
		con = append(con, fmt.Sprintf("export %s=%s", k, v))
	}

	return vm.File{
		Encoding:    "b64",
		Path:        envVariblesFilePath,
		Permissions: envVariblesFilePermissions,
		Content:     base64.StdEncoding.EncodeToString([]byte(strings.Join(con, "\n\t"))),
	}
}

func addCMDsForMounts(mounts []vm.MountFileConfig) []string {
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

func buildHostname(taskName string) string {
	return fmt.Sprintf("nomad-%s", taskName)
}

// StartTask returns a task handle and a driver network if necessary.
func (d *VirtDriverPlugin) StartTask(cfg *drivers.TaskConfig) (*drivers.TaskHandle, *drivers.DriverNetwork, error) {
	if _, ok := d.tasks.Get(cfg.ID); ok {
		return nil, nil, fmt.Errorf("task with ID %q already started", cfg.ID)
	}

	var driverConfig virt.TaskConfig
	if err := cfg.DecodeDriverConfig(&driverConfig); err != nil {
		return nil, nil, fmt.Errorf("virt: failed to decode driver config: %v", err)
	}

	d.logger.Debug("starting task", "driver_cfg", hclog.Fmt("%+v\n", driverConfig))

	taskName := vmNameFromTaskID(cfg.ID)

	d.logger.Info("starting task", "name", taskName)

	// The process to have directories mounted on the VM consists on two steps,
	// one is declaring them as backing storage in the VM and the second is to
	// create the directory inside the VM and executing the mount using 9P.
	// These commands are added here to execute at bootime.
	allocFSMounts := createAllocFileMounts(cfg)
	bootCMDs := addCMDsForMounts(allocFSMounts)

	var osVariant *vm.OSVariant
	if driverConfig.OS != nil {
		osVariant = &vm.OSVariant{
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
	allowedPaths := append(d.config.ImagePaths, cfg.AllocDir)
	imagePaths := append(allowedPaths, cfg.TaskDir().Dir)

	cpuSet := idset.Parse[hw.CoreID](cfg.Resources.LinuxResources.CpusetCpus)

	// Fetch the virtualizer
	virtualizer, err := d.providers.Default()
	if err != nil {
		return nil, nil, fmt.Errorf("virt: failed to start task %s: %w", cfg.AllocID, err)
	}

	dc := &vm.Config{
		RemoveConfigFiles: true,
		Name:              taskName,
		Memory:            uint(cfg.Resources.NomadResources.Memory.MemoryMB),
		CPUs:              uint(cpuSet.Size()),
		CPUset:            cfg.Resources.LinuxResources.CpusetCpus,
		OsVariant:         osVariant,
		HostName:          hostname,
		Mounts:            allocFSMounts,
		CMDs:              driverConfig.CMDs,
		BOOTCMDs:          bootCMDs,
		CIUserData:        driverConfig.UserData,
		Password:          driverConfig.DefaultUserPassword,
		SSHKey:            driverConfig.DefaultUserSSHKey,
		Files:             []vm.File{createEnvsFile(cfg.Env)},
		NetworkInterfaces: driverConfig.NetworkInterfacesConfig,
	}

	disks := driverConfig.Disks
	// Compat to add old config into disks
	if driverConfig.ImagePath != "" {
		disks = disks.CompatAddImage(driverConfig.ImagePath, int64(driverConfig.PrimaryDiskSize),
			driverConfig.UseThinCopy)
	}

	// Fix up the image paths
	disks.ResolveImages(imagePaths)

	// If cloudinit configuration is available, add it
	if virtualizer.UseCloudInit() && dc.CloudInitConfig() != nil {
		isoPath := filepath.Join(cfg.AllocDir, "cloudinit.iso")
		if err := d.ci.Apply(dc.CloudInitConfig(), isoPath); err != nil {
			return nil, nil, err
		}
		// the iso will be copied into the storage pool, so
		// this file does not need to persist.
		defer os.RemoveAll(isoPath)

		disks = disks.ApplyCloudInit(isoPath)
	}

	// Set defaults on the disks
	disks.SetDefaults(virtualizer.Storage())
	// And set the updated disks into the config
	dc.Disks = disks

	// Run validation
	if err := dc.Validate(allowedPaths); err != nil {
		return nil, nil, fmt.Errorf("virt: invalid configuration %s: %w", cfg.AllocID, err)
	}

	// Prepare the disks to generate the storage pool volumes
	if err := dc.Disks.Prepare(taskName, virtualizer.Storage()); err != nil {
		return nil, nil, fmt.Errorf("virt: failed to create storage volumes %s: %w", cfg.AllocID, err)
	}

	if err := dc.Validate(allowedPaths); err != nil {
		return nil, nil, fmt.Errorf("virt: invalid configuration %s: %w", cfg.AllocID, err)
	}

	networking, err := virtualizer.Networking()
	if err != nil {
		return nil, nil, fmt.Errorf("virt: failed to start task %s: %w", cfg.AllocID, err)
	}

	if err := virtualizer.CreateVM(dc); err != nil {
		return nil, nil, fmt.Errorf("virt: failed to start task %s: %w", cfg.AllocID, err)
	}

	ifaces, err := virtualizer.GetNetworkInterfaces(dc.Name)
	if err != nil {
		return nil, nil, fmt.Errorf("virt: failed to retrieve guest interfaces %s: %w", cfg.AllocID, err)
	}
	hwaddrs := make([]string, len(ifaces))
	for i, iface := range ifaces {
		hwaddrs[i] = iface.MAC
	}

	h := &taskHandle{
		taskConfig: cfg,
		procState:  drivers.TaskStateRunning,
		startedAt:  time.Now().Round(time.Millisecond),
		logger:     d.logger.Named("handle").With("alloc-id", cfg.AllocID),
		taskGetter: d.providers,
		name:       taskName,
	}

	// Build our network request to send now that the VM has been started. The
	// response will contain our teardown spec, which gets stored in the task
	// handle, so we can easily perform deletions.
	netBuildReq := net.VMStartedBuildRequest{
		VMName:    taskName,
		Hostname:  hostname,
		NetConfig: driverConfig.NetworkInterfacesConfig,
		Resources: cfg.Resources,
		Hwaddrs:   hwaddrs,
	}

	// Build out the network now that the VM has been started.
	//
	// In the event of an error, we need to try and destroy the already running
	// VM. Nomad will not do this, as technically the task has not been started
	// from its perspective.
	//
	// In the future, we may want to add some retry logic when destroying a VM,
	// however, at least attempting it is a good start.
	netBuildResp, err := networking.VMStartedBuild(&netBuildReq)
	if err != nil {
		if destroyDomainErr := virtualizer.DestroyVM(taskName); destroyDomainErr != nil {
			d.logger.Error("virt: failed to destroy virtual machine, manual cleanup needed",
				"task_name", taskName, "error", destroyDomainErr)
		}
		return nil, nil, fmt.Errorf("virt: failed to build task network: %w", err)
	}

	// If the VM did not include any network configuration, there will not be a
	// teardown spec.
	if netBuildResp.TeardownSpec != nil {
		h.netTeardown = netBuildResp.TeardownSpec
	}

	d.logger.Info("task started successfully", "task_name", taskName)

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

	h := &taskHandle{
		name:        vmNameFromTaskID(handle.Config.ID),
		logger:      d.logger.Named("handle").With("alloc-id", handle.Config.AllocID),
		taskConfig:  taskState.TaskConfig,
		startedAt:   taskState.StartedAt,
		taskGetter:  d.providers,
		netTeardown: taskState.NetTeardown,
	}

	taskVm, err := h.taskGetter.GetVM(h.name)
	if err != nil {
		if errors.Is(err, vm.ErrNotFound) {
			return drivers.ErrTaskNotFound
		}

		d.logger.Warn("Recovery restart failed, unknown task state", "task", handle.Config.ID)
		return fmt.Errorf("virt: failed to recover task %s: %v", handle.Config.ID, err)
	}

	h.procState = taskVm.State.ToTaskState()

	d.tasks.Set(handle.Config.ID, h)

	return nil
}

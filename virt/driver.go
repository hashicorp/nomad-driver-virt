// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "github/hashicorp/nomad-driver-virt/internal/shared"
	"github/hashicorp/nomad-driver-virt/libvirt"

	"github.com/hashicorp/go-hclog"
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

	envVariblesFilePath        = "/etc/profile.d/virt.sh"
	envVariblesFilePermissions = "0777"
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
)

// TaskState is the runtime state which is encoded in the handle returned to
// Nomad client.
// This information is needed to rebuild the task state and handler during
// recovery.
type TaskState struct {
	ReattachConfig *structs.ReattachConfig
	TaskConfig     *drivers.TaskConfig
	StartedAt      time.Time
}

type Virtualizer interface {
	CreateDomain(config *domain.Config) error
	StopDomain(name string) error
	DestroyDomain(name string) error
	GetInfo() (domain.Info, error)
}

type VirtDriverPlugin struct {
	eventer        *eventer.Eventer
	virtualizer    Virtualizer
	config         *Config
	nomadConfig    *base.ClientDriverConfig
	tasks          *taskStore
	signalShutdown context.CancelFunc
	logger         hclog.Logger
}

// NewPlugin returns a new driver plugin
func NewPlugin(logger hclog.Logger) drivers.DriverPlugin {
	ctx, cancel := context.WithCancel(context.Background())
	logger = logger.Named(pluginName)

	v, err := libvirt.New(ctx, logger)
	if err != nil {
		print(err)
	}

	// Should we check if extentions and kernel modules are there?
	// grep -E 'svm|vmx' /proc/cpuinfo
	// lsmod | grep kvm -> kvm_intel, kvm_amd, nvme-tcp
	return &VirtDriverPlugin{
		eventer:        eventer.NewEventer(ctx, logger),
		virtualizer:    v,
		config:         &Config{},
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

	// TODO: parse and validated any configuration value if necessary.
	//
	// If your driver agent configuration requires any complex validation
	// (some dependency between attributes) or special data parsing (the
	// string "10s" into a time.Interval) you can do it here and update the
	// value in d.config.
	//
	// In the example below we check if the shell specified by the user is
	// supported by the plugin.
	/* 	shell := d.config.Shell
	   	if shell != "bash" && shell != "fish" {
	   		return fmt.Errorf("invalid shell %s", d.config.Shell)
	   	}
	*/
	// Save the Nomad agent configuration
	if cfg.AgentConfig != nil {
		d.nomadConfig = cfg.AgentConfig.Driver
	}

	// TODO: initialize any extra requirements if necessary.
	//
	// Here you can use the config values to initialize any resources that are
	// shared by all tasks that use this driver, such as a daemon process.

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
	ticker := time.NewTimer(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// after the initial fingerprint we can set the proper fingerprint
			// period
			ticker.Reset(fingerprintPeriod)
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
	attrs["driver.virt.active"] = structs.NewIntAttribute(int64(virtInfo.FreeMemory), "")
	attrs["driver.virt.inactive"] = structs.NewIntAttribute(int64(virtInfo.FreeMemory), "bytes")

	fp := &drivers.Fingerprint{
		Attributes:        attrs,
		Health:            drivers.HealthStateHealthy,
		HealthDescription: drivers.DriverHealthy,
	}

	return fp
}

func appendBootCMDsForMounts(mounts []domain.MountFileConfig) []string {
	cmds := []string{}
	for _, m := range mounts {
		c := []string{
			fmt.Sprintf("mkdir -p %s", m.Destination),
			fmt.Sprintf("mountpoint -q %s || mount -t virtiofs %s %s", m.Destination, m.Tag, m.Destination),
		}

		cmds = append(cmds, c...)
	}

	return cmds
}

func createAllocFileMounts(task *drivers.TaskConfig) []domain.MountFileConfig {
	mounts := []domain.MountFileConfig{
		{
			Source:      task.TaskDir().SharedAllocDir,
			Tag:         "allocDir",
			Destination: task.Env[taskenv.AllocDir],
		},
		{
			Source:      task.TaskDir().LocalDir,
			Tag:         "localDir",
			Destination: task.Env[taskenv.TaskLocalDir],
		},
		{
			Source:      task.TaskDir().SecretsDir,
			Tag:         "secretsDir",
			Destination: task.Env[taskenv.SecretsDir],
		},
	}

	return mounts
}

func createEnvsFile(envs map[string]string) []domain.File {
	con := []string{}

	for k, v := range envs {
		con = append(con, fmt.Sprintf("export %s=%s", k, v))
	}

	files := []domain.File{
		{
			Path:        envVariblesFilePath,
			Permissions: envVariblesFilePermissions,
			Content:     strings.Join(con, "\n"),
		},
	}

	return files
}

// StartTask returns a task handle and a driver network if necessary.
func (d *VirtDriverPlugin) StartTask(cfg *drivers.TaskConfig) (*drivers.TaskHandle, *drivers.DriverNetwork, error) {
	if _, ok := d.tasks.Get(cfg.ID); ok {
		return nil, nil, fmt.Errorf("task with ID %q already started", cfg.ID)
	}

	var driverConfig TaskConfig
	if err := cfg.DecodeDriverConfig(&driverConfig); err != nil {
		return nil, nil, fmt.Errorf("failed to decode driver config: %v", err)
	}

	d.logger.Info("starting task", "driver_cfg", hclog.Fmt("%+v", driverConfig))
	handle := drivers.NewTaskHandle(taskHandleVersion)
	handle.Config = cfg

	h := &taskHandle{
		taskConfig: cfg,
		procState:  drivers.TaskStateRunning,
		startedAt:  time.Now().Round(time.Millisecond),
		logger:     d.logger,
		virtURI:    d.config.EmulatorURI,
	}

	driverState := TaskState{
		//ReattachConfig: structs.ReattachConfigFromGoPlugin(pluginClient.ReattachConfig()),
		TaskConfig: cfg,
		StartedAt:  h.startedAt,
	}

	allocFSMounts := createAllocFileMounts(cfg)

	if err := d.virtualizer.CreateDomain(&domain.Config{
		Name:   cfg.ID,
		Memory: uint(cfg.Resources.NomadResources.Memory.MemoryMB),
		//Cores:             int(cfg.Resources.NomadResources.Cpu.ReservedCores[]),
		CPUs: int(cfg.Resources.NomadResources.Cpu.CpuShares),
		//OsVariant:         driverConfig.OSVariant.Type,
		BaseImage:         driverConfig.ImagePath,
		DiskFmt:           "qcow2",
		NetworkInterfaces: []string{"virbr0"},
		HostName:          cfg.Name,
		Files:             createEnvsFile(cfg.Env),
		Mounts:            allocFSMounts,
		BOOTCMDs:          appendBootCMDsForMounts(allocFSMounts),
		CMDs:              driverConfig.CMDs,
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to start task: %w", err)
	}

	if err := handle.SetDriverState(&driverState); err != nil {
		return nil, nil, fmt.Errorf("failed to set driver state: %v", err)
	}

	d.tasks.Set(cfg.ID, h)
	go h.run()
	return handle, nil, nil
}

// RecoverTask recreates the in-memory state of a task from a TaskHandle.
func (d *VirtDriverPlugin) RecoverTask(handle *drivers.TaskHandle) error {
	if handle == nil {
		return errors.New("error: handle cannot be nil")
	}

	if _, ok := d.tasks.Get(handle.Config.ID); ok {
		return nil
	}

	var taskState TaskState
	if err := handle.GetDriverState(&taskState); err != nil {
		return fmt.Errorf("failed to decode task state from handle: %v", err)
	}

	var driverConfig TaskConfig
	if err := taskState.TaskConfig.DecodeDriverConfig(&driverConfig); err != nil {
		return fmt.Errorf("failed to decode driver config: %v", err)
	}
	/*
			// TODO: implement driver specific logic to recover a task.
			//
			// Recovering a task involves recreating and storing a taskHandle as if the
			// task was just started.
			//
			// In the example below we use the executor to re-attach to the process
			// that was created when the task first started.
			plugRC, err := structs.ReattachConfigToGoPlugin(taskState.ReattachConfig)
			if err != nil {
				return fmt.Errorf("failed to build ReattachConfig from taskConfig state: %v", err)
			}

		 execImpl, pluginClient, err := executor.ReattachToExecutor(plugRC, d.logger)
			if err != nil {
				return fmt.Errorf("failed to reattach to executor: %v", err)
			}
	*/
	h := &taskHandle{
		//	exec:         execImpl,
		//	pluginClient: pluginClient,
		taskConfig: taskState.TaskConfig,
		procState:  drivers.TaskStateRunning,
		startedAt:  taskState.StartedAt,
		exitResult: &drivers.ExitResult{},
	}

	d.tasks.Set(taskState.TaskConfig.ID, h)

	go h.run()
	return nil
}

// WaitTask returns a channel used to notify Nomad when a task exits.
func (d *VirtDriverPlugin) WaitTask(ctx context.Context, taskID string) (<-chan *drivers.ExitResult, error) {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return nil, drivers.ErrTaskNotFound
	}

	ch := make(chan *drivers.ExitResult)
	go d.handleWait(ctx, handle, ch)
	return ch, nil
}

func (d *VirtDriverPlugin) handleWait(ctx context.Context, handle *taskHandle, ch chan *drivers.ExitResult) {
	defer close(ch)
	var result *drivers.ExitResult

	// TODO: implement driver specific logic to notify Nomad the task has been
	// completed and what was the exit result.
	//
	// When a result is sent in the result channel Nomad will stop the task and
	// emit an event that an operator can use to get an insight on why the task
	// stopped.
	//
	// In the example below we block and wait until the executor finishes
	// running, at which point we send the exit code and signal in the result
	// channel.
	/* 	ps, err := handle.exec.Wait(ctx)
	   	if err != nil {
	   		result = &drivers.ExitResult{
	   			Err: fmt.Errorf("executor: error waiting on process: %v", err),
	   		}
	   	} else {
	   		result = &drivers.ExitResult{
	   			ExitCode: ps.ExitCode,
	   			Signal:   ps.Signal,
	   		}
	   	} */

	for {
		select {
		case <-ctx.Done():
			return
		case ch <- result:
		}
	}
}

// StopTask stops a running task with the given signal and within the timeout window.
func (d *VirtDriverPlugin) StopTask(taskID string, timeout time.Duration, signal string) error {
	_, ok := d.tasks.Get(taskID)
	if !ok {
		return drivers.ErrTaskNotFound
	}

	err := d.virtualizer.DestroyDomain(taskID)
	if err != nil {
		return fmt.Errorf("virt: unable to stop task: %w", err)
	}

	return nil
}

// DestroyTask cleans up and removes a task that has terminated.
func (d *VirtDriverPlugin) DestroyTask(taskID string, force bool) error {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return drivers.ErrTaskNotFound
	}

	if handle.IsRunning() && !force {
		return errors.New("cannot destroy running task")
	}
	// Check for status to undefine task

	err := d.virtualizer.StopDomain(taskID)
	if err != nil {
		return fmt.Errorf("virt: unable to destroy task: %w", err)
	}

	d.tasks.Delete(taskID)
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
	_, ok := d.tasks.Get(taskID)
	if !ok {
		return nil, drivers.ErrTaskNotFound
	}

	// TODO: implement driver specific logic to send task stats.
	//
	// This function returns a channel that Nomad will use to listen for task
	// stats (e.g., CPU and memory usage) in a given interval. It should send
	// stats until the context is canceled or the task stops running.
	//
	// In the example below we use the Stats function provided by the executor,
	// but you can build a set of functions similar to the fingerprint process.
	//return handle.exec.Stats(ctx, interval)
	return nil, nil
}

// TaskEvents returns a channel that the plugin can use to emit task related events.
func (d *VirtDriverPlugin) TaskEvents(ctx context.Context) (<-chan *drivers.TaskEvent, error) {
	return d.eventer.TaskEvents(ctx)
}

// SignalTask forwards a signal to a task.
// This is an optional capability.
func (d *VirtDriverPlugin) SignalTask(taskID string, signal string) error {
	_, ok := d.tasks.Get(taskID)
	if !ok {
		return drivers.ErrTaskNotFound
	}

	// TODO: implement driver specific signal handling logic.
	//
	// The given signal must be forwarded to the target taskID. If this plugin
	// doesn't support receiving signals (capability SendSignals is set to
	// false) you can just return nil.
	/* sig := os.Interrupt
	if s, ok := signals.SignalLookup[signal]; ok {
		sig = s
	} else {
		d.logger.Warn("unknown signal to send to task, using SIGINT instead", "signal", signal, "task_id", handle.taskConfig.ID)

	}
	return handle.exec.Signal(sig) */
	return nil

}

// ExecTask returns the result of executing the given command inside a task.
// This is an optional capability.
func (d *VirtDriverPlugin) ExecTask(taskID string, cmd []string, timeout time.Duration) (*drivers.ExecTaskResult, error) {
	// TODO: implement driver specific logic to execute commands in a task.
	return nil, errors.New("This driver does not support exec")
}

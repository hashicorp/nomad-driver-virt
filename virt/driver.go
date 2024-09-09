// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	domain "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/libvirt"

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
}

type Virtualizer interface {
	CreateDomain(config *domain.Config) error
	StopDomain(name string) error
	DestroyDomain(name string) error
	GetInfo() (domain.VirtualizerInfo, error)
}

type DomainGetter interface {
	GetDomain(name string) (*domain.Info, error)
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
}

// NewPlugin returns a new driver plugin
func NewPlugin(logger hclog.Logger) drivers.DriverPlugin {
	ctx, cancel := context.WithCancel(context.Background())
	logger = logger.Named(pluginName)

	// Should we check if extentions and kernel modules are there?
	// grep -E 'svm|vmx' /proc/cpuinfo
	// lsmod | grep kvm -> kvm_intel, kvm_amd, nvme-tcp
	return &VirtDriverPlugin{
		eventer:        eventer.NewEventer(ctx, logger),
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

	v, err := libvirt.New(context.TODO(), d.logger, libvirt.WithDataDirectory(d.dataDir))
	if err != nil {
		return err
	}

	d.virtualizer = v
	d.taskGetter = v

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

// domainNameFromTaskID creates a name to be used for the vms, using the
// last 8 chars of the taskName which should be unique per task.
func domainNameFromTaskID(taskID string) string {
	return taskID[len(taskID)-8:]
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

// StartTask returns a task handle and a driver network if necessary.
func (d *VirtDriverPlugin) StartTask(cfg *drivers.TaskConfig) (*drivers.TaskHandle, *drivers.DriverNetwork, error) {
	if _, ok := d.tasks.Get(cfg.ID); ok {
		return nil, nil, fmt.Errorf("task with ID %q already started", cfg.ID)
	}

	var driverConfig TaskConfig
	if err := cfg.DecodeDriverConfig(&driverConfig); err != nil {
		return nil, nil, fmt.Errorf("failed to decode driver config: %v", err)
	}

	d.logger.Debug("starting task", "driver_cfg", hclog.Fmt("%+v", driverConfig))
	d.logger.Error(" errorr          ", fmt.Sprintf("%+v", cfg.Resources.LinuxResources))
	handle := drivers.NewTaskHandle(taskHandleVersion)
	handle.Config = cfg

	taskName := domainNameFromTaskID(cfg.ID)

	d.logger.Info("starting task", "name", taskName)

	h := &taskHandle{
		taskConfig: cfg,
		procState:  drivers.TaskStateRunning,
		startedAt:  time.Now().Round(time.Millisecond),
		logger:     d.logger.Named("handle").With(cfg.AllocID),
		taskGetter: d.taskGetter,
		name:       taskName,
	}

	driverState := TaskState{
		TaskConfig: cfg,
		StartedAt:  h.startedAt,
	}

	allocFSMounts := createAllocFileMounts(cfg)

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

	// The alloc directory and plugin data directory are always an allowed path to load images from.
	allowedPaths := append(d.config.ImagePaths, d.dataDir, cfg.AllocDir)

	diskImagePath := driverConfig.ImagePath
	if !fileExists(diskImagePath) {

		// Assume the image was downloaded using artifacts and will be placed
		// somewhere in the alloc's filesystem.
		diskImagePath = filepath.Join(cfg.TaskDir().Dir, diskImagePath)
		if !fileExists(diskImagePath) {
			return nil, nil, ErrImageNotFound
		}
	}

	diskFormat, err := d.getImageFormat(diskImagePath)
	if err != nil {
		return nil, nil, fmt.Errorf("virt: unable to get disk format %s: %w", cfg.AllocID, err)
	}

	if driverConfig.UseThinCopy {
		copyPath := filepath.Join(d.dataDir, taskName+".img")

		d.logger.Info("creating thin copy at", "path", copyPath)
		if err := d.createThinCopy(diskImagePath, copyPath, cfg.Resources.NomadResources.Memory.MemoryMB); err != nil {
			return nil, nil, fmt.Errorf("virt: unable to create thin copy for %s: %w", taskName, err)
		}

		diskImagePath = copyPath
		diskFormat = "qcow2"
	}

	dc := &domain.Config{
		RemoveConfigFiles: true,
		Name:              taskName,
		Memory:            uint(cfg.Resources.NomadResources.Memory.MemoryMB),
		CPUs:              uint(cfg.Resources.NomadResources.Cpu.CpuShares),
		CPUset:            cfg.Resources.LinuxResources.CpusetCpus,
		OsVariant:         osVariant,
		BaseImage:         diskImagePath,
		DiskFmt:           diskFormat,
		PrimaryDiskSize:   driverConfig.PrimaryDiskSize,
		HostName:          hostname,
		Mounts:            allocFSMounts,
		CMDs:              driverConfig.CMDs,
		CIUserData:        driverConfig.UserData,
		Password:          driverConfig.DefaultUserPassword,
		SSHKey:            driverConfig.DefaultUserSSHKey,
		Files:             []domain.File{createEnvsFile(cfg.Env)},
	}

	if err := dc.Validate(allowedPaths); err != nil {
		return nil, nil, fmt.Errorf("virt: invalid configuration %s: %w", cfg.AllocID, err)
	}

	if err := d.virtualizer.CreateDomain(dc); err != nil {
		return nil, nil, fmt.Errorf("virt: failed to start task %s: %w", cfg.AllocID, err)
	}

	d.logger.Info("task started successfully", "taskName", taskName)

	if err := handle.SetDriverState(&driverState); err != nil {
		return nil, nil, fmt.Errorf("virt: failed to set driver state for %s: %v", cfg.AllocID, err)
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
		name:       domainNameFromTaskID(handle.Config.ID),
		logger:     d.logger.Named("handle").With(handle.Config.AllocID),
		taskConfig: taskState.TaskConfig,
		startedAt:  taskState.StartedAt,
		taskGetter: d.taskGetter,
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

// GetImageFormat runs `qemu-img info` to get the format of a disk image.
func (d *VirtDriverPlugin) getImageFormat(basePath string) (string, error) {
	d.logger.Debug("reading the disk format", "base", basePath)

	var stdoutBuf, stderrBuf bytes.Buffer

	cmd := exec.Command("qemu-img", "info", "--output=json", basePath)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if err != nil {
		d.logger.Error("qemu-img read image", "stderr", stderrBuf.String())
		d.logger.Debug("qemu-img read image", "stdout", stdoutBuf.String())
		return "", err
	}

	d.logger.Debug("qemu-img read image", "stdout", stdoutBuf.String())

	// The qemu command returns more information, but for now, only the format
	// is necessary.
	var output = struct {
		Format string `json:"format"`
	}{}

	err = json.Unmarshal(stdoutBuf.Bytes(), &output)
	if err != nil {
		return "", fmt.Errorf("qemu-img: unable read info response %s: %w", basePath, err)
	}

	return output.Format, nil
}

func (d *VirtDriverPlugin) createThinCopy(basePath string, destination string, sizeM int64) error {
	d.logger.Debug("creating thin copy", "base", basePath, "dest", destination)

	var stdoutBuf, stderrBuf bytes.Buffer

	cmd := exec.Command("qemu-img", "create", "-b", basePath, "-f", "qcow2", "-F", "qcow2",
		destination, fmt.Sprintf("%dM", sizeM),
	)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if err != nil {
		d.logger.Error("qemu-img create output", "stderr", stderrBuf.String())
		d.logger.Debug("qemu-img create output", "stdout", stdoutBuf.String())
		return err
	}

	d.logger.Debug("qemu-img create output", "stdout", stdoutBuf.String())
	return nil
}

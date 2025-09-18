// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ccheshirecat/nomad-driver-ch/cloudinit"
	domain "github.com/ccheshirecat/nomad-driver-ch/internal/shared"
	"github.com/ccheshirecat/nomad-driver-ch/virt/net"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/ci"
	"github.com/hashicorp/nomad/helper/testlog"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/plugins/base"
	"github.com/hashicorp/nomad/plugins/drivers"
	dtestutil "github.com/hashicorp/nomad/plugins/drivers/testutils"
	plugins "github.com/hashicorp/nomad/plugins/shared/structs"
	"github.com/shoenig/test/must"
)

type mockNet struct{}

func (mn *mockNet) Fingerprint(map[string]*plugins.Attribute) {
}

func (mn *mockNet) Init() error {
	return nil
}

func (mn *mockNet) VMStartedBuild(*net.VMStartedBuildRequest) (*net.VMStartedBuildResponse, error) {
	return &net.VMStartedBuildResponse{}, nil
}

func (mn *mockNet) VMTerminatedTeardown(*net.VMTerminatedTeardownRequest) (*net.VMTerminatedTeardownResponse, error) {
	return &net.VMTerminatedTeardownResponse{}, nil
}

type mockImageHandler struct {
	lock sync.RWMutex

	basePath    string
	imageFormat string
	err         error
}

func (mh *mockImageHandler) GetImageFormat(basePath string) (string, error) {
	mh.basePath = basePath
	return mh.imageFormat, mh.err
}

func (mh *mockImageHandler) CreateThinCopy(basePath string, destination string, sizeM int64) error {
	return nil
}

type mockTaskGetter struct {
	lock sync.RWMutex

	count int
	info  *domain.Info
	err   error
}

func (mtg *mockTaskGetter) GetDomain(name string) (*domain.Info, error) {
	mtg.lock.Lock()
	defer mtg.lock.Unlock()

	mtg.count += 1
	return mtg.info, mtg.err
}

func (mtg *mockTaskGetter) getNumberOfCalls() int {
	mtg.lock.Lock()
	defer mtg.lock.Unlock()

	return mtg.count
}

type mockVirtualizar struct {
	lock sync.RWMutex

	config *domain.Config
	count  int
	err    error
}

func (mv *mockVirtualizar) Start(dataDir string) error {
	return nil
}

func (mv *mockVirtualizar) CreateDomain(config *domain.Config) error {
	mv.lock.Lock()
	defer mv.lock.Unlock()

	mv.count += 1
	mv.config = config

	return mv.err
}

func (mv *mockVirtualizar) getPassedConfig() *domain.Config {
	mv.lock.Lock()
	defer mv.lock.Unlock()

	return mv.config.Copy()
}

func (mv *mockVirtualizar) getNumberOfVMs() int {
	mv.lock.Lock()
	defer mv.lock.Unlock()

	return mv.count
}

func (mv *mockVirtualizar) StopDomain(name string) error {
	return nil
}

func (mv *mockVirtualizar) DestroyDomain(name string) error {
	mv.lock.Lock()
	defer mv.lock.Unlock()

	mv.count -= 1
	return nil
}

func (mv *mockVirtualizar) GetInfo() (domain.VirtualizerInfo, error) {
	return domain.VirtualizerInfo{}, nil
}

func (mv *mockVirtualizar) GetNetworkInterfaces(name string) ([]domain.NetworkInterface, error) {
	return []domain.NetworkInterface{}, nil
}

func (mv *mockVirtualizar) GetAllDomains() ([]domain.Info, error) {
	mv.lock.Lock()
	defer mv.lock.Unlock()

	// Return mock domains based on current count
	domains := make([]domain.Info, mv.count)
	for i := 0; i < mv.count; i++ {
		domains[i] = domain.Info{
			Name:  fmt.Sprintf("test-domain-%d", i),
			State: "running",
		}
	}
	return domains, nil
}

func createBasicResources() *drivers.Resources {
	res := drivers.Resources{
		NomadResources: &structs.AllocatedTaskResources{
			Memory: structs.AllocatedMemoryResources{
				MemoryMB: 6000,
			},
			Cpu: structs.AllocatedCpuResources{},
		},
		LinuxResources: &drivers.LinuxResources{
			CpusetCpus:       "1,2,3",
			CPUPeriod:        100000,
			CPUQuota:         100000,
			CPUShares:        2000,
			MemoryLimitBytes: 256 * 1024 * 1024,
			PercentTicks:     float64(500) / float64(2000),
		},
	}
	return &res
}

// virtDriverHarness wires up everything needed to launch a task with a virt driver.
// A driver plugin interface and cleanup function is returned
func virtDriverHarness(t *testing.T, v Virtualizer, dg DomainGetter, ih ImageHandler,
	dataDir string) *dtestutil.DriverHarness {
	logger := testlog.HCLogger(t)
	if testing.Verbose() {
		logger.SetLevel(hclog.Trace)
	} else {
		logger.SetLevel(hclog.Info)
	}

	baseConfig := &base.Config{}
	config := &Config{
		DataDir: dataDir,
	}

	if err := base.MsgPackEncode(&baseConfig.PluginConfig, config); err != nil {
		t.Error("Unable to encode plugin config", err)
	}

	d := NewPlugin(logger).(*VirtDriverPlugin)
	if v != nil {
		d.virtualizer = v
		d.networkController = &mockNet{}
		d.networkInit.Store(true)
	}

	must.NoError(t, d.SetConfig(baseConfig))
	d.imageHandler = ih
	d.taskGetter = dg

	harness := dtestutil.NewDriverHarness(t, d)

	return harness
}

func newTaskConfig(image string) TaskConfig {
	return TaskConfig{
		ImagePath:           image,
		UserData:            "/path/to/user/data",
		CMDs:                []string{"cmd arg arg", "cmd arg arg"},
		DefaultUserSSHKey:   "ssh-ed666 randomkey",
		DefaultUserPassword: "password",
		UseThinCopy:         false,
		PrimaryDiskSize:     2666,
		OS: &OS{
			Arch:    "arch",
			Machine: "machine",
		},
	}
}

func TestVirtDriver_Start_Wait_Destroy(t *testing.T) {
	ci.Parallel(t)

	tempDir, err := os.MkdirTemp("", "exampledir-*")
	must.NoError(t, err)
	defer os.RemoveAll(tempDir)

	mockImage, err := os.CreateTemp(tempDir, "test-*.img")
	must.NoError(t, err)
	defer os.Remove(mockImage.Name())

	allocID := uuid.Generate()
	taskCfg := newTaskConfig(mockImage.Name())

	taskID := fmt.Sprintf("%s/%s/%s", allocID[:7], "task-name", "0000000")
	task := &drivers.TaskConfig{
		ID:        taskID,
		AllocID:   allocID,
		Resources: createBasicResources(),
	}

	must.NoError(t, task.EncodeConcreteDriverConfig(&taskCfg))

	mockVirtualizer := &mockVirtualizar{}

	mockTaskGetter := &mockTaskGetter{
		info: &domain.Info{
			State: running,
		},
	}

	mockImageHandler := &mockImageHandler{
		imageFormat: "tif",
	}

	d := virtDriverHarness(t, mockVirtualizer, mockTaskGetter, mockImageHandler, tempDir)
	cleanup := d.MkAllocDir(task, true)
	defer cleanup()

	dth, _, err := d.StartTask(task)
	must.NoError(t, err)

	must.One(t, dth.Version)
	must.One(t, mockVirtualizer.getNumberOfVMs())

	callConfig := mockVirtualizer.getPassedConfig()
	// Assert the correct configuration was passed on to the virtualizer.
	must.Eq(t, "task-name-0000000", callConfig.Name)
	must.Eq(t, 6000, callConfig.Memory)
	must.Eq(t, 3, callConfig.CPUs)
	must.StrContains(t, "arch", callConfig.OsVariant.Arch)
	must.StrContains(t, "machine", callConfig.OsVariant.Machine)
	must.StrContains(t, mockImage.Name(), callConfig.BaseImage)
	must.StrContains(t, "tif", callConfig.DiskFmt)
	must.Eq(t, 2666, callConfig.PrimaryDiskSize)
	must.StrContains(t, "nomad-task-name-0000000", callConfig.HostName)
	must.Eq(t, 3, len(callConfig.Mounts))
	must.Eq(t, domain.MountFileConfig{
		Source:      task.AllocDir + "/alloc",
		Destination: "/alloc",
		ReadOnly:    true,
		Tag:         "allocDir",
	}, callConfig.Mounts[0])
	must.Eq(t, domain.MountFileConfig{
		Source:      task.AllocDir + "/local",
		Destination: "/local",
		ReadOnly:    true,
		Tag:         "localDir",
	}, callConfig.Mounts[1])
	must.Eq(t, domain.MountFileConfig{
		Source:      task.AllocDir + "/secrets",
		Destination: "/secrets",
		ReadOnly:    true,
		Tag:         "secretsDir",
	}, callConfig.Mounts[2])
	must.StrContains(t, "ssh-ed666 randomkey", callConfig.SSHKey)
	must.StrContains(t, "password", callConfig.Password)
	must.Eq(t, []string{"cmd arg arg", "cmd arg arg"}, callConfig.CMDs)
	must.Eq(t, []string{
		"mkdir -p /alloc",
		"mountpoint -q /alloc || mount -t 9p -o trans=virtio allocDir /alloc",
		"mkdir -p /local",
		"mountpoint -q /local || mount -t 9p -o trans=virtio localDir /local",
		"mkdir -p /secrets",
		"mountpoint -q /secrets || mount -t 9p -o trans=virtio secretsDir /secrets",
	}, callConfig.BOOTCMDs)

	// Attempt to wait
	waitCh, err := d.WaitTask(context.Background(), task.ID)
	must.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	statsChan, err := d.TaskStats(ctx, task.ID, time.Second)
	must.NoError(t, err)

	go func(t *testing.T) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-statsChan:
			case <-time.After(2 * time.Second):
				t.Error("no stats comming from task channel")
			}
		}
	}(t)

	select {
	case <-waitCh:
		t.Fatalf("wait channel should not have received an exit result")
	case <-time.After(10 * time.Second):
	}

	ts, err := d.InspectTask(task.ID)
	must.NoError(t, err)
	must.Eq(t, drivers.TaskStateRunning, ts.State)
	must.StrContains(t, task.ID, ts.ID)

	// Assert the correct monitoring
	must.Greater(t, 10, mockTaskGetter.getNumberOfCalls())

	cancel()
	err = d.DestroyTask(task.ID, true)
	must.NoError(t, err)
	must.Zero(t, mockVirtualizer.getNumberOfVMs())
}

func TestVirtDriver_Start_Recover_Destroy(t *testing.T) {
	ci.Parallel(t)

	tempDir, err := os.MkdirTemp("", "exampledir-*")
	must.NoError(t, err)
	defer os.RemoveAll(tempDir)

	mockImage, err := os.CreateTemp(tempDir, "test-*.img")
	must.NoError(t, err)
	defer os.Remove(mockImage.Name())

	allocID := uuid.Generate()
	taskCfg := newTaskConfig(mockImage.Name())

	taskID := fmt.Sprintf("%s/%s/%s", allocID[:7], "task-name", "0000000")
	task := &drivers.TaskConfig{
		ID:        taskID,
		AllocID:   allocID,
		Resources: createBasicResources(),
	}

	must.NoError(t, task.EncodeConcreteDriverConfig(&taskCfg))

	mockVirtualizer := &mockVirtualizar{}

	mockTaskGetter := &mockTaskGetter{
		info: &domain.Info{
			State: running,
		},
	}

	mockImageHandler := &mockImageHandler{
		imageFormat: "tif",
	}

	d := virtDriverHarness(t, mockVirtualizer, mockTaskGetter, mockImageHandler, tempDir)
	cleanup := d.MkAllocDir(task, true)
	defer cleanup()

	dth, _, err := d.StartTask(task)
	must.NoError(t, err)

	must.One(t, dth.Version)
	must.One(t, mockVirtualizer.getNumberOfVMs())

	callConfig := mockVirtualizer.getPassedConfig()
	// Assert the correct configuration was passed on to the virtualizer.
	must.Eq(t, "task-name-0000000", callConfig.Name)

	ts, err := d.InspectTask(task.ID)
	must.NoError(t, err)
	must.Eq(t, drivers.TaskStateRunning, ts.State)
	must.StrContains(t, task.ID, ts.ID)

	d = virtDriverHarness(t, mockVirtualizer, mockTaskGetter, mockImageHandler, tempDir)

	err = d.RecoverTask(dth)
	must.NoError(t, err)

	err = d.DestroyTask(task.ID, true)
	must.NoError(t, err)
	must.Zero(t, mockVirtualizer.getNumberOfVMs())
}

func TestVirtDriver_Start_Wait_Crashed(t *testing.T) {

	tempDir, err := os.MkdirTemp("", "exampledir-*")
	must.NoError(t, err)
	defer os.RemoveAll(tempDir)

	mockImage, err := os.CreateTemp(tempDir, "test-*.img")
	must.NoError(t, err)
	defer os.Remove(mockImage.Name())

	allocID := uuid.Generate()
	taskCfg := newTaskConfig(mockImage.Name())

	taskID := fmt.Sprintf("%s/%s/%s", allocID[:7], "task-name", "0000000")
	task := &drivers.TaskConfig{
		ID:        taskID,
		AllocID:   allocID,
		Resources: createBasicResources(),
	}

	must.NoError(t, task.EncodeConcreteDriverConfig(&taskCfg))

	mockVirtualizer := &mockVirtualizar{}

	mockTaskGetter := &mockTaskGetter{
		info: &domain.Info{
			State: crashed,
		},
	}

	mockImageHandler := &mockImageHandler{
		imageFormat: "tif",
	}

	d := virtDriverHarness(t, mockVirtualizer, mockTaskGetter, mockImageHandler, tempDir)
	cleanup := d.MkAllocDir(task, true)
	defer cleanup()

	dth, _, err := d.StartTask(task)
	must.NoError(t, err)

	must.One(t, dth.Version)
	must.One(t, mockVirtualizer.getNumberOfVMs())

	waitCh, err := d.WaitTask(context.Background(), task.ID)
	must.NoError(t, err)

	select {
	case exitResult := <-waitCh:
		must.One(t, exitResult.ExitCode)
		must.ErrorContains(t, exitResult.Err, "task has crashed")

	case <-time.After(10 * time.Second):
		t.Fatalf("wait channel should have received an exit result")
	}

	dts, err := d.InspectTask(task.ID)
	must.NoError(t, err)

	must.One(t, dts.ExitResult.ExitCode)
	must.Eq(t, "exited", dts.State)
}

func TestVirtDriver_ImageOptions(t *testing.T) {
	ci.Parallel(t)

	tempDir, err := os.MkdirTemp("", "exampledir-*")
	must.NoError(t, err)
	defer os.RemoveAll(tempDir)

	mockImage, err := os.CreateTemp(tempDir, "test-*.img")
	must.NoError(t, err)
	defer os.Remove(mockImage.Name())

	allocID := uuid.Generate()

	mockVirtualizer := &mockVirtualizar{}

	mockTaskGetter := &mockTaskGetter{
		info: &domain.Info{
			State: running,
		},
	}

	mockImageHandler := &mockImageHandler{
		imageFormat: "tif",
	}

	tests := []struct {
		name           string
		enableThinCopy bool
		expectedPath   string
		expectedFormat string
	}{
		{
			name:           "no_copy_requested",
			enableThinCopy: false,
			expectedPath:   fmt.Sprintf("%s/%s.img", tempDir, mockImage.Name()),
			expectedFormat: "tif",
		},
		{
			name:           "copy_requested",
			enableThinCopy: true,
			expectedPath:   fmt.Sprintf("%s/%s.img", tempDir, "task-name-0000000"),
			expectedFormat: "qcow2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskCfg := newTaskConfig(mockImage.Name())
			taskCfg.UseThinCopy = tt.enableThinCopy

			taskID := fmt.Sprintf("%s/%s/%s", allocID[:7], "task-name", "0000000")
			task := &drivers.TaskConfig{
				ID:        taskID,
				AllocID:   allocID,
				Resources: createBasicResources(),
			}
			must.NoError(t, task.EncodeConcreteDriverConfig(&taskCfg))

			d := virtDriverHarness(t, mockVirtualizer, mockTaskGetter, mockImageHandler, tempDir)
			cleanup := d.MkAllocDir(task, true)
			defer cleanup()

			_, _, err = d.StartTask(task)
			must.NoError(t, err)

			calledConfig := mockVirtualizer.getPassedConfig()
			must.StrContains(t, tt.expectedPath, calledConfig.BaseImage)
			must.StrContains(t, tt.expectedFormat, calledConfig.DiskFmt)
		})
	}
}

type cloudInitMock struct {
	passedConfig *cloudinit.Config
	err          error
}

func (cim *cloudInitMock) Apply(ci *cloudinit.Config, path string) error {
	if err := os.WriteFile(path, []byte("Hello, World!"), 0644); err != nil {
		return err
	}

	cim.passedConfig = ci

	return cim.err
}

func TestVirtDriver_Start_Wait_Destroy_LibvirtIntegration(t *testing.T) {
	ci.Parallel(t)

	tempDir, err := os.MkdirTemp("", "exampledir-*")
	must.NoError(t, err)
	defer os.RemoveAll(tempDir)

	mockImage, err := os.CreateTemp(tempDir, "test-*.img")
	must.NoError(t, err)
	defer os.Remove(mockImage.Name())

	allocID := uuid.Generate()
	taskCfg := newTaskConfig(mockImage.Name())
	taskCfg.UserData = ""
	taskCfg.OS = &OS{
		Arch:    "x86_64",
		Machine: "pc-i440fx-jammy",
	}

	taskID := fmt.Sprintf("%s/%s/%s", allocID[:7], "task-name", "0000000")
	task := &drivers.TaskConfig{
		ID:        taskID,
		AllocID:   allocID,
		Resources: createBasicResources(),
	}

	must.NoError(t, task.EncodeConcreteDriverConfig(&taskCfg))

	mockImageHandler := &mockImageHandler{
		imageFormat: "qcow2",
	}

	cloudInitMock := cloudInitMock{}

	v := &mockVirtualizar{
		count: 1, // Simulate initial test hypervisor domain
	}

	d := virtDriverHarness(t, v, v, mockImageHandler, tempDir)
	cleanup := d.MkAllocDir(task, true)
	defer cleanup()

	dth, _, err := d.StartTask(task)
	must.NoError(t, err)

	must.One(t, dth.Version)

	doms, err := v.GetAllDomains()
	must.NoError(t, err)

	// The initial test hypervisor has one plus the one that was just started.
	must.Len(t, 2, doms)

	// Attempt to wait
	waitCh, err := d.WaitTask(context.Background(), task.ID)
	must.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	statsChan, err := d.TaskStats(ctx, task.ID, time.Second)
	must.NoError(t, err)

	go func(t *testing.T) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-statsChan:
			case <-time.After(2 * time.Second):
				t.Error("no stats comming from task channel")
			}
		}
	}(t)

	select {
	case <-waitCh:
		t.Fatalf("wait channel should not have received an exit result")
	case <-time.After(10 * time.Second):
	}

	ts, err := d.InspectTask(task.ID)
	must.NoError(t, err)
	must.Eq(t, drivers.TaskStateRunning, ts.State)
	must.StrContains(t, task.ID, ts.ID)

	cancel()
	err = d.DestroyTask(task.ID, true)
	must.NoError(t, err)

	doms, err = v.GetAllDomains()
	must.NoError(t, err)

	// The initial test hypervisor has one plus the one that was just started.
	must.Len(t, 1, doms)
}

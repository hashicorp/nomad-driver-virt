// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	domain "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/libvirt"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/ci"
	"github.com/hashicorp/nomad/helper/testlog"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/plugins/base"
	"github.com/hashicorp/nomad/plugins/drivers"
	dtestutil "github.com/hashicorp/nomad/plugins/drivers/testutils"
	"github.com/shoenig/test/must"
)

type mockImageHandler struct {
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
	count int
	info  *domain.Info
	err   error
}

func (mtg *mockTaskGetter) GetDomain(name string) (*domain.Info, error) {
	mtg.count += 1
	return mtg.info, mtg.err
}

type mockVirtualizar struct {
	config *domain.Config
	count  int
	err    error
}

func (mv *mockVirtualizar) CreateDomain(config *domain.Config) error {
	mv.count += 1
	mv.config = config

	return mv.err
}

func (mv *mockVirtualizar) StopDomain(name string) error {
	return nil
}

func (mv *mockVirtualizar) DestroyDomain(name string) error {
	mv.count -= 1
	return nil
}

func (mv *mockVirtualizar) GetInfo() (domain.VirtualizerInfo, error) {
	return domain.VirtualizerInfo{}, nil
}

func createBasicResources() *drivers.Resources {
	res := drivers.Resources{
		NomadResources: &structs.AllocatedTaskResources{
			Memory: structs.AllocatedMemoryResources{
				MemoryMB: 6000,
			},
			Cpu: structs.AllocatedCpuResources{
				CpuShares: 250,
			},
		},
		LinuxResources: &drivers.LinuxResources{
			CPUPeriod:        100000,
			CPUQuota:         100000,
			CPUShares:        500,
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
	must.NoError(t, d.SetConfig(baseConfig))
	d.virtualizer = v
	d.imageHandler = ih
	d.taskGetter = dg

	d.buildFingerprint()

	harness := dtestutil.NewDriverHarness(t, d)

	return harness
}

func newTaskConfig(image string) TaskConfig {
	return TaskConfig{
		ImagePath:           image,
		UserData:            "/user/data/path",
		CMDs:                []string{"cmd arg arg", "cmd arg arg"},
		DefaultUserSSHKey:   "ssh-ed666 randomkey",
		DefaultUserPassword: "password",
		//UseThinCopy:         true,
		PrimaryDiskSize: 2666,
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

	mockVirtualizer := &mockVirtualizar{
		count: 0,
	}

	mockTaskGetter := &mockTaskGetter{
		count: 0,
		info: &domain.Info{
			State: libvirt.DomainRunning,
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
	must.One(t, mockVirtualizer.count)

	// Assert the correct configuration was passed on to the virtualizer.
	must.Eq(t, "task-name-0000000", mockVirtualizer.config.Name)
	must.Eq(t, 6000, mockVirtualizer.config.Memory)
	must.Eq(t, 250, mockVirtualizer.config.CPUs)
	must.StrContains(t, "arch", mockVirtualizer.config.OsVariant.Arch)
	must.StrContains(t, "machine", mockVirtualizer.config.OsVariant.Machine)
	must.StrContains(t, mockImage.Name(), mockVirtualizer.config.BaseImage)
	must.StrContains(t, "tif", mockVirtualizer.config.DiskFmt)
	must.Eq(t, 2666, mockVirtualizer.config.PrimaryDiskSize)
	must.StrContains(t, "nomad-task-name-0000000", mockVirtualizer.config.HostName)
	must.Eq(t, 3, len(mockVirtualizer.config.Mounts))
	must.Eq(t, domain.MountFileConfig{
		Source:      task.AllocDir + "/alloc",
		Destination: "/alloc",
		ReadOnly:    true,
		Tag:         "allocDir",
	}, mockVirtualizer.config.Mounts[0])
	must.Eq(t, domain.MountFileConfig{
		Source:      task.AllocDir + "/local",
		Destination: "/local",
		ReadOnly:    true,
		Tag:         "localDir",
	}, mockVirtualizer.config.Mounts[1])
	must.Eq(t, domain.MountFileConfig{
		Source:      task.AllocDir + "/secrets",
		Destination: "/secrets",
		ReadOnly:    true,
		Tag:         "secretsDir",
	}, mockVirtualizer.config.Mounts[2])
	must.StrContains(t, "ssh-ed666 randomkey", mockVirtualizer.config.SSHKey)
	must.StrContains(t, "password", mockVirtualizer.config.Password)
	must.Eq(t, []string{"cmd arg arg", "cmd arg arg"}, mockVirtualizer.config.CMDs)
	must.Eq(t, []string{
		"mkdir -p /alloc",
		"mountpoint -q /alloc || mount -t 9p -o trans=virtio allocDir /alloc",
		"mkdir -p /local",
		"mountpoint -q /local || mount -t 9p -o trans=virtio localDir /local",
		"mkdir -p /secrets",
		"mountpoint -q /secrets || mount -t 9p -o trans=virtio secretsDir /secrets",
	}, mockVirtualizer.config.BOOTCMDs)

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
	// TODO: Assert task status
	must.StrContains(t, task.ID, ts.ID)
	// Assert the correct monitoring
	must.Greater(t, 10, mockTaskGetter.count)

	cancel()
	err = d.DestroyTask(task.ID, true)
	must.NoError(t, err)
	must.Zero(t, mockVirtualizer.count)
}

func TestVirtDriver_Start_Wait_Crash(t *testing.T) {

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

	mockVirtualizer := &mockVirtualizar{
		count: 0,
	}

	mockTaskGetter := &mockTaskGetter{
		count: 0,
		info: &domain.Info{
			State: libvirt.DomainCrashed,
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
	must.One(t, mockVirtualizer.count)

	// Attempt to wait
	waitCh, err := d.WaitTask(context.Background(), task.ID)
	must.NoError(t, err)

	select {
	case exitResult := <-waitCh:
		must.One(t, exitResult.ExitCode)
		fmt.Printf("\n %+T \n %+T\n", ErrTaskCrashed, exitResult.Err)
		must.ErrorIs(t, ErrTaskCrashed, exitResult.Err)

	case <-time.After(10 * time.Second):
		t.Fatalf("wait channel should have received an exit result")
	}

	_, err = d.InspectTask(task.ID)
	must.Error(t, err)
}

// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"context"
	"os"
	"testing"
	"time"

	domain "github.com/hashicorp/nomad-driver-virt/internal/shared"

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

type mockImageHandler struct{}

func (mh *mockImageHandler) GetImageFormat(basePath string) (string, error) {
	return "", nil
}

func (mh *mockImageHandler) CreateThinCopy(basePath string, destination string, sizeM int64) error {
	return nil
}

type mockTaskGetter struct{}

func (mtg *mockTaskGetter) GetDomain(name string) (*domain.Info, error) {
	return nil, nil
}

type mockVirtualizar struct {
}

func (mv *mockVirtualizar) CreateDomain(config *domain.Config) error {
	return nil
}

func (mv *mockVirtualizar) StopDomain(name string) error {
	return nil
}

func (mv *mockVirtualizar) DestroyDomain(name string) error {
	return nil
}

func (mv *mockVirtualizar) GetInfo() (domain.VirtualizerInfo, error) {
	return domain.VirtualizerInfo{}, nil
}

func createBasicResources() *drivers.Resources {
	res := drivers.Resources{
		NomadResources: &structs.AllocatedTaskResources{
			Memory: structs.AllocatedMemoryResources{
				MemoryMB: 100,
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
func virtDriverHarness(t *testing.T, v Virtualizer, cfg map[string]interface{},
	dg DomainGetter, ih ImageHandler, dataDir string) *dtestutil.DriverHarness {
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

	d.buildFingerprint()

	harness := dtestutil.NewDriverHarness(t, d)

	return harness
}

/* func Test_StartAndStopTask(t *testing.T) {
	logger := hclog.NewNullLogger()

	mv := &mockVirtualizar{}
	mtg := &mockTaskGetter{}
	mih := &mockImageHandler{}

	vp := &VirtDriverPlugin{
		config: &Config{
			ImagePaths: []string{"valid/path/here"},
		},
		tasks:        newTaskStore(),
		logger:       logger,
		virtualizer:  mv,
		taskGetter:   mtg,
		imageHandler: mih,
	}

	th, dn, err := vp.StartTask(&drivers.TaskConfig{
		ID: "allocID/taskName/12345678",
	})
	fmt.Println(th, dn, err)
} */

func newTaskConfig(image string, command []string) TaskConfig {
	return TaskConfig{
		ImagePath:           image,
		UserData:            "/user/data/path",
		CMDs:                command,
		DefaultUserSSHKey:   "ssh-ed666 randomkey",
		DefaultUserPassword: "password",
		UseThinCopy:         true,
		PrimaryDiskSize:     666,
	}
}

func TestVirtDriver_Start_Wait(t *testing.T) {
	ci.Parallel(t)

	tempFile, err := os.CreateTemp("", "testfile-*.txt")
	must.NoError(t, err)
	defer os.Remove(tempFile.Name())

	taskCfg := newTaskConfig("", []string{"cmd arg arg", "cmd arg arg"})
	task := &drivers.TaskConfig{
		ID:        uuid.Generate(),
		Name:      "start_wait",
		AllocID:   uuid.Generate(),
		Resources: createBasicResources(),
	}
	must.NoError(t, task.EncodeConcreteDriverConfig(&taskCfg))

	mv := &mockVirtualizar{}
	mtg := &mockTaskGetter{}
	mih := &mockImageHandler{}

	d := virtDriverHarness(t, mv, nil, mtg, mih, tempFile.Name())
	cleanup := d.MkAllocDir(task, true)
	defer cleanup()

	_, _, err = d.StartTask(task)
	must.NoError(t, err)

	defer func() {
		_ = d.DestroyTask(task.ID, true)
	}()

	// Attempt to wait
	waitCh, err := d.WaitTask(context.Background(), task.ID)
	must.NoError(t, err)

	select {
	case <-waitCh:
		t.Fatalf("wait channel should not have received an exit result")
	case <-time.After(10 * time.Second):
	}
}

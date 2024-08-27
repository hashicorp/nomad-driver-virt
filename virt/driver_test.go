// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/helper/testlog"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/plugins/base"
	"github.com/hashicorp/nomad/plugins/drivers"
	dtestutil "github.com/hashicorp/nomad/plugins/drivers/testutils"
	"github.com/shoenig/test/must"
)

var (
	// busyboxLongRunningCmd is a busybox command that runs indefinitely, and
	// ideally responds to SIGINT/SIGTERM.  Sadly, busybox:1.29.3 /bin/sleep doesn't.
	busyboxLongRunningCmd = []string{"nc", "-l", "-p", "3000", "127.0.0.1"}
)

func getVirtDriver(t *testing.T, harness *dtestutil.DriverHarness) *VirtDriverPlugin {
	driver, ok := harness.Impl().(*VirtDriverPlugin)
	must.True(t, ok)
	return driver
}

func createBasicResources() *drivers.Resources {
	res := drivers.Resources{
		NomadResources: &structs.AllocatedTaskResources{
			Memory: structs.AllocatedMemoryResources{
				MemoryMB: 25600,
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

// driverHarness wires up everything needed to launch a task with a podman driver.
// A driver plugin interface and cleanup function is returned
func driverHarness(t *testing.T, cfg map[string]interface{}) *dtestutil.DriverHarness {
	logger := testlog.HCLogger(t)
	if testing.Verbose() {
		logger.SetLevel(hclog.Info)
	} else {
		logger.SetLevel(hclog.Info)
	}

	baseConfig := base.Config{}
	pluginConfig := Config{}

	if err := base.MsgPackEncode(&baseConfig.PluginConfig, &pluginConfig); err != nil {
		t.Error("Unable to encode plugin config", err)
	}

	d := NewPlugin(logger).(*VirtDriverPlugin)
	must.NoError(t, d.SetConfig(&baseConfig))

	d.buildFingerprint()

	if v, ok := cfg["Datadir"]; ok {
		if sv, ok := v.(string); ok {
			d.config.DataDir = sv
		}
	}

	harness := dtestutil.NewDriverHarness(t, d)

	return harness
}

func newTaskConfig(image string, name string) TaskConfig {

	return TaskConfig{
		ImagePath:           image,
		Hostname:            name,
		UserData:            "/home/ubuntu/cc/user-data",
		DefaultUserPassword: "password",
		UseThinCopy:         true,
	}
}

func TestVirtDriver_Start_WaitFinish(t *testing.T) {
	t.Parallel()
	allocID := uuid.Generate()

	/* 	imgPath := "/home/ubuntu/" + allocID[0:7] + ".img"
	   	err := createCopy("/home/ubuntu/focal-server-cloudimg-amd64.img", "/home/ubuntu/", allocID[0:7])
	   	must.NoError(t, err)
	*/
	br := createBasicResources()
	br.LinuxResources = &drivers.LinuxResources{}

	task := &drivers.TaskConfig{
		Name:      "start_waitfinish",
		AllocID:   allocID,
		Resources: br,
		ID:        "nomad-" + uuid.Short(),
	}

	taskCfg := newTaskConfig("/home/ubuntu/focal-server-cloudimg-amd64.img", allocID)

	must.NoError(t, task.EncodeConcreteDriverConfig(&taskCfg))

	opts := make(map[string]interface{})

	d := driverHarness(t, opts)

	cleanup := d.MkAllocDir(task, true)
	defer cleanup()

	_, _, err := d.StartTask(task)
	must.NoError(t, err)

	defer func() {
		err = d.DestroyTask(task.ID, true)
		must.NoError(t, err)
	}()
	waitCh, err := d.WaitTask(context.Background(), task.ID)
	must.NoError(t, err)

	statsChannel, err := d.TaskStats(context.TODO(), task.ID, time.Second)
	must.NoError(t, err)
	fmt.Println(statsChannel)
	// Attempt to wait
	time.Sleep(10 * time.Second)
	t.FailNow()
	select {
	case res := <-waitCh:
		must.True(t, res.Successful())
	case <-time.After(20 * time.Second):
		must.Unreachable(t, must.Sprint("timeout"))
	}
}

func createCopy(basePath string, destination string, name string) error {

	cmd := exec.Command("bash", "-c", fmt.Sprintf("qemu-img create -b %s -f qcow2 -F qcow2 %s 8G", basePath, destination+name+".img"))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

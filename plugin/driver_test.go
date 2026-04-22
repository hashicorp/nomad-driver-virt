// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/cloudinit"
	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/providers"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt"
	"github.com/hashicorp/nomad-driver-virt/storage"
	mock_cloudinit "github.com/hashicorp/nomad-driver-virt/testutil/mock/cloudinit"
	mock_providers "github.com/hashicorp/nomad-driver-virt/testutil/mock/providers"
	mock_storage "github.com/hashicorp/nomad-driver-virt/testutil/mock/storage"
	mock_image_tools "github.com/hashicorp/nomad-driver-virt/testutil/mock/storage/image_tools"
	mock_virt "github.com/hashicorp/nomad-driver-virt/testutil/mock/virt"
	mock_virt_net "github.com/hashicorp/nomad-driver-virt/testutil/mock/virt/net"
	"github.com/hashicorp/nomad-driver-virt/virt"
	"github.com/hashicorp/nomad-driver-virt/virt/disks"
	"github.com/hashicorp/nomad/ci"
	"github.com/hashicorp/nomad/helper/testlog"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/plugins/base"
	"github.com/hashicorp/nomad/plugins/drivers"
	driver_testutils "github.com/hashicorp/nomad/plugins/drivers/testutils"
	"github.com/shoenig/test/must"
)

const (
	testOsArch    = "x86_64"
	testOsMachine = "pc-i440fx-jammy"
)

func modulesPath(t *testing.T) string {
	return filepath.Join(filepath.Dir(t.TempDir()), "test-modules")
}

func testHarness(t *testing.T, config *virt.Config, p providers.Providers, ci cloudinit.CloudInit, task *drivers.TaskConfig, timeout time.Duration) *driver_testutils.DriverHarness {
	t.Helper()

	libvirt.ModifyMountFsAvailability(func() (map[string]struct{}, error) {
		return map[string]struct{}{"virtio-9p-pci": {}}, nil
	})
	t.Cleanup(func() { libvirt.ModifyMountFsAvailability(nil) })

	// Setup the testing logger
	logger := testlog.HCLogger(t)
	if testing.Verbose() {
		logger.SetLevel(hclog.Trace)
	} else {
		logger.SetLevel(hclog.Info)
	}

	// Encode the configuration
	c := &base.Config{}
	must.NoError(t, base.MsgPackEncode(&c.PluginConfig, config),
		must.Sprint("could not encode plugin configuration"))

	// Create the plugin and set the passed interfaces
	d := NewPlugin(logger).(*VirtDriverPlugin)
	d.providers = p
	d.ci = ci

	// Set the config and create the harness
	must.NoError(t, d.SetConfig(c),
		must.Sprint("cloud not set the plugin configuration"))

	driver := driver_testutils.NewDriverHarness(t, d)
	t.Cleanup(driver.MkAllocDir(task, false))

	if timeout > 0 {
		ctx, cancel := context.WithCancel(t.Context())
		t.Cleanup(cancel)
		go func() {
			select {
			case <-ctx.Done():
			case <-time.After(timeout):
				driver.Kill()
			}
		}()
	}

	return driver
}

func driverConfig(dir string) *virt.Config {
	return &virt.Config{
		ImagePaths: []string{dir},
		StoragePools: &storage.Config{
			Directory: map[string]storage.Directory{
				"default-pool": {Path: filepath.Join(dir, "pools", "default-pool")},
			},
		},
	}
}

func testResources() *drivers.Resources {
	return &drivers.Resources{
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
}

func testTaskConfig() *drivers.TaskConfig {
	allocID := uuid.Generate()
	return &drivers.TaskConfig{
		ID:        fmt.Sprintf("%s/%s/%s", allocID[:7], "test-task", uuid.Generate()[:8]),
		AllocID:   allocID,
		Resources: testResources(),
	}
}

func testVirtTaskConfig(t *testing.T, imgDir string) virt.TaskConfig {
	t.Helper()

	must.NoError(t, os.MkdirAll(imgDir, 0755))
	img, err := os.CreateTemp(imgDir, "*.img")
	must.NoError(t, err, must.Sprint("failed to create test image file"))
	img.Close()

	return virt.TaskConfig{
		UserData:            "/path/to/user/data",
		CMDs:                []string{"cmd arg arg", "cmd arg arg"},
		DefaultUserSSHKey:   "ssh-ed666 randomkey",
		DefaultUserPassword: "password",
		Disks: disks.Disks{
			{
				Size: "50MB",
				Source: &disks.Source{
					Image: img.Name(),
				},
			},
		},
		OS: &virt.OS{
			Arch:    testOsArch,
			Machine: testOsMachine,
		},
	}
}

// These tests are fully mocked
func TestVirtDriver(t *testing.T) {
	ci.Parallel(t)

	t.Run("start wait destroy", func(t *testing.T) {
		dir := t.TempDir()
		virtcfg := testVirtTaskConfig(t, filepath.Join(dir, "images"))
		task := testTaskConfig()
		// Add a mount to test
		task.Mounts = []*drivers.MountConfig{
			{
				HostPath: "/testing/path/host",
				TaskPath: "/testing/path/guest",
				Readonly: false,
			},
		}
		must.NoError(t, task.EncodeConcreteDriverConfig(virtcfg))
		vmName := vmNameFromTaskID(task.ID)

		// Create all the needed mocks
		ih := &mock_image_tools.StaticImageHandler{GetImageFormatResult: "tif"}
		st := mock_storage.NewMockStorage(t)
		defer st.AssertExpectations()
		pl := mock_storage.NewMockPool(t)
		defer pl.AssertExpectations()
		vt := mock_virt.NewMock(t)
		defer vt.AssertExpectations()
		pv := mock_providers.NewStatic(vt)
		ci := mock_cloudinit.NewStaticCloudInit()

		driverCfg := driverConfig(dir)
		// Load initialization expectations
		vt.Expect(
			mock_virt.Init{},
			mock_virt.SetupStorage{Config: driverCfg.StoragePools},
			mock_virt.Networking{Result: mock_virt_net.NewStatic()},
			mock_virt.GenerateMountCommands{
				Result: []string{
					"mkdir -p /alloc",
					"mountpoint -q /alloc || mount -t 9p -o trans=virtio allocDir /alloc",
					"mkdir -p /local",
					"mountpoint -q /local || mount -t 9p -o trans=virtio localDir /local",
					"mkdir -p /secrets",
					"mountpoint -q /secrets || mount -t 9p -o trans=virtio secretsDir /secrets",
					"mkdir -p /testing/path/guest",
					"mountpoint -q /testing/path/guest || mount -t 9p -o trans=virtio _testing_path_guest /testing/path/guest",
				},
			},
		)

		// Build the test driver and create the alloc directory
		driver := testHarness(t, driverCfg, pv, ci, task, 1*time.Second)

		// Set all the expectations for the mocks
		st.Expect(
			mock_storage.ImageHandler{Result: ih},
			mock_storage.DefaultPool{Result: pl},
			mock_storage.DefaultPool{Result: pl},
			mock_storage.DefaultPool{Result: pl},
			mock_storage.DefaultPool{Result: pl},
			mock_storage.DefaultPool{Result: pl},
			mock_storage.DefaultPool{Result: pl},
			mock_storage.GenerateDeviceName{BusType: "virtio", ExistingDevices: []string{}, Result: "sda"},
			mock_storage.GenerateDeviceName{BusType: "ide", ExistingDevices: []string{"sda"}, Result: "hda"},
			mock_storage.DefaultDiskDriver{Result: "test-driver"},
			mock_storage.DefaultDiskDriver{Result: "test-driver"},
		)

		vt.Expect(
			mock_virt.UseCloudInit{Result: true},
			mock_virt.Storage{Result: st},
			mock_virt.Storage{Result: st},
			mock_virt.Storage{Result: st},
			mock_virt.Networking{Result: mock_virt_net.NewStatic()},
			mock_virt.CreateVM{
				Config: &vm.Config{
					RemoveConfigFiles: true,
					Name:              vmName,
					Memory:            6000,
					CPUset:            "1,2,3",
					CPUs:              3,
					OsVariant:         &vm.OSVariant{Arch: testOsArch, Machine: testOsMachine},
					HostName:          "nomad-" + vmName,
					Mounts: []vm.MountFileConfig{
						{
							Source:      filepath.Join(task.AllocDir, "alloc"),
							Destination: "/alloc",
							ReadOnly:    true,
							Tag:         "allocDir",
						},
						{
							Source:      filepath.Join(task.AllocDir, "local"),
							Destination: "/local",
							ReadOnly:    true,
							Tag:         "localDir",
						},
						{
							Source:      filepath.Join(task.AllocDir, "secrets"),
							Destination: "/secrets",
							ReadOnly:    true,
							Tag:         "secretsDir",
						},
						{
							Source:      "/testing/path/host",
							Destination: "/testing/path/guest",
							Tag:         "_testing_path_guest",
						},
					},
					Files: []vm.File{
						{
							Path:        "/etc/profile.d/virt.sh",
							Permissions: "777",
							Encoding:    "b64",
						},
					},
					SSHKey:   "ssh-ed666 randomkey",
					Password: "password",
					CMDs:     []string{"cmd arg arg", "cmd arg arg"},
					BOOTCMDs: []string{
						"mkdir -p /alloc",
						"mountpoint -q /alloc || mount -t 9p -o trans=virtio allocDir /alloc",
						"mkdir -p /local",
						"mountpoint -q /local || mount -t 9p -o trans=virtio localDir /local",
						"mkdir -p /secrets",
						"mountpoint -q /secrets || mount -t 9p -o trans=virtio secretsDir /secrets",
						"mkdir -p /testing/path/guest",
						"mountpoint -q /testing/path/guest || mount -t 9p -o trans=virtio _testing_path_guest /testing/path/guest",
					},
					CIUserData: "/path/to/user/data",
					Volumes: []storage.Volume{
						{
							Kind:       "disk",
							Driver:     "test-driver",
							Format:     "tif",
							DeviceName: "sda",
							BusType:    "virtio",
							Primary:    true,
						},
						{
							Kind:       "cdrom",
							Driver:     "test-driver",
							Format:     "raw",
							DeviceName: "hda",
							BusType:    "ide",
						},
					},
				},
			},
			mock_virt.GetNetworkInterfaces{Name: vmName},
			mock_virt.Networking{Result: mock_virt_net.NewStatic()},
			mock_virt.DestroyVM{Name: vmName},
			// GetVM is used for stats, and they should be collected twice
			mock_virt.GetVM{Name: vmName, Result: &vm.Info{State: vm.VMStateRunning}},
			mock_virt.GetVM{Name: vmName, Result: &vm.Info{State: vm.VMStateRunning}},
		)

		// stub path that would be created by cloudinit
		f, err := os.Create(filepath.Join(task.AllocDir, "cloudinit.iso"))
		must.NoError(t, err)
		f.Close()

		pl.Expect(
			mock_storage.DefaultImageFormat{Result: "tif"},
			mock_storage.AddVolume{
				Name: vmName + "_sda.img",
				Opts: storage.Options{
					Size: 50000000,
					Target: storage.Target{
						Format: "tif",
					},
					Source: storage.Source{
						Path: virtcfg.Disks[0].Source.Image,
					},
				},
				Result: &storage.Volume{},
			},
			mock_storage.AddVolume{
				Name: vmName + "_hda.img",
				Opts: storage.Options{
					Target: storage.Target{Format: "raw"},
					Source: storage.Source{Path: f.Name()},
				},
				Result: &storage.Volume{},
			},
		)

		// start the task
		taskHandle, _, err := driver.StartTask(task)
		must.NoError(t, err)
		must.One(t, taskHandle.Version)

		waitCh, err := driver.WaitTask(t.Context(), task.ID)
		must.NoError(t, err)

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		statsChan, err := driver.TaskStats(ctx, task.ID, 50*time.Millisecond)
		must.NoError(t, err)

		// watch for stats to ensure they are received
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-statsChan:
				case <-time.After(100 * time.Millisecond):
					t.Error("no stats received")
					return
				}
			}
		}()

		// wait long enough for stats to get pulled twice
		select {
		case <-waitCh:
			t.Fatal("wait channel received unexpected exit result")
		case <-time.After(110 * time.Millisecond):
		}

		// inspect the task to verify running state
		ts, err := driver.InspectTask(task.ID)
		must.NoError(t, err)
		must.Eq(t, drivers.TaskStateRunning, ts.State)
		must.StrContains(t, task.ID, ts.ID)

		// force destroy the task
		must.NoError(t, driver.DestroyTask(task.ID, true))
	})

	t.Run("start recover destroy", func(t *testing.T) {
		dir := t.TempDir()
		virtcfg := testVirtTaskConfig(t, filepath.Join(dir, "images"))
		task := testTaskConfig()
		must.NoError(t, task.EncodeConcreteDriverConfig(virtcfg))
		vmName := vmNameFromTaskID(task.ID)

		// Create all the needed mocks
		ih := &mock_image_tools.StaticImageHandler{GetImageFormatResult: "tif"}
		st := mock_storage.NewMockStorage(t)
		defer st.AssertExpectations()
		pl := mock_storage.NewMockPool(t)
		defer pl.AssertExpectations()
		vt := mock_virt.NewMock(t)
		defer vt.AssertExpectations()
		pv := mock_providers.NewStatic(vt)
		ci := mock_cloudinit.NewStaticCloudInit()

		driverCfg := driverConfig(dir)
		// Load initialization expectations
		vt.Expect(
			mock_virt.Init{},
			mock_virt.SetupStorage{Config: driverCfg.StoragePools},
			mock_virt.SetupStorage{Config: driverCfg.StoragePools},
			mock_virt.Networking{Result: mock_virt_net.NewStatic()},
			mock_virt.Networking{Result: mock_virt_net.NewStatic()},
			mock_virt.GenerateMountCommands{
				Result: []string{
					"mkdir -p /alloc",
					"mountpoint -q /alloc || mount -t 9p -o trans=virtio allocDir /alloc",
					"mkdir -p /local",
					"mountpoint -q /local || mount -t 9p -o trans=virtio localDir /local",
					"mkdir -p /secrets",
					"mountpoint -q /secrets || mount -t 9p -o trans=virtio secretsDir /secrets",
				},
			},
		)

		// Build the test driver and create the alloc directory
		driver := testHarness(t, driverCfg, pv, ci, task, 1*time.Second)

		// Set all the expectations for the mocks
		st.Expect(
			mock_storage.ImageHandler{Result: ih},
			mock_storage.DefaultPool{Result: pl},
			mock_storage.DefaultPool{Result: pl},
			mock_storage.DefaultPool{Result: pl},
			mock_storage.GenerateDeviceName{BusType: "virtio", ExistingDevices: []string{}, Result: "sda"},
			mock_storage.DefaultDiskDriver{Result: "test-driver"},
		)

		vt.Expect(
			mock_virt.Init{},
			mock_virt.UseCloudInit{Result: false},
			mock_virt.Storage{Result: st},
			mock_virt.Storage{Result: st},
			mock_virt.Storage{Result: st},
			mock_virt.Networking{Result: mock_virt_net.NewStatic()},
			mock_virt.CreateVM{
				Config: &vm.Config{
					RemoveConfigFiles: true,
					Name:              vmName,
					Memory:            6000,
					CPUset:            "1,2,3",
					CPUs:              3,
					OsVariant:         &vm.OSVariant{Arch: testOsArch, Machine: testOsMachine},
					HostName:          "nomad-" + vmName,
					Mounts: []vm.MountFileConfig{
						{
							Source:      filepath.Join(task.AllocDir, "alloc"),
							Destination: "/alloc",
							ReadOnly:    true,
							Tag:         "allocDir",
						},
						{
							Source:      filepath.Join(task.AllocDir, "local"),
							Destination: "/local",
							ReadOnly:    true,
							Tag:         "localDir",
						},
						{
							Source:      filepath.Join(task.AllocDir, "secrets"),
							Destination: "/secrets",
							ReadOnly:    true,
							Tag:         "secretsDir",
						},
					},
					Files: []vm.File{
						{
							Path:        "/etc/profile.d/virt.sh",
							Permissions: "777",
							Encoding:    "b64",
						},
					},
					SSHKey:   "ssh-ed666 randomkey",
					Password: "password",
					CMDs:     []string{"cmd arg arg", "cmd arg arg"},
					BOOTCMDs: []string{
						"mkdir -p /alloc",
						"mountpoint -q /alloc || mount -t 9p -o trans=virtio allocDir /alloc",
						"mkdir -p /local",
						"mountpoint -q /local || mount -t 9p -o trans=virtio localDir /local",
						"mkdir -p /secrets",
						"mountpoint -q /secrets || mount -t 9p -o trans=virtio secretsDir /secrets",
					},
					CIUserData: "/path/to/user/data",
					Volumes: []storage.Volume{
						{
							Kind:       "disk",
							Driver:     "test-driver",
							Format:     "tif",
							DeviceName: "sda",
							BusType:    "virtio",
							Primary:    true,
						},
					},
				},
			},
			mock_virt.GetNetworkInterfaces{Name: vmName},
			mock_virt.Networking{Result: mock_virt_net.NewStatic()},
			mock_virt.DestroyVM{Name: vmName},
			// GetVM is used for stats, and they should be collected twice
			mock_virt.GetVM{Name: vmName, Result: &vm.Info{State: vm.VMStateRunning}},
		)

		pl.Expect(
			mock_storage.DefaultImageFormat{Result: "tif"},
			mock_storage.AddVolume{
				Name: vmName + "_sda.img",
				Opts: storage.Options{
					Size: 50000000,
					Target: storage.Target{
						Format: "tif",
					},
					Source: storage.Source{
						Path: virtcfg.Disks[0].Source.Image,
					},
				},
				Result: &storage.Volume{},
			},
		)

		// start the task
		taskHandle, _, err := driver.StartTask(task)
		must.NoError(t, err)
		must.One(t, taskHandle.Version)

		// inspect the task to verify running state
		ts, err := driver.InspectTask(task.ID)
		must.NoError(t, err)
		must.Eq(t, drivers.TaskStateRunning, ts.State)
		must.StrContains(t, task.ID, ts.ID)

		// create a new driver plugin
		driver = testHarness(t, driverCfg, pv, ci, task, 1*time.Second)
		err = driver.RecoverTask(taskHandle)
		must.NoError(t, err)

		// force destroy the task
		must.NoError(t, driver.DestroyTask(task.ID, true))
	})

	t.Run("start wait crashed", func(t *testing.T) {
		dir := t.TempDir()
		virtcfg := testVirtTaskConfig(t, filepath.Join(dir, "images"))
		task := testTaskConfig()
		must.NoError(t, task.EncodeConcreteDriverConfig(virtcfg))
		vmName := vmNameFromTaskID(task.ID)

		// Create all the needed mocks
		ih := &mock_image_tools.StaticImageHandler{GetImageFormatResult: "tif"}
		st := mock_storage.NewMockStorage(t)
		defer st.AssertExpectations()
		pl := mock_storage.NewMockPool(t)
		defer pl.AssertExpectations()
		vt := mock_virt.NewMock(t)
		defer vt.AssertExpectations()
		pv := mock_providers.NewStatic(vt)
		ci := mock_cloudinit.NewStaticCloudInit()

		driverCfg := driverConfig(dir)
		// Load initialization expectations
		vt.Expect(
			mock_virt.Init{},
			mock_virt.SetupStorage{Config: driverCfg.StoragePools},
			mock_virt.Networking{Result: mock_virt_net.NewStatic()},
			mock_virt.GenerateMountCommands{
				Result: []string{
					"mkdir -p /alloc",
					"mountpoint -q /alloc || mount -t 9p -o trans=virtio allocDir /alloc",
					"mkdir -p /local",
					"mountpoint -q /local || mount -t 9p -o trans=virtio localDir /local",
					"mkdir -p /secrets",
					"mountpoint -q /secrets || mount -t 9p -o trans=virtio secretsDir /secrets",
				},
			},
		)

		// Build the test driver and create the alloc directory
		driver := testHarness(t, driverCfg, pv, ci, task, 5*time.Second)

		// Set all the expectations for the mocks
		st.Expect(
			mock_storage.ImageHandler{Result: ih},
			mock_storage.DefaultPool{Result: pl},
			mock_storage.DefaultPool{Result: pl},
			mock_storage.DefaultPool{Result: pl},
			mock_storage.GenerateDeviceName{BusType: "virtio", ExistingDevices: []string{}, Result: "sda"},
			mock_storage.DefaultDiskDriver{Result: "test-driver"},
		)

		vt.Expect(
			mock_virt.UseCloudInit{Result: false},
			mock_virt.Storage{Result: st},
			mock_virt.Storage{Result: st},
			mock_virt.Storage{Result: st},
			mock_virt.Networking{Result: mock_virt_net.NewStatic()},
			mock_virt.CreateVM{
				Config: &vm.Config{
					RemoveConfigFiles: true,
					Name:              vmName,
					Memory:            6000,
					CPUset:            "1,2,3",
					CPUs:              3,
					OsVariant:         &vm.OSVariant{Arch: testOsArch, Machine: testOsMachine},
					HostName:          "nomad-" + vmName,
					Mounts: []vm.MountFileConfig{
						{
							Source:      filepath.Join(task.AllocDir, "alloc"),
							Destination: "/alloc",
							ReadOnly:    true,
							Tag:         "allocDir",
						},
						{
							Source:      filepath.Join(task.AllocDir, "local"),
							Destination: "/local",
							ReadOnly:    true,
							Tag:         "localDir",
						},
						{
							Source:      filepath.Join(task.AllocDir, "secrets"),
							Destination: "/secrets",
							ReadOnly:    true,
							Tag:         "secretsDir",
						},
					},
					Files: []vm.File{
						{
							Path:        "/etc/profile.d/virt.sh",
							Permissions: "777",
							Encoding:    "b64",
						},
					},
					SSHKey:   "ssh-ed666 randomkey",
					Password: "password",
					CMDs:     []string{"cmd arg arg", "cmd arg arg"},
					BOOTCMDs: []string{
						"mkdir -p /alloc",
						"mountpoint -q /alloc || mount -t 9p -o trans=virtio allocDir /alloc",
						"mkdir -p /local",
						"mountpoint -q /local || mount -t 9p -o trans=virtio localDir /local",
						"mkdir -p /secrets",
						"mountpoint -q /secrets || mount -t 9p -o trans=virtio secretsDir /secrets",
					},
					CIUserData: "/path/to/user/data",
					Volumes: []storage.Volume{
						{
							Kind:       "disk",
							Driver:     "test-driver",
							Format:     "tif",
							DeviceName: "sda",
							BusType:    "virtio",
							Primary:    true,
						},
					},
				},
			},
			mock_virt.GetNetworkInterfaces{Name: vmName},
			// GetVM is used for stats, and they should be collected twice
			mock_virt.GetVM{Name: vmName, Result: &vm.Info{State: vm.VMStateError}},
		)

		pl.Expect(
			mock_storage.DefaultImageFormat{Result: "tif"},
			mock_storage.AddVolume{
				Name: vmName + "_sda.img",
				Opts: storage.Options{
					Size: 50000000,
					Target: storage.Target{
						Format: "tif",
					},
					Source: storage.Source{
						Path: virtcfg.Disks[0].Source.Image,
					},
				},
				Result: &storage.Volume{},
			},
		)

		// start the task
		taskHandle, _, err := driver.StartTask(task)
		must.NoError(t, err)
		must.One(t, taskHandle.Version)

		waitCh, err := driver.WaitTask(t.Context(), task.ID)
		must.NoError(t, err)

		// wait long enough for stats to get pulled twice
		select {
		case exitResult := <-waitCh:
			must.Eq(t, 1, exitResult.ExitCode)
			must.ErrorContains(t, exitResult.Err, "task has crashed")
		case <-time.After(2 * time.Second):
			t.Fatal("wait channel did not receive exit result")

		}

		// inspect the task to verify running state
		ts, err := driver.InspectTask(task.ID)
		must.NoError(t, err)
		must.StrContains(t, task.ID, ts.ID)
		must.Eq(t, 1, ts.ExitResult.ExitCode)
		must.Eq(t, "exited", ts.State)
	})

	t.Run("start fail cleanup", func(t *testing.T) {
		testErr := errors.New("testing error")

		dir := t.TempDir()
		virtcfg := testVirtTaskConfig(t, filepath.Join(dir, "images"))
		task := testTaskConfig()
		must.NoError(t, task.EncodeConcreteDriverConfig(virtcfg))
		vmName := vmNameFromTaskID(task.ID)

		// Create all the needed mocks
		ih := &mock_image_tools.StaticImageHandler{GetImageFormatResult: "tif"}
		st := mock_storage.NewMockStorage(t)
		defer st.AssertExpectations()
		pl := mock_storage.NewMockPool(t)
		defer pl.AssertExpectations()
		vt := mock_virt.NewMock(t)
		defer vt.AssertExpectations()
		pv := mock_providers.NewStatic(vt)
		ci := mock_cloudinit.NewStaticCloudInit()

		driverCfg := driverConfig(dir)
		// Load initialization expectations
		vt.Expect(
			mock_virt.Init{},
			mock_virt.SetupStorage{Config: driverCfg.StoragePools},
			mock_virt.Networking{Result: mock_virt_net.NewStatic()},
			mock_virt.GenerateMountCommands{
				Result: []string{
					"mkdir -p /alloc",
					"mountpoint -q /alloc || mount -t 9p -o trans=virtio allocDir /alloc",
					"mkdir -p /local",
					"mountpoint -q /local || mount -t 9p -o trans=virtio localDir /local",
					"mkdir -p /secrets",
					"mountpoint -q /secrets || mount -t 9p -o trans=virtio secretsDir /secrets",
				},
			},
		)

		// Build the test driver and create the alloc directory
		driver := testHarness(t, driverCfg, pv, ci, task, 5*time.Second)

		// Set all the expectations for the mocks
		st.Expect(
			mock_storage.ImageHandler{Result: ih},
			mock_storage.DefaultPool{Result: pl},
			mock_storage.DefaultPool{Result: pl},
			mock_storage.DefaultPool{Result: pl},
			mock_storage.GenerateDeviceName{BusType: "virtio", ExistingDevices: []string{}, Result: "sda"},
			mock_storage.DefaultDiskDriver{Result: "test-driver"},
			mock_storage.GetPool{Name: "default-pool", Result: pl},
		)

		vt.Expect(
			mock_virt.UseCloudInit{Result: false},
			mock_virt.Storage{Result: st},
			mock_virt.Storage{Result: st},
			mock_virt.Storage{Result: st},
			mock_virt.Networking{Result: mock_virt_net.NewStatic()},
			mock_virt.CreateVM{
				Config: &vm.Config{
					RemoveConfigFiles: true,
					Name:              vmName,
					Memory:            6000,
					CPUset:            "1,2,3",
					CPUs:              3,
					OsVariant:         &vm.OSVariant{Arch: testOsArch, Machine: testOsMachine},
					HostName:          "nomad-" + vmName,
					Mounts: []vm.MountFileConfig{
						{
							Source:      filepath.Join(task.AllocDir, "alloc"),
							Destination: "/alloc",
							ReadOnly:    true,
							Tag:         "allocDir",
						},
						{
							Source:      filepath.Join(task.AllocDir, "local"),
							Destination: "/local",
							ReadOnly:    true,
							Tag:         "localDir",
						},
						{
							Source:      filepath.Join(task.AllocDir, "secrets"),
							Destination: "/secrets",
							ReadOnly:    true,
							Tag:         "secretsDir",
						},
					},
					Files: []vm.File{
						{
							Path:        "/etc/profile.d/virt.sh",
							Permissions: "777",
							Encoding:    "b64",
						},
					},
					SSHKey:   "ssh-ed666 randomkey",
					Password: "password",
					CMDs:     []string{"cmd arg arg", "cmd arg arg"},
					BOOTCMDs: []string{
						"mkdir -p /alloc",
						"mountpoint -q /alloc || mount -t 9p -o trans=virtio allocDir /alloc",
						"mkdir -p /local",
						"mountpoint -q /local || mount -t 9p -o trans=virtio localDir /local",
						"mkdir -p /secrets",
						"mountpoint -q /secrets || mount -t 9p -o trans=virtio secretsDir /secrets",
					},
					CIUserData: "/path/to/user/data",
					Volumes: []storage.Volume{
						{
							Pool:       "default-pool",
							Name:       vmName + "_sda.img",
							Kind:       "disk",
							Driver:     "test-driver",
							Format:     "tif",
							DeviceName: "sda",
							BusType:    "virtio",
							Primary:    true,
						},
					},
				},
				Err: testErr,
			},
			mock_virt.Storage{Result: st},
		)

		pl.Expect(
			mock_storage.DefaultImageFormat{Result: "tif"},
			mock_storage.AddVolume{
				Name: vmName + "_sda.img",
				Opts: storage.Options{
					Size: 50000000,
					Target: storage.Target{
						Format: "tif",
					},
					Source: storage.Source{
						Path: virtcfg.Disks[0].Source.Image,
					},
				},
				Result: &storage.Volume{Pool: "default-pool", Name: vmName + "_sda.img"},
			},
			mock_storage.DeleteVolume{Name: vmName + "_sda.img"},
		)

		// start the task
		_, _, err := driver.StartTask(task)
		must.ErrorContains(t, err, testErr.Error())
	})
}

func TestVirtDriver_Libvirt(t *testing.T) {
	ci.Parallel(t)

	dir := t.TempDir()
	virtcfg := testVirtTaskConfig(t, filepath.Join(dir, "images"))
	task := testTaskConfig()

	f, err := os.Create(modulesPath(t))
	must.NoError(t, err)
	f.WriteString("9pnet_virtio")
	f.Close()

	// NOTE: Setting size to zero to prevent a volume resize
	// which the libvirt test driver doesn't support.
	virtcfg.Disks[0].Size = "0"
	// NOTE: Setting format to qcow2 to prevent qemu-img from
	// being run on the empty source file.
	virtcfg.Disks[0].Source.Format = "qcow2"
	virtcfg.OS = &virt.OS{
		Arch:    "x86_64",
		Machine: "pc-i440fx-jammy",
	}

	must.NoError(t, task.EncodeConcreteDriverConfig(virtcfg))
	vmName := vmNameFromTaskID(task.ID)

	libvirtProvider := libvirt.New(t.Context(), hclog.NewNullLogger(),
		libvirt.WithConnectionURI(libvirt.TestURI))

	cloudinitMock := mock_cloudinit.NewStaticCloudInit()

	// Generate the configuration and add the libvirt provider to create
	// a real providers instance for the harness.
	config := driverConfig(dir)
	config.Provider = &virt.Provider{
		Libvirt: &libvirt.Config{
			URI: libvirt.TestURI,
		},
	}
	logger := testlog.HCLogger(t)
	if testing.Verbose() {
		logger.SetLevel(hclog.Trace)
	} else {
		logger.SetLevel(hclog.Info)
	}
	prv := providers.New(t.Context(), logger)
	driver := testHarness(t, config, prv, cloudinitMock, task, 5*time.Second)

	// Stub the cloudinit generated file
	f, err = os.Create(filepath.Join(task.AllocDir, "cloudinit.iso"))
	must.NoError(t, err)
	f.Close()

	// Start the task
	taskHandle, _, err := driver.StartTask(task)
	must.NoError(t, err)
	must.One(t, taskHandle.Version)

	// Verify that the vm "exists"
	testVm, err := libvirtProvider.GetVM(vmName)
	must.NoError(t, err)
	must.Eq(t, vm.VMStateRunning, testVm.State)

	// Attempt to wait and collect stats
	waitCh, err := driver.WaitTask(t.Context(), task.ID)
	must.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	statsChan, err := driver.TaskStats(ctx, task.ID, 1*time.Second)
	must.NoError(t, err)

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-statsChan:
		case <-time.After(3 * time.Second):
			t.Error("no stats received")
		}
	}()

	select {
	case <-waitCh:
		t.Fatal("wait channel received unexpected exit result")
	case <-time.After(4 * time.Second):
	}

	// Inspect the task
	ts, err := driver.InspectTask(task.ID)
	must.NoError(t, err)
	must.Eq(t, drivers.TaskStateRunning, ts.State)
	must.StrContains(t, task.ID, ts.ID)

	// Destroy the task
	err = driver.DestroyTask(task.ID, true)
	must.NoError(t, err)

	// Validate VM no longer exists
	_, err = libvirtProvider.GetVM(vmName)
	must.ErrorIs(t, err, errs.ErrNotFound)

	// Get fingerprint information
	fCh, err := driver.Fingerprint(t.Context())
	must.NoError(t, err)
	var print *drivers.Fingerprint
	select {
	case print = <-fCh:
	case <-time.After(1 * time.Second):
		t.Fatal("no fingerprint received")
	}

	// Don't match fingerprint exactly since local configurations
	// may be different. Start with some virt information
	must.True(t, *print.Attributes["driver.virt"].Bool)
	must.True(t, *print.Attributes["driver.virt.provider.libvirt"].Bool)
	must.Eq(t, "TEST", *print.Attributes["driver.virt.provider.libvirt.driver"].String)
	// Check that storage pools are included
	must.Eq(t, "directory", *print.Attributes["driver.virt.storage_pool.default-pool"].String)
	must.True(t, *print.Attributes["driver.virt.storage_pool.default-pool.default"].Bool)
	must.Eq(t, "libvirt", *print.Attributes["driver.virt.storage_pool.default-pool.provider"].String)
}

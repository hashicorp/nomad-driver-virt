// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/shoenig/test/must"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

// NOTE: The libvirt test driver is memory backed per-process so tests are
// not isolated

// validate the driver implements the connect interface
var _ shims.Connect = (*provider)(nil)

type libvirtModifier func(l *provider)

func overrideFs(avail ...string) libvirtModifier {
	return func(l *provider) {
		m := map[string]struct{}{}
		for _, a := range avail {
			m[a] = struct{}{}
		}
		l.availableMountFsOverride = m
	}
}

func testNew(t *testing.T, modifiers ...libvirtModifier) (*provider, string) {
	t.Helper()
	poolName := strings.ReplaceAll(t.Name(), "/", "_")

	l := New(
		context.Background(),
		hclog.NewNullLogger(),
		WithConnectionURI(TestURI),
	)
	t.Cleanup(func() { l.Close() })
	for _, m := range modifiers {
		m(l)
	}
	must.NoError(t, l.Init())
	must.NoError(t, l.SetupStorage(&storage.Config{
		Directory: map[string]storage.Directory{
			poolName: {Path: t.TempDir()},
		},
	}))
	return l, poolName
}

func vmName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test-vm-%s", uuid.Generate()[:8])
}

func TestStorage(t *testing.T) {
	t.Parallel()

	mkPoolDir := func(t *testing.T) (string, string) {
		t.Helper()
		name := fmt.Sprintf("pools-%s", uuid.Generate()[:8])
		return name, filepath.Join(t.TempDir(), name)
	}

	t.Run("Setup", func(t *testing.T) {
		poolName, poolDir := mkPoolDir(t)
		mainName := fmt.Sprintf("%s-%s", poolName, "main-pool")
		secondName := fmt.Sprintf("%s-%s", poolName, "secondary-pool")
		l, _ := testNew(t, overrideFs(mountFs9p))
		pools := &storage.Config{
			Default: mainName,
			Directory: map[string]storage.Directory{
				mainName: {
					Path: filepath.Join(poolDir, "main-pool"),
				},
				secondName: {
					Path: filepath.Join(poolDir, "secondary-pool"),
				},
			},
		}

		must.NoError(t, l.SetupStorage(pools))
		// Check for expected pools
		main, err := l.storage.GetPool(mainName)
		must.NoError(t, err)
		must.Eq(t, mainName, main.Name())
		second, err := l.storage.GetPool(secondName)
		must.NoError(t, err)
		must.Eq(t, secondName, second.Name())
		// Check that the backing directories where created
		must.DirExists(t, filepath.Join(poolDir, "main-pool"))
		must.DirExists(t, filepath.Join(poolDir, "secondary-pool"))
		// Check that the default pool is correct
		defPool, err := l.storage.DefaultPool()
		must.NoError(t, err)
		must.Eq(t, main, defPool)
	})

	// NOTE: The libvirt test endpoint does not support uploading to
	// volumes so we can't test sources.
	t.Run("Volumes", func(t *testing.T) {
		t.Run("create-retrieve-delete", func(t *testing.T) {
			poolName, poolDir := mkPoolDir(t)
			l, _ := testNew(t, overrideFs(mountFs9p))
			pools := &storage.Config{
				Directory: map[string]storage.Directory{
					poolName: {Path: filepath.Join(poolDir, "pool")}}}
			must.NoError(t, l.SetupStorage(pools))
			pool, err := l.storage.DefaultPool()
			must.NoError(t, err)

			// Add an empty volume
			v, err := pool.AddVolume("test-vol", storage.Options{Size: 1024, Target: storage.Target{Format: "raw"}})
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: "test-vol", Pool: poolName, Format: "raw", Kind: "file", Size: 1024}, v)

			// Check that format is defaulted if unset
			vf, err := pool.AddVolume("test-vol-2", storage.Options{})
			must.NoError(t, err)
			must.Eq(t, "qcow2", vf.Format)

			// Get the volume
			getV, err := pool.GetVolume("test-vol")
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: "test-vol", Pool: poolName, Kind: "file", Format: "raw", Size: 1024}, getV)

			// Delete the volume
			must.NoError(t, pool.DeleteVolume("test-vol"))

			// Check that the volume does not exist
			_, err = pool.GetVolume("test-vol")
			must.ErrorIs(t, err, vm.ErrNotFound)
		})
	})
}

func TestGetInfo(t *testing.T) {
	t.Parallel()

	ld, _ := testNew(t, overrideFs(mountFs9p))

	i, err := ld.GetInfo()
	must.NoError(t, err)

	must.NonZero(t, i.LibvirtVersion)
	must.NonZero(t, i.EmulatorVersion)
	must.NonZero(t, i.StoragePools)
	// The test driver has at least one running machine.
	must.Greater(t, 0, i.RunningDomains)
}

func TestStartDomain(t *testing.T) {
	t.Parallel()

	makeConfig := func(poolName string) *vm.Config {
		return &vm.Config{
			Memory:   66600,
			CPUs:     2,
			HostName: "test-hostname",
			SSHKey:   "sshkey lkbfubwfu...",
			Password: "test-password",
			CMDs:     []string{"cmd arg arg", "cmd arg arg"},
			BOOTCMDs: []string{"cmd arg arg", "cmd arg arg"},
			Mounts: []vm.MountFileConfig{
				{
					Source:      "/mount/source/one",
					Destination: "/path/to/file/one",
					Tag:         "tagOne",
					ReadOnly:    true,
				},
				{Source: "/mount/source/two",
					Destination: "/path/to/file/two",
					Tag:         "tagTwo",
					ReadOnly:    false},
			},
			Files: []vm.File{
				{
					Path:        "/path/to/file/one",
					Content:     "content",
					Permissions: "444",
					Encoding:    "b64",
				},
				{
					Path:        "/path/to/file/two",
					Content:     "content",
					Permissions: "666",
				},
			},
			Volumes: []storage.Volume{
				{
					Pool:       poolName,
					Name:       "vol-name",
					Kind:       "disk",
					Driver:     "qemu",
					Format:     "qcow2",
					DeviceName: "sda",
					BusType:    "sata",
				},
			},
		}

	}

	t.Run("domain created successfully", func(t *testing.T) {
		ld, poolName := testNew(t, overrideFs(mountFs9p))

		domConfig := makeConfig(poolName)
		domConfig.Name = vmName(t)

		must.NoError(t, ld.CreateVM(domConfig))

		dom, err := ld.getDomain(domConfig.Name)
		must.NoError(t, err)
		state, _, err := dom.GetState()
		must.Eq(t, libvirt.DOMAIN_RUNNING, state)
	})

	t.Run("duplicated domain error", func(t *testing.T) {
		ld, poolName := testNew(t, overrideFs(mountFs9p))

		domConfig := makeConfig(poolName)
		domConfig.Name = vmName(t)
		must.NoError(t, ld.CreateVM(domConfig))

		// try again
		err := ld.CreateVM(domConfig)
		must.ErrorIs(t, err, ErrDomainExists)
	})

	t.Run("includes volume information", func(t *testing.T) {
		ld, poolName := testNew(t, overrideFs(mountFs9p))

		domConfig := makeConfig(poolName)
		domConfig.Name = vmName(t)
		must.NoError(t, ld.CreateVM(domConfig))

		dom, err := ld.getDomain(domConfig.Name)
		must.NoError(t, err)
		descXml, err := dom.GetXMLDesc(0)
		must.NoError(t, err)
		desc := &libvirtxml.Domain{}
		must.NoError(t, desc.Unmarshal(descXml))
		must.SliceLen(t, 1, desc.Devices.Disks, must.Sprint("expecting only one disk"))
		must.Eq(t, "disk", desc.Devices.Disks[0].Device)
		must.Eq(t, "sda", desc.Devices.Disks[0].Target.Dev)
	})

	t.Run("includes additional volumes", func(t *testing.T) {
		ld, poolName := testNew(t, overrideFs(mountFs9p))

		domConfig := makeConfig(poolName)
		domConfig.Name = vmName(t)
		domConfig.Volumes = append(domConfig.Volumes, storage.Volume{
			Pool:       poolName,
			Name:       "vol-name",
			Kind:       "cdrom",
			Driver:     "qemu",
			Format:     "raw",
			DeviceName: "hda",
			BusType:    "ide",
		})
		must.NoError(t, ld.CreateVM(domConfig))

		dom, err := ld.getDomain(domConfig.Name)
		must.NoError(t, err)
		descXml, err := dom.GetXMLDesc(0)
		must.NoError(t, err)
		desc := &libvirtxml.Domain{}
		must.NoError(t, desc.Unmarshal(descXml))
		must.SliceLen(t, 2, desc.Devices.Disks, must.Sprint("expecting two disks"))
		must.Eq(t, "disk", desc.Devices.Disks[0].Device)
		must.Eq(t, "sda", desc.Devices.Disks[0].Target.Dev)
		must.Eq(t, "cdrom", desc.Devices.Disks[1].Device)
		must.Eq(t, "hda", desc.Devices.Disks[1].Target.Dev)
	})
}

func Test_CreateStopAndDestroyDomain(t *testing.T) {
	t.Parallel()

	ld, _ := testNew(t, overrideFs(mountFs9p))

	domainName := vmName(t)
	err := ld.CreateVM(&vm.Config{
		RemoveConfigFiles: true,
		Name:              domainName,
		Memory:            66600,
		CPUs:              6,
	})
	must.NoError(t, err)

	dom, err := ld.getDomain(domainName)
	must.NoError(t, err)
	defer dom.Free()

	state, _, err := dom.GetState()
	must.NoError(t, err)
	must.Eq(t, libvirt.DOMAIN_RUNNING, state)

	err = ld.StopVM(domainName)
	must.NoError(t, err)

	state, _, err = dom.GetState()
	must.NoError(t, err)
	must.Eq(t, libvirt.DOMAIN_SHUTOFF, state)

	err = ld.DestroyVM(domainName)
	must.NoError(t, err)

	state, _, err = dom.GetState()
	must.ErrorIs(t, err, libvirt.ERR_NO_DOMAIN)
}

func Test_GetNetworkInterfaces(t *testing.T) {
	t.Parallel()

	ld, _ := testNew(t, overrideFs(mountFs9p))
	domainName := vmName(t)
	err := ld.CreateVM(&vm.Config{
		RemoveConfigFiles: true,
		Name:              domainName,
		Memory:            66600,
		CPUs:              6,
		NetworkInterfaces: []*net.NetworkInterfaceConfig{
			{
				Bridge: &net.NetworkInterfaceBridgeConfig{
					Name: "testbr0",
				},
			},
		},
	})
	must.NoError(t, err)

	interfaces, err := ld.GetNetworkInterfaces(domainName)
	must.NoError(t, err)
	must.Len(t, 1, interfaces)
	must.StrContains(t, interfaces[0].MAC, ":")
}

func Test_GenerateMountCommands(t *testing.T) {
	t.Parallel()
	mounts := func() []*vm.MountFileConfig {
		return []*vm.MountFileConfig{
			{
				Source:      "/dev/null",
				Destination: "/test",
				ReadOnly:    false,
				Tag:         "test-tag",
			},
		}
	}

	t.Run("not available", func(t *testing.T) {
		ld, _ := testNew(t, overrideFs())
		_, err := ld.GenerateMountCommands(mounts())
		must.ErrorIs(t, err, vm.ErrNotSupported)
	})

	t.Run("9p available", func(t *testing.T) {
		ld, _ := testNew(t, overrideFs(mountFs9p))

		mnts := mounts()
		result, err := ld.GenerateMountCommands(mnts)
		must.NoError(t, err)
		must.Eq(t, []string{
			`mkdir -p "/test"`,
			`mountpoint -q "/test" || mount -t 9p -o trans=virtio test-tag "/test"`,
		}, result)
		must.Eq(t, mountFs9p, mnts[0].Driver)
	})

	t.Run("virtiofs available", func(t *testing.T) {
		ld, _ := testNew(t, overrideFs(mountFsVirtiofs))

		mnts := mounts()
		result, err := ld.GenerateMountCommands(mnts)
		must.NoError(t, err)
		must.Eq(t, []string{
			`mkdir -p "/test"`,
			`mountpoint -q "/test" || mount -t virtiofs test-tag "/test"`,
		}, result)
		must.Eq(t, mountFsVirtiofs, mnts[0].Driver)
	})

	t.Run("virtiofs and 9p available", func(t *testing.T) {
		ld, _ := testNew(t, overrideFs(mountFs9p, mountFsVirtiofs))

		result, err := ld.GenerateMountCommands(mounts())
		must.NoError(t, err)
		must.Eq(t, []string{
			`mkdir -p "/test"`,
			`mountpoint -q "/test" || mount -t virtiofs test-tag "/test"`,
		}, result)
	})

	t.Run("read-only mount without virtiofs support", func(t *testing.T) {
		// Make mount read-only
		mnts := mounts()
		mnts[0].ReadOnly = true

		t.Run("9p and virtiofs available", func(t *testing.T) {
			ld, _ := testNew(t, overrideFs(mountFs9p, mountFsVirtiofs))
			// Force low version of libvirt
			ld.libvirtVersion = 1

			result, err := ld.GenerateMountCommands(mnts)
			must.NoError(t, err)
			must.Eq(t, []string{
				`mkdir -p "/test"`,
				`mountpoint -q "/test" || mount -t 9p -o trans=virtio test-tag "/test"`,
			}, result)
		})

		t.Run("virtiofs only available", func(t *testing.T) {
			ld, _ := testNew(t, overrideFs(mountFsVirtiofs))
			// Force low version of libvirt
			ld.libvirtVersion = 1

			_, err := ld.GenerateMountCommands(mnts)
			must.ErrorIs(t, err, vm.ErrNotSupported)
		})
	})
}

// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"testing"

	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	"github.com/shoenig/test/must"
	"libvirt.org/go/libvirtxml"
)

const (
	// Executable that outputs qemu device output with virtio
	fsbinWithVirtio = "./testdata/fs-virtio/fs-virtio"
	// Executable that outputs qemu device output with virtio and 9p
	fsbinWithVirtio9p = "./testdata/fs-virtio-9p/fs-virtio-9p"
	// Executable that returns an error
	fsbinError = "./testdata/fs-err/fs-err"
)

func TestCapabilities_Guest(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		arch := "testArch"
		caps := &Capabilities{
			Host: &libvirtxml.CapsHost{},
			Guests: map[string]*CapsGuest{
				arch: {
					CapsGuest: &libvirtxml.CapsGuest{
						Arch: libvirtxml.CapsGuestArch{
							Name: arch,
						},
					},
				},
			},
		}

		result, err := caps.Guest(arch)
		must.NoError(t, err)
		must.Eq(t, caps.Guests[arch], result)
	})

	t.Run("not found", func(t *testing.T) {
		arch := "testArch"
		caps := &Capabilities{
			Host:   &libvirtxml.CapsHost{},
			Guests: map[string]*CapsGuest{},
		}

		result, err := caps.Guest(arch)
		must.ErrorIs(t, err, errs.ErrNotFound)
		must.Nil(t, result)
	})
}

func TestCapsGuest_LoadMountFilesystems(t *testing.T) {
	// Check that executables are present.
	for _, path := range []string{fsbinWithVirtio, fsbinWithVirtio9p, fsbinError} {
		must.FileExists(t, path, must.Sprintf("missing required test executable. run: `make test-bins`"))
	}

	t.Run("virtio only", func(t *testing.T) {
		cap := &CapsGuest{
			CapsGuest: &libvirtxml.CapsGuest{
				Arch: libvirtxml.CapsGuestArch{
					Emulator: fsbinWithVirtio,
				},
			},
		}

		must.NoError(t, cap.LoadMountFilesystems())
		must.True(t, cap.MountFilesystems.Contains(MountFsVirtiofs), must.Sprint("expected virtiofs to be available"))
		must.False(t, cap.MountFilesystems.Contains(MountFs9p), must.Sprint("9p should not be available"))
	})

	t.Run("virtio and 9p", func(t *testing.T) {
		cap := &CapsGuest{
			CapsGuest: &libvirtxml.CapsGuest{
				Arch: libvirtxml.CapsGuestArch{
					Emulator: fsbinWithVirtio9p,
				},
			},
		}

		must.NoError(t, cap.LoadMountFilesystems())
		must.True(t, cap.MountFilesystems.Contains(MountFsVirtiofs), must.Sprint("expected virtiofs to be available"))
		must.True(t, cap.MountFilesystems.Contains(MountFs9p), must.Sprint("expected 9p to be available"))
	})

	t.Run("error", func(t *testing.T) {
		cap := &CapsGuest{
			CapsGuest: &libvirtxml.CapsGuest{
				Arch: libvirtxml.CapsGuestArch{
					Emulator: fsbinError,
				},
			},
		}

		err := cap.LoadMountFilesystems()
		must.ErrorContains(t, err, "failed to inspect emulator devices")
	})
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/storage"
	mock_libvirt "github.com/hashicorp/nomad-driver-virt/testutil/mock/providers/libvirt"
	mock_storage "github.com/hashicorp/nomad-driver-virt/testutil/mock/providers/libvirt/storage"
	"github.com/shoenig/test/must"
	"libvirt.org/go/libvirtxml"
)

func TestStorage_New(t *testing.T) {
	t.Parallel()

	mkconfig := func(dir string) *storage.Config {
		return &storage.Config{
			Directory: map[string]storage.Directory{
				"main-pool": {
					Path:    filepath.Join(dir, "main-pool"),
					Default: true,
				},
				"aux-pool": {
					Path: filepath.Join(dir, "aux-pool"),
				},
			},
		}
	}

	t.Run("creates pools", func(t *testing.T) {
		poolDir := t.TempDir()
		staticPool := mock_storage.NewStaticStoragePool()
		l := mock_libvirt.NewMockLibvirt(t)
		l.Expect(
			mock_libvirt.FindStoragePool{Name: "main-pool", Err: ErrPoolNotFound},
			mock_libvirt.CreateStoragePool{
				Desc: libvirtxml.StoragePool{
					Name: "main-pool",
					Target: &libvirtxml.StoragePoolTarget{
						Path: filepath.Join(poolDir, "main-pool")},
					Type: "dir",
				},
				Result: staticPool,
			},
			mock_libvirt.FindStoragePool{Name: "aux-pool", Err: ErrPoolNotFound},
			mock_libvirt.CreateStoragePool{
				Desc: libvirtxml.StoragePool{
					Name: "aux-pool",
					Target: &libvirtxml.StoragePoolTarget{
						Path: filepath.Join(poolDir, "aux-pool")},
					Type: "dir",
				},
				Result: staticPool,
			},
		)

		s, err := New(hclog.NewNullLogger(), l, mkconfig(poolDir))
		must.NoError(t, err)
		// Should have two pools
		must.MapLen(t, 2, s.pools, must.Sprint("expected number of pools"))
		// Both pools are available
		mainPool, err := s.GetPool("main-pool")
		must.NoError(t, err)
		must.NotNil(t, mainPool)
		auxPool, err := s.GetPool("aux-pool")
		must.NoError(t, err)
		must.NotNil(t, auxPool)
		// Default pool should be main pool
		defaultPool, err := s.DefaultPool()
		must.NoError(t, err)
		must.Eq(t, mainPool, defaultPool)
		// Should create the expected directories
		must.DirExists(t, filepath.Join(poolDir, "main-pool"))
		must.DirExists(t, filepath.Join(poolDir, "aux-pool"))
		// It should free both pools
		must.Eq(t, 2, staticPool.CallCount("Free"), must.Sprint("expected pool Free calls"))
		// Ensure everything was called
		l.AssertExpectations()
	})

	t.Run("creates missing pool", func(t *testing.T) {
		poolDir := t.TempDir()
		l := mock_libvirt.NewMockLibvirt(t)
		l.Expect(
			mock_libvirt.FindStoragePool{Name: "main-pool", Result: mock_storage.NewStaticStoragePool()},
			mock_libvirt.FindStoragePool{Name: "aux-pool", Err: ErrPoolNotFound},
			mock_libvirt.CreateStoragePool{
				Desc: libvirtxml.StoragePool{
					Name: "aux-pool",
					Target: &libvirtxml.StoragePoolTarget{
						Path: filepath.Join(poolDir, "aux-pool")},
					Type: "dir",
				},
				Result: mock_storage.NewStaticStoragePool(),
			},
		)

		s, err := New(hclog.NewNullLogger(), l, mkconfig(poolDir))
		must.NoError(t, err)
		// Should have two pools
		must.MapLen(t, 2, s.pools, must.Sprint("expected number of pools"))
		// Both pools are available
		mainPool, err := s.GetPool("main-pool")
		must.NoError(t, err)
		must.NotNil(t, mainPool)
		auxPool, err := s.GetPool("aux-pool")
		must.NoError(t, err)
		must.NotNil(t, auxPool)
		// Default pool should be main pool
		defaultPool, err := s.DefaultPool()
		must.NoError(t, err)
		must.Eq(t, mainPool, defaultPool)
		// Should create the aux directory since the pool was created
		must.DirExists(t, filepath.Join(poolDir, "aux-pool"))
		// Should not create the main directory since the pool already exists
		must.DirNotExists(t, filepath.Join(poolDir, "main-pool"))

		// Ensure everything was called
		l.AssertExpectations()
	})
}

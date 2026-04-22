// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"errors"
	"path/filepath"
	"plugin"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/storage"
	mock_libvirt "github.com/hashicorp/nomad-driver-virt/testutil/mock/providers/libvirt"
	mock_libvirt_storage "github.com/hashicorp/nomad-driver-virt/testutil/mock/providers/libvirt/storage"
	mock_storage "github.com/hashicorp/nomad-driver-virt/testutil/mock/storage"
	"github.com/hashicorp/nomad/plugins/shared/structs"
	"github.com/shoenig/test/must"
	"libvirt.org/go/libvirtxml"
)

func mkconfig(dir string) *storage.Config {
	return &storage.Config{
		Default: "main-pool",
		Directory: map[string]storage.Directory{
			"main-pool": {
				Path: filepath.Join(dir, "main-pool"),
			},
			"aux-pool": {
				Path: filepath.Join(dir, "aux-pool"),
			},
		},
	}
}

func TestStorage_New(t *testing.T) {
	t.Parallel()

	t.Run("creates directory pools", func(t *testing.T) {
		poolDir := t.TempDir()
		staticPool := mock_libvirt_storage.NewStaticStoragePool()
		l := mock_libvirt.NewMockLibvirt(t)
		l.Expect(
			mock_libvirt.FindStoragePool{Name: "aux-pool", Err: ErrPoolNotFound},
			mock_libvirt.CreateStoragePool{
				Desc: &libvirtxml.StoragePool{
					Name: "aux-pool",
					Target: &libvirtxml.StoragePoolTarget{
						Path: filepath.Join(poolDir, "aux-pool")},
					Type: "dir",
				},
				Result: staticPool,
			},
			mock_libvirt.FindStoragePool{Name: "main-pool", Err: ErrPoolNotFound},
			mock_libvirt.CreateStoragePool{
				Desc: &libvirtxml.StoragePool{
					Name: "main-pool",
					Target: &libvirtxml.StoragePoolTarget{
						Path: filepath.Join(poolDir, "main-pool")},
					Type: "dir",
				},
				Result: staticPool,
			},
		)

		s, err := New(t.Context(), hclog.NewNullLogger(), l, mkconfig(poolDir))
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

	t.Run("creates ceph pools", func(t *testing.T) {
		// Stub the plugin loader to prevent loading the ceph plugin
		pluginLoader = func(string, *plugin.Plugin) error { return nil }
		t.Cleanup(func() { pluginLoader = loadPlugin })
		config := &storage.Config{
			Default: "main-pool",
			Ceph: map[string]storage.Ceph{
				"main-pool": {
					Pool: "ceph-pool",
					Hosts: []string{
						"testing.localhost",
						"test.localhost:8888",
						"127.0.3.33:9933",
						"[2001:db8:85a3:8d3:1319:8a2e:370:7348]:9876",
					},
					Authentication: storage.Authentication{
						Username: "test-user",
						Secret:   "test-secret",
					},
				},
				"aux-pool": {
					Pool: "secondary",
					Hosts: []string{
						"localhost",
					},
					Authentication: storage.Authentication{
						Username: "other-user",
						Secret:   "other-secret",
					},
				},
			},
		}
		staticPool := mock_libvirt_storage.NewStaticStoragePool()
		l := mock_libvirt.NewMockLibvirt(t)
		l.Expect(
			mock_libvirt.FindStoragePool{Name: "aux-pool", Err: ErrPoolNotFound},
			mock_libvirt.SetCephSecret{Name: "aux-pool", Credential: "other-secret", Result: "other-secret-id"},
			mock_libvirt.CreateStoragePool{
				Desc: &libvirtxml.StoragePool{
					Name: "aux-pool",
					Type: "rbd",
					Source: &libvirtxml.StoragePoolSource{
						Name: "secondary",
						Host: []libvirtxml.StoragePoolSourceHost{
							{Name: "localhost"},
						},
						Auth: &libvirtxml.StoragePoolSourceAuth{
							Type:     "ceph",
							Username: "other-user",
							Secret: &libvirtxml.StoragePoolSourceAuthSecret{
								UUID: "other-secret-id",
							},
						},
					},
				},
				Result: staticPool,
			},
			mock_libvirt.FindStoragePool{Name: "main-pool", Err: ErrPoolNotFound},
			mock_libvirt.SetCephSecret{Name: "main-pool", Credential: "test-secret", Result: "secret-id"},
			mock_libvirt.CreateStoragePool{
				Desc: &libvirtxml.StoragePool{
					Name: "main-pool",
					Type: "rbd",
					Source: &libvirtxml.StoragePoolSource{
						Name: "ceph-pool",
						Auth: &libvirtxml.StoragePoolSourceAuth{
							Type:     "ceph",
							Username: "test-user",
							Secret: &libvirtxml.StoragePoolSourceAuthSecret{
								UUID: "secret-id",
							},
						},
						Host: []libvirtxml.StoragePoolSourceHost{
							{Name: "testing.localhost"},
							{Name: "test.localhost", Port: "8888"},
							{Name: "127.0.3.33", Port: "9933"},
							{Name: "2001:db8:85a3:8d3:1319:8a2e:370:7348", Port: "9876"},
						},
					},
				},
				Result: staticPool,
			},
		)

		s, err := New(t.Context(), hclog.NewNullLogger(), l, config)
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
		// It should free both pools
		must.Eq(t, 2, staticPool.CallCount("Free"), must.Sprint("expected pool Free calls"))
		// Ensure everything was called
		l.AssertExpectations()
	})

	t.Run("creates missing pool", func(t *testing.T) {
		poolDir := t.TempDir()
		l := mock_libvirt.NewMockLibvirt(t)
		l.Expect(
			mock_libvirt.FindStoragePool{Name: "main-pool", Result: mock_libvirt_storage.NewStaticStoragePool()},
			mock_libvirt.FindStoragePool{Name: "aux-pool", Err: ErrPoolNotFound},
			mock_libvirt.CreateStoragePool{
				Desc: &libvirtxml.StoragePool{
					Name: "aux-pool",
					Target: &libvirtxml.StoragePoolTarget{
						Path: filepath.Join(poolDir, "aux-pool")},
					Type: "dir",
				},
				Result: mock_libvirt_storage.NewStaticStoragePool(),
			},
		)

		s, err := New(t.Context(), hclog.NewNullLogger(), l, mkconfig(poolDir))
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

func TestStorage_ListPools(t *testing.T) {
	t.Run("multiple pools", func(t *testing.T) {
		l := mock_libvirt.NewStaticLibvirt()
		s, err := New(t.Context(), hclog.NewNullLogger(), l, mkconfig(t.TempDir()))
		must.NoError(t, err)

		must.Eq(t, []string{"aux-pool", "main-pool"}, s.ListPools())
	})

	t.Run("single pool", func(t *testing.T) {
		l := mock_libvirt.NewStaticLibvirt()
		config := mkconfig(t.TempDir())
		delete(config.Directory, "aux-pool")
		s, err := New(t.Context(), hclog.NewNullLogger(), l, config)
		must.NoError(t, err)

		must.Eq(t, []string{"main-pool"}, s.ListPools())
	})
}

func TestStorage_Fingerprint(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		l := mock_libvirt.NewStaticLibvirt()
		s, err := New(t.Context(), hclog.NewNullLogger(), l, mkconfig(t.TempDir()))
		must.NoError(t, err)

		attrs := make(map[string]*structs.Attribute)
		s.Fingerprint(attrs)

		expected := map[string]*structs.Attribute{
			vm.FingerprintAttributeKeyPrefix + ".storage_pool.main-pool":                          structs.NewStringAttribute("directory"),
			vm.FingerprintAttributeKeyPrefix + ".storage_pool.main-pool.provider." + providerName: structs.NewBoolAttribute(true),
			vm.FingerprintAttributeKeyPrefix + ".storage_pool.main-pool.default":                  structs.NewBoolAttribute(true),
			vm.FingerprintAttributeKeyPrefix + ".storage_pool.aux-pool":                           structs.NewStringAttribute("directory"),
			vm.FingerprintAttributeKeyPrefix + ".storage_pool.aux-pool.provider." + providerName:  structs.NewBoolAttribute(true),
		}

		must.Eq(t, expected, attrs)
	})

	t.Run("single pool", func(t *testing.T) {
		l := mock_libvirt.NewStaticLibvirt()
		config := mkconfig(t.TempDir())
		delete(config.Directory, "aux-pool")
		s, err := New(t.Context(), hclog.NewNullLogger(), l, config)
		must.NoError(t, err)

		attrs := make(map[string]*structs.Attribute)
		s.Fingerprint(attrs)

		expected := map[string]*structs.Attribute{
			vm.FingerprintAttributeKeyPrefix + ".storage_pool.main-pool":                          structs.NewStringAttribute("directory"),
			vm.FingerprintAttributeKeyPrefix + ".storage_pool.main-pool.provider." + providerName: structs.NewBoolAttribute(true),
			vm.FingerprintAttributeKeyPrefix + ".storage_pool.main-pool.default":                  structs.NewBoolAttribute(true),
		}

		must.Eq(t, expected, attrs)
	})

	t.Run("no pools", func(t *testing.T) {
		l := mock_libvirt.NewStaticLibvirt()
		s := emptyStorage()
		s.l = l

		attrs := make(map[string]*structs.Attribute)
		s.Fingerprint(attrs)

		expected := make(map[string]*structs.Attribute)
		must.Eq(t, expected, attrs)
	})
}

func TestStorage_VolumeToDisk(t *testing.T) {
	testErr := errors.New("test error")

	testCases := []struct {
		name     string
		volume   storage.Volume
		expected *libvirtxml.DomainDisk
		pool     storage.Pool
		mock     *mock_libvirt.MockLibvirt
		err      error
	}{
		{
			name: "block volume",
			volume: storage.Volume{
				Block:      "/dev/null",
				Kind:       "disk",
				Driver:     "test",
				Format:     "raw",
				DeviceName: "/dev/sda",
				BusType:    "sata",
			},
			expected: &libvirtxml.DomainDisk{
				Device: "disk",
				Driver: &libvirtxml.DomainDiskDriver{
					Name: "test",
					Type: "raw",
				},
				Target: &libvirtxml.DomainDiskTarget{
					Dev: "/dev/sda",
					Bus: "sata",
				},
				Source: &libvirtxml.DomainDiskSource{
					Block: &libvirtxml.DomainDiskSourceBlock{
						Dev: "/dev/null",
					},
				},
			},
		},
		{
			name: "directory pool volume",
			volume: storage.Volume{
				Pool:       "test-pool",
				Name:       "test-vol",
				Kind:       "disk",
				Driver:     "test",
				Format:     "raw",
				DeviceName: "/dev/sda",
				BusType:    "sata",
			},
			expected: &libvirtxml.DomainDisk{
				Device: "disk",
				Driver: &libvirtxml.DomainDiskDriver{
					Name: "test",
					Type: "raw",
				},
				Target: &libvirtxml.DomainDiskTarget{
					Dev: "/dev/sda",
					Bus: "sata",
				},
				Source: &libvirtxml.DomainDiskSource{
					Volume: &libvirtxml.DomainDiskSourceVolume{
						Pool:   "test-pool",
						Volume: "test-vol",
					},
				},
			},
			pool: &mock_storage.StaticPool{
				TypeResult: storage.PoolTypeDirectory,
				NameResult: "test-pool",
			},
		},
		{
			name: "ceph pool volume",
			volume: storage.Volume{
				Pool:       "test-pool",
				Name:       "test-vol",
				Kind:       "disk",
				Driver:     "test",
				Format:     "raw",
				DeviceName: "/dev/sda",
				BusType:    "sata",
			},
			expected: &libvirtxml.DomainDisk{
				Device: "disk",
				Driver: &libvirtxml.DomainDiskDriver{
					Name: "test",
					Type: "raw",
				},
				Target: &libvirtxml.DomainDiskTarget{
					Dev: "/dev/sda",
					Bus: "sata",
				},
				Source: &libvirtxml.DomainDiskSource{
					Network: &libvirtxml.DomainDiskSourceNetwork{
						Protocol: "rbd",
						Name:     "ceph-pool/test-vol",
						Hosts: []libvirtxml.DomainDiskSourceHost{
							{
								Name: "localhost",
								Port: "6789",
							},
						},
						Auth: &libvirtxml.DomainDiskAuth{
							Username: "test-user",
							Secret: &libvirtxml.DomainDiskSecret{
								Type: "ceph",
								UUID: "test-secret",
							},
						},
					},
				},
			},
			pool: &mock_storage.StaticPool{
				TypeResult: storage.PoolTypeCeph,
				NameResult: "test-pool",
			},
			mock: mock_libvirt.NewMockLibvirt(t).Expect(
				mock_libvirt.FindStoragePool{
					Name: "test-pool",
					Result: &mock_libvirt_storage.StaticStoragePool{
						GetXMLDescResult: `<pool><source><name>ceph-pool</name><host name="localhost" port="6789"></host><auth username="test-user"></auth></source></pool>`,
					},
				},
				mock_libvirt.GetCephSecretID{
					Name:   "test-pool",
					Result: "test-secret",
				},
			),
		},
		{
			name: "ceph pool volume - pool lookup error",
			volume: storage.Volume{
				Pool: "test-pool",
				Name: "test-vol",
			},
			pool: &mock_storage.StaticPool{
				TypeResult: storage.PoolTypeCeph,
				NameResult: "test-pool",
			},
			mock: mock_libvirt.NewMockLibvirt(t).Expect(
				mock_libvirt.FindStoragePool{
					Name: "test-pool",
					Err:  testErr,
				},
			),
			err: testErr,
		},
		{
			name: "ceph pool volume - secretlookup error",
			volume: storage.Volume{
				Pool: "test-pool",
				Name: "test-vol",
			},
			pool: &mock_storage.StaticPool{
				TypeResult: storage.PoolTypeCeph,
				NameResult: "test-pool",
			},
			mock: mock_libvirt.NewMockLibvirt(t).Expect(
				mock_libvirt.FindStoragePool{
					Name: "test-pool",
					Result: &mock_libvirt_storage.StaticStoragePool{
						GetXMLDescResult: "<pool></pool>",
					},
				},
				mock_libvirt.GetCephSecretID{
					Name: "test-pool",
					Err:  testErr,
				},
			),
			err: testErr,
		},
		{
			name: "unknown pool type",
			volume: storage.Volume{
				Pool: "test-pool",
				Name: "test-vol",
			},
			pool: &mock_storage.StaticPool{
				TypeResult: "unknown",
				NameResult: "test-pool",
			},
			err: errs.ErrNotImplemented,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var l libvirtStorage
			if tc.mock != nil {
				l = tc.mock
			} else {
				l = mock_libvirt.NewStaticLibvirt()
			}

			s := emptyStorage()
			s.l = l
			if tc.pool != nil {
				s.pools[tc.pool.Name()] = tc.pool
			}

			result, err := s.VolumeToDisk(tc.volume)
			must.Eq(t, tc.expected, result)
			if tc.err != nil {
				must.ErrorIs(t, err, tc.err)
			}
		})
	}
}

func emptyStorage() *Storage {
	return &Storage{
		config: &storage.Config{},
		logger: hclog.NewNullLogger(),
		pools:  make(map[string]storage.Pool),
	}
}

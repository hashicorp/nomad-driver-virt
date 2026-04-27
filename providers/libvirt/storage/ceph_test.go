// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
	"github.com/hashicorp/nomad-driver-virt/storage"
	mock_libvirt "github.com/hashicorp/nomad-driver-virt/testutil/mock/providers/libvirt"
	mock_libvirt_storage "github.com/hashicorp/nomad-driver-virt/testutil/mock/providers/libvirt/storage"
	mock_storage "github.com/hashicorp/nomad-driver-virt/testutil/mock/storage"
	"github.com/hashicorp/nomad-driver-virt/virt/disks"
	"github.com/hashicorp/nomad/helper/pointer"
	"github.com/shoenig/test/must"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

func TestCeph_ValidateDisk(t *testing.T) {
	t.Parallel()
	pool := &ceph{
		pool: &pool{
			name:       "test-pool",
			logger:     hclog.NewNullLogger(),
			l:          mock_libvirt.NewStaticLibvirt(),
			s:          mock_storage.NewStaticStorage(),
			ctx:        t.Context(),
			overwriter: func(shims.StorageVol, string) error { return nil },
		},
	}

	t.Run("format support", func(t *testing.T) {
		t.Run("raw", func(t *testing.T) {
			disk := &disks.Disk{Format: storage.DiskFormatRaw}
			must.NoError(t, pool.ValidateDisk(disk))
		})

		t.Run("qcow2", func(t *testing.T) {
			disk := &disks.Disk{Format: storage.DiskFormatQcow2}
			err := pool.ValidateDisk(disk)
			must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
			must.ErrorContains(t, err, "format can only be raw")
		})
	})

	t.Run("sparse support", func(t *testing.T) {
		t.Run("enabled", func(t *testing.T) {
			disk := &disks.Disk{Format: storage.DiskFormatRaw, Sparse: pointer.Of(true)}
			must.NoError(t, pool.ValidateDisk(disk))
		})

		t.Run("disabled", func(t *testing.T) {
			disk := &disks.Disk{Format: storage.DiskFormatRaw, Sparse: pointer.Of(false)}
			err := pool.ValidateDisk(disk)
			must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
			must.ErrorContains(t, err, "sparse cannot be disabled")
		})
	})
}

func TestCeph_AddVolume(t *testing.T) {
	t.Parallel()

	errTest := errors.New("test error")
	testImageContent := []byte("test content")
	testImageConvertedContent := []byte("test content converted")
	testImagePath := func() string {
		testImageDir := t.TempDir()
		testImagePath := filepath.Join(testImageDir, "testing.img")
		f, err := os.OpenFile(testImagePath, os.O_CREATE|os.O_WRONLY, 0644)
		must.NoError(t, err)
		f.Write(testImageContent)
		must.NoError(t, f.Close())
		testImageConvertedPath := filepath.Join(testImageDir, "testing.img.converted")
		f, err = os.OpenFile(testImageConvertedPath, os.O_CREATE|os.O_WRONLY, 0644)
		must.NoError(t, err)
		f.Write(testImageConvertedContent)
		must.NoError(t, f.Close())
		return testImagePath
	}

	testPoolName := "test-pool"
	mkCephPool := func() *ceph {
		return &ceph{
			pool: &pool{
				name:       testPoolName,
				logger:     hclog.NewNullLogger(),
				l:          mock_libvirt.NewStaticLibvirt(),
				s:          mock_storage.NewStaticStorage(),
				ctx:        t.Context(),
				overwriter: func(shims.StorageVol, string) error { return nil },
			},
		}
	}

	t.Run("errors", func(t *testing.T) {
		t.Run("find storage pool errors", func(t *testing.T) {
			l := mock_libvirt.NewMockLibvirt(t)
			defer l.AssertExpectations()
			l.Expect(mock_libvirt.FindStoragePool{Name: testPoolName, Err: errTest})
			pool := mkCephPool()
			pool.l = l

			_, err := pool.AddVolume(testPoolName, storage.Options{})
			must.Error(t, err)
			must.ErrorIs(t, err, errTest)
		})

		t.Run("failure looking for existing volume", func(t *testing.T) {
			lv := mock_libvirt.NewMockLibvirt(t)
			defer lv.AssertExpectations()
			lvPool := mock_libvirt_storage.NewMockStoragePool(t)
			defer lvPool.AssertExpectations()

			lv.Expect(mock_libvirt.FindStoragePool{Name: testPoolName, Result: lvPool})
			lvPool.Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{
					Name: "test-volume",
					Err:  errTest,
				},
				mock_libvirt_storage.Free{},
			)

			pool := mkCephPool()
			pool.l = lv

			_, err := pool.AddVolume("test-volume", storage.Options{})
			must.ErrorIs(t, err, errTest)
		})

		t.Run("failure looking up parent volume", func(t *testing.T) {
			volName := "test-volume"
			lvPool := mock_libvirt_storage.NewMockStoragePool(t)
			defer lvPool.AssertExpectations()
			lvPool.Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{Name: volName, Err: ErrVolumeNotFound},
				mock_libvirt_storage.LookupStorageVolByName{Name: "parent.img", Err: errTest},
				mock_libvirt_storage.Free{},
			)
			lv := &mock_libvirt.StaticLibvirt{FindStoragePoolResult: lvPool}

			pool := mkCephPool()
			pool.l = lv

			volOptions := storage.Options{
				Chained: true,
				Source: storage.Source{
					Path:   testImagePath(),
					Volume: "parent.img",
				},
				Target: storage.Target{Format: "test-format"},
			}
			_, err := pool.AddVolume(volName, volOptions)
			must.ErrorIs(t, err, errTest)
		})
	})

	t.Run("success", func(t *testing.T) {
		t.Run("returns existing volume", func(t *testing.T) {
			lv := mock_libvirt.NewMockLibvirt(t)
			defer lv.AssertExpectations()
			lvPool := mock_libvirt_storage.NewMockStoragePool(t)
			defer lvPool.AssertExpectations()
			lvStrVol := mock_libvirt_storage.NewStaticStorageVol()

			lv.Expect(mock_libvirt.FindStoragePool{Name: testPoolName, Result: lvPool})
			lvPool.Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.GetName{Result: testPoolName},
				mock_libvirt_storage.LookupStorageVolByName{
					Name:   "test-volume",
					Result: lvStrVol,
				},
				mock_libvirt_storage.Free{},
			)

			pool := mkCephPool()
			pool.l = lv

			result, err := pool.AddVolume("test-volume", storage.Options{})
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: "test-volume", Pool: testPoolName}, result)
			// Check that the libvirt volume was properly freed
			must.One(t, lvStrVol.CallCount("Free"), must.Sprint("expected volume Free call"))
		})

		t.Run("creates empty volume", func(t *testing.T) {
			volName := "test-volume"
			expectedVolCreate := libvirtxml.StorageVolume{
				Name: volName,
				Target: &libvirtxml.StorageVolumeTarget{
					Format: &libvirtxml.StorageVolumeTargetFormat{
						Type: "raw",
					},
				},
				Capacity: &libvirtxml.StorageVolumeSize{
					Unit:  "B",
					Value: 200,
				},
				Allocation: &libvirtxml.StorageVolumeSize{
					Unit:  "B",
					Value: 0,
				},
			}
			expectedVolCreateXml, err := expectedVolCreate.Marshal()
			must.NoError(t, err)
			vol := &mock_libvirt_storage.StaticStorageVol{
				GetXMLDescResult: expectedVolCreateXml,
			}
			lvPool := mock_libvirt_storage.NewMockStoragePool(t)
			defer lvPool.AssertExpectations()
			lvPool.Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{Name: volName, Err: ErrVolumeNotFound},
				mock_libvirt_storage.StorageVolCreateXML{Desc: expectedVolCreateXml, Result: vol},
				mock_libvirt_storage.Free{},
			)
			lv := &mock_libvirt.StaticLibvirt{FindStoragePoolResult: lvPool}

			pool := mkCephPool()
			pool.l = lv

			result, err := pool.AddVolume(volName,
				storage.Options{Size: 200, Target: storage.Target{Format: "raw"}})
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: volName, Pool: testPoolName, Format: "raw", Size: 200}, result)
			// Ensure volume was freed
			must.One(t, vol.CallCount("Free"), must.Sprint("expected volume Free call"))
		})

		t.Run("creates volume from image", func(t *testing.T) {
			volName := "test-volume"
			expectedVolCreate := libvirtxml.StorageVolume{
				Name: volName,
				Target: &libvirtxml.StorageVolumeTarget{
					Format: &libvirtxml.StorageVolumeTargetFormat{
						Type: "raw",
					},
				},
				Allocation: &libvirtxml.StorageVolumeSize{
					Unit:  "B",
					Value: 0,
				},
				Capacity: &libvirtxml.StorageVolumeSize{
					Unit:  "B",
					Value: 200,
				},
			}
			expectedVolCreateXml, err := expectedVolCreate.Marshal()
			must.NoError(t, err)
			vol := &mock_libvirt_storage.StaticStorageVol{
				GetXMLDescResult: expectedVolCreateXml,
			}
			lvPool := mock_libvirt_storage.NewMockStoragePool(t)
			defer lvPool.AssertExpectations()
			lvPool.Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{Name: volName, Err: ErrVolumeNotFound},
				mock_libvirt_storage.StorageVolCreateXML{Desc: expectedVolCreateXml, Result: vol},
				mock_libvirt_storage.Free{},
			)
			lv := &mock_libvirt.StaticLibvirt{
				FindStoragePoolResult: lvPool,
			}

			pool := mkCephPool()
			pool.l = lv

			volOptions := storage.Options{
				Size: 200,
				Source: storage.Source{
					Path: testImagePath(),
				},
				Target: storage.Target{Format: "raw"},
			}
			result, err := pool.AddVolume(volName, volOptions)
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: volName, Pool: testPoolName, Format: "raw", Size: 200}, result)
			// Volume should have been resized
			must.One(t, vol.CallCount("Resize"), must.Sprint("expected volume Resize call"))
			// Ensure volume was freed
			must.One(t, vol.CallCount("Free"), must.Sprint("expected volume Free call"))
		})

		t.Run("creates chained volume using existing volume", func(t *testing.T) {
			volName := "test-volume"
			expectedVolCreate := libvirtxml.StorageVolume{
				Name: volName,
				Target: &libvirtxml.StorageVolumeTarget{
					Format: &libvirtxml.StorageVolumeTargetFormat{
						Type: "test-format",
					},
				},
				Capacity: &libvirtxml.StorageVolumeSize{
					Unit:  "B",
					Value: uint64(len(testImageConvertedContent)),
				},
				Allocation: &libvirtxml.StorageVolumeSize{
					Unit:  "B",
					Value: 0,
				},
			}
			expectedVolCreateXml, err := expectedVolCreate.Marshal()
			must.NoError(t, err)
			parentVolDesc := libvirtxml.StorageVolume{
				Target: &libvirtxml.StorageVolumeTarget{
					Format: &libvirtxml.StorageVolumeTargetFormat{
						Type: "parent-volume-format",
					},
				},
			}
			parentVolXml, err := parentVolDesc.Marshal()
			must.NoError(t, err)
			parentVol := &mock_libvirt_storage.StaticStorageVol{
				GetPathResult:    "/dev/null/parent.img",
				GetXMLDescResult: parentVolXml,
				GetInfoResult: &libvirt.StorageVolInfo{
					Capacity: uint64(len(testImageConvertedContent)),
				},
			}

			vol := &mock_libvirt_storage.StaticStorageVol{
				GetXMLDescResult: expectedVolCreateXml,
			}
			lvPool := mock_libvirt_storage.NewMockStoragePool(t)
			defer lvPool.AssertExpectations()
			lvPool.Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{Name: volName, Err: ErrVolumeNotFound},
				mock_libvirt_storage.LookupStorageVolByName{Name: "parent.img", Result: parentVol},
				mock_libvirt_storage.StorageVolCreateXMLFrom{CloneVol: parentVol, Desc: expectedVolCreateXml, Result: vol},
				mock_libvirt_storage.Free{},
			)
			lv := &mock_libvirt.StaticLibvirt{
				FindStoragePoolResult: lvPool,
			}

			pool := mkCephPool()
			pool.l = lv

			volOptions := storage.Options{
				Chained: true,
				Size:    uint64(len(testImageConvertedContent)),
				Source: storage.Source{
					Volume: "parent.img",
				},
				Target: storage.Target{Format: "test-format"},
			}
			result, err := pool.AddVolume(volName, volOptions)
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: volName, Pool: testPoolName, Format: "test-format", Size: uint64(len(testImageConvertedContent))}, result)
		})
	})
}

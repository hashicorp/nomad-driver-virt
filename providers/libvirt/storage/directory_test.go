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

func TestDirectory_ValidateDisk(t *testing.T) {
	t.Parallel()
	pool := &directory{
		pool: &pool{
			name:   "test-pool",
			logger: hclog.NewNullLogger(),
			l:      mock_libvirt.NewStaticLibvirt(),
			s:      mock_storage.NewStaticStorage(),
			ctx:    t.Context(),
		},
	}

	t.Run("format support", func(t *testing.T) {
		t.Run("qcow2", func(t *testing.T) {
			disk := &disks.Disk{Format: storage.DiskFormatQcow2}
			must.NoError(t, pool.ValidateDisk(disk))
		})

		t.Run("raw", func(t *testing.T) {
			disk := &disks.Disk{Format: storage.DiskFormatRaw}
			must.NoError(t, pool.ValidateDisk(disk))
		})

		t.Run("unknown", func(t *testing.T) {
			disk := &disks.Disk{Format: "unknown"}
			err := pool.ValidateDisk(disk)
			must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
			must.ErrorContains(t, err, "format only supports")
		})
	})

	t.Run("chained support", func(t *testing.T) {
		t.Run("qcow2 format", func(t *testing.T) {
			disk := &disks.Disk{Format: storage.DiskFormatQcow2, Chained: true}
			must.NoError(t, pool.ValidateDisk(disk))
		})

		t.Run("raw format", func(t *testing.T) {
			disk := &disks.Disk{Format: storage.DiskFormatRaw, Chained: true}
			err := pool.ValidateDisk(disk)
			must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
			must.ErrorContains(t, err, "format must be qcow2")
		})
	})

	t.Run("sparse modification", func(t *testing.T) {
		t.Run("value is unset", func(t *testing.T) {
			t.Run("enabled if format is qcow2", func(t *testing.T) {
				disk := &disks.Disk{Format: storage.DiskFormatQcow2}
				err := pool.ValidateDisk(disk)
				must.NoError(t, err)
				must.NotNil(t, disk.Sparse, must.Sprint("expected sparse to be set"))
				must.True(t, *disk.Sparse, must.Sprint("expected sparse to be enabled"))
			})

			t.Run("unset if format is not qcow2", func(t *testing.T) {
				disk := &disks.Disk{Format: storage.DiskFormatRaw}
				err := pool.ValidateDisk(disk)
				must.NoError(t, err)
				must.Nil(t, disk.Sparse, must.Sprint("expected sparse to be unset"))
			})
		})

		t.Run("value is set", func(t *testing.T) {
			t.Run("unchanged when format is qcow2", func(t *testing.T) {
				disk := &disks.Disk{Format: storage.DiskFormatQcow2, Sparse: pointer.Of(false)}
				err := pool.ValidateDisk(disk)
				must.NoError(t, err)
				must.NotNil(t, disk.Sparse, must.Sprint("expected sparse to be set"))
				must.False(t, *disk.Sparse, must.Sprint("expected sparse to be disabled"))
			})

			t.Run("unchanged when format is not qcow2", func(t *testing.T) {
				disk := &disks.Disk{Format: storage.DiskFormatRaw, Sparse: pointer.Of(false)}
				err := pool.ValidateDisk(disk)
				must.NoError(t, err)
				must.NotNil(t, disk.Sparse, must.Sprint("expected sparse to be set"))
				must.False(t, *disk.Sparse, must.Sprint("expected sparse to be disabled"))
			})
		})
	})
}

func TestDirectory_AddVolume(t *testing.T) {
	t.Parallel()

	errTest := errors.New("test error")
	testImageContent := []byte("test content")
	testImagePath := filepath.Join(t.TempDir(), "testing.img")
	f, err := os.OpenFile(testImagePath, os.O_CREATE|os.O_WRONLY, 0644)
	must.NoError(t, err)
	f.Write(testImageContent)
	must.NoError(t, f.Close())

	testPoolName := "test-pool"
	mkDirPool := func() *directory {
		return &directory{
			pool: &pool{
				name:   testPoolName,
				logger: hclog.NewNullLogger(),
				l:      mock_libvirt.NewStaticLibvirt(),
				s:      mock_storage.NewStaticStorage(),
				ctx:    t.Context(),
			},
		}
	}

	t.Run("errors", func(t *testing.T) {
		t.Run("storage pool is not found", func(t *testing.T) {
			l := mock_libvirt.NewMockLibvirt(t)
			defer l.AssertExpectations()
			l.Expect(mock_libvirt.FindStoragePool{Name: testPoolName, Err: errTest})
			dirPool := mkDirPool()
			dirPool.l = l

			_, err := dirPool.AddVolume(testPoolName, storage.Options{})
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

			dirPool := mkDirPool()
			dirPool.l = lv

			_, err := dirPool.AddVolume("test-volume", storage.Options{})
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

			dirPool := mkDirPool()
			dirPool.l = lv

			result, err := dirPool.AddVolume("test-volume", storage.Options{})
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
						Type: "test-format",
					},
				},
				Allocation: &libvirtxml.StorageVolumeSize{
					Unit:  "B",
					Value: 200,
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
			lv := &mock_libvirt.StaticLibvirt{FindStoragePoolResult: lvPool}

			dirPool := mkDirPool()
			dirPool.l = lv

			result, err := dirPool.AddVolume(volName,
				storage.Options{Size: 200, Target: storage.Target{Format: "test-format"}})
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: volName, Pool: testPoolName, Format: "test-format", Size: 200}, result)
			// Ensure volume was freed
			must.One(t, vol.CallCount("Free"), must.Sprint("expected volume Free call"))
		})

		t.Run("creates volume from image", func(t *testing.T) {
			volName := "test-volume"
			expectedVolCreate := libvirtxml.StorageVolume{
				Name: volName,
				Target: &libvirtxml.StorageVolumeTarget{
					Format: &libvirtxml.StorageVolumeTargetFormat{
						Type: "test-format",
					},
				},
				Allocation: &libvirtxml.StorageVolumeSize{
					Unit:  "B",
					Value: 200,
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
			lvStream := mock_libvirt.NewStaticStream()
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
				NewStreamResult:       lvStream,
			}

			dirPool := mkDirPool()
			dirPool.l = lv

			volOptions := storage.Options{
				Size: 200,
				Source: storage.Source{
					Path: testImagePath,
				},
				Target: storage.Target{Format: "test-format"},
			}
			result, err := dirPool.AddVolume(volName, volOptions)
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: volName, Pool: testPoolName, Format: "test-format", Size: 200}, result)
			// Volume should have received an upload request
			must.One(t, vol.CallCount("Upload"), must.Sprint("expected volume Upload call"))
			// Stream should have been written to
			must.One(t, lvStream.CallCount("Write"), must.Sprint("expected stream Write call"))
			// Volume should have been resized
			must.One(t, vol.CallCount("Resize"), must.Sprint("expected volume Resize call"))
			// Stream should have been finished
			must.One(t, lvStream.CallCount("Finish"), must.Sprint("expected stream Finish call"))
			// Stream should not have been aborted
			must.Zero(t, lvStream.CallCount("Abort"), must.Sprint("did not expect stream Abort call"))
			// Ensure stream was freed
			must.One(t, lvStream.CallCount("Free"), must.Sprint("expected stream Free call"))
			// Ensure volume was freed
			must.One(t, vol.CallCount("Free"), must.Sprint("expected volume Free call"))
		})

		t.Run("creates sparse volume from image", func(t *testing.T) {
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
					Value: 200,
				},
				Allocation: &libvirtxml.StorageVolumeSize{
					Unit:  "B",
					Value: 0,
				},
			}
			expectedVolCreateXml, err := expectedVolCreate.Marshal()
			must.NoError(t, err)
			vol := mock_libvirt_storage.NewMockStorageVol(t)
			defer vol.AssertExpectations()
			lvStream := mock_libvirt.NewStaticStream()
			lvPool := mock_libvirt_storage.NewMockStoragePool(t)
			defer lvPool.AssertExpectations()
			vol.Expect(
				mock_libvirt_storage.Resize{Size: 200, Flags: libvirtNoFlags},
				mock_libvirt_storage.Upload{Stream: lvStream, Size: uint64(len(testImageContent))},
				mock_libvirt_storage.GetInfo{Result: &libvirt.StorageVolInfo{}},
				mock_libvirt_storage.GetXMLDesc{Result: expectedVolCreateXml},
				mock_libvirt_storage.Free{},
			)
			lvPool.Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{Name: volName, Err: ErrVolumeNotFound},
				mock_libvirt_storage.StorageVolCreateXML{Desc: expectedVolCreateXml, Result: vol},
				mock_libvirt_storage.Free{},
			)
			lv := &mock_libvirt.StaticLibvirt{
				FindStoragePoolResult: lvPool,
				NewStreamResult:       lvStream,
			}

			dirPool := mkDirPool()
			dirPool.l = lv

			volOptions := storage.Options{
				Size: 200,
				Source: storage.Source{
					Path: testImagePath,
				},
				Target: storage.Target{Format: "test-format"},
				Sparse: true,
			}
			result, err := dirPool.AddVolume(volName, volOptions)
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: volName, Pool: testPoolName, Format: "test-format", Size: 200}, result)
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
					Value: uint64(len(testImageContent)),
				},
				Allocation: &libvirtxml.StorageVolumeSize{
					Unit:  "B",
					Value: uint64(len(testImageContent)),
				},
				BackingStore: &libvirtxml.StorageVolumeBackingStore{
					Path: "/dev/null/parent.img",
					Format: &libvirtxml.StorageVolumeTargetFormat{
						Type: "test-format",
					},
				},
			}
			expectedVolCreateXml, err := expectedVolCreate.Marshal()
			must.NoError(t, err)
			parentVolDesc := libvirtxml.StorageVolume{
				Target: &libvirtxml.StorageVolumeTarget{
					Format: &libvirtxml.StorageVolumeTargetFormat{
						Type: "test-format",
					},
				},
			}
			parentVolXml, err := parentVolDesc.Marshal()
			must.NoError(t, err)
			parentVol := &mock_libvirt_storage.StaticStorageVol{
				GetPathResult:    "/dev/null/parent.img",
				GetXMLDescResult: parentVolXml,
				GetInfoResult: &libvirt.StorageVolInfo{
					Capacity: uint64(len(testImageContent)),
				},
			}

			vol := &mock_libvirt_storage.StaticStorageVol{
				GetXMLDescResult: expectedVolCreateXml,
			}
			lvStream := mock_libvirt.NewStaticStream()
			lvPool := mock_libvirt_storage.NewMockStoragePool(t)
			defer lvPool.AssertExpectations()
			lvPool.Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{Name: volName, Err: ErrVolumeNotFound},
				mock_libvirt_storage.LookupStorageVolByName{Name: "parent.img", Result: parentVol},

				mock_libvirt_storage.StorageVolCreateXML{Desc: expectedVolCreateXml, Result: vol},
				mock_libvirt_storage.Free{},
			)
			lv := &mock_libvirt.StaticLibvirt{
				FindStoragePoolResult: lvPool,
				NewStreamResult:       lvStream,
			}

			dirPool := mkDirPool()
			dirPool.l = lv

			volOptions := storage.Options{
				Chained: true,
				Size:    uint64(len(testImageContent)),
				Source: storage.Source{
					Volume: "parent.img",
				},
				Target: storage.Target{Format: "test-format"},
			}
			result, err := dirPool.AddVolume(volName, volOptions)
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: volName, Pool: testPoolName, Format: "test-format", Size: 12}, result)
			must.Zero(t, lvStream.CallCount("Upload"), must.Sprint("image upload is not expected"))
		})
	})
}

func TestDirectory_DeleteVolume(t *testing.T) {
	t.Parallel()

	errTest := errors.New("test error")
	testPoolName := "test-pool"
	mkDirPool := func() *directory {
		return &directory{
			pool: &pool{
				name:   testPoolName,
				logger: hclog.NewNullLogger(),
				l:      mock_libvirt.NewStaticLibvirt(),
				s:      mock_storage.NewStaticStorage(),
			},
		}
	}

	t.Run("existing volume", func(t *testing.T) {
		volName := "test-volume"
		lvPool := mock_libvirt_storage.NewMockStoragePool(t)
		vol := mock_libvirt_storage.NewStaticStorageVol()
		defer lvPool.AssertExpectations()
		lvPool.Expect(
			mock_libvirt_storage.Refresh{},
			mock_libvirt_storage.LookupStorageVolByName{Name: volName, Result: vol},
			mock_libvirt_storage.Free{},
		)
		lv := &mock_libvirt.StaticLibvirt{FindStoragePoolResult: lvPool}
		dirPool := mkDirPool()
		dirPool.l = lv

		err := dirPool.DeleteVolume(volName)
		must.NoError(t, err)
		// Check that the volume was deleted
		must.One(t, vol.CallCount("Delete"), must.Sprint("expected volume Delete call"))
		// Check that the volume was freed
		must.One(t, vol.CallCount("Free"), must.Sprint("expected volume Free call"))
	})

	t.Run("missing volume", func(t *testing.T) {
		volName := "test-volume"
		lvPool := mock_libvirt_storage.NewMockStoragePool(t)
		defer lvPool.AssertExpectations()
		lvPool.Expect(
			mock_libvirt_storage.Refresh{},
			mock_libvirt_storage.LookupStorageVolByName{Name: volName, Err: ErrVolumeNotFound},
			mock_libvirt_storage.Free{},
		)
		lv := &mock_libvirt.StaticLibvirt{FindStoragePoolResult: lvPool}
		dirPool := mkDirPool()
		dirPool.l = lv

		err := dirPool.DeleteVolume(volName)
		must.NoError(t, err)
	})

	t.Run("error on volume lookup", func(t *testing.T) {
		volName := "test-volume"
		lvPool := mock_libvirt_storage.NewMockStoragePool(t)
		defer lvPool.AssertExpectations()
		lvPool.Expect(
			mock_libvirt_storage.Refresh{},
			mock_libvirt_storage.LookupStorageVolByName{Name: volName, Err: errTest},
			mock_libvirt_storage.Free{},
		)
		lv := &mock_libvirt.StaticLibvirt{FindStoragePoolResult: lvPool}
		dirPool := mkDirPool()
		dirPool.l = lv

		err := dirPool.DeleteVolume(volName)
		must.ErrorIs(t, err, errTest)
	})
}

func TestDirectory_GetVolume(t *testing.T) {
	t.Parallel()

	errTest := errors.New("test error")
	testPoolName := "test-pool"
	mkDirPool := func() *directory {
		return &directory{
			pool: &pool{
				name:   testPoolName,
				logger: hclog.NewNullLogger(),
				l:      mock_libvirt.NewStaticLibvirt(),
				s:      mock_storage.NewStaticStorage(),
			},
		}
	}

	t.Run("existing volume", func(t *testing.T) {
		volName := "test-volume"
		lvPool := mock_libvirt_storage.NewMockStoragePool(t)
		lvVol := mock_libvirt_storage.NewStaticStorageVol()
		defer lvPool.AssertExpectations()
		lvPool.Expect(
			mock_libvirt_storage.LookupStorageVolByName{Name: volName, Result: lvVol},
			mock_libvirt_storage.GetName{Result: testPoolName},
			mock_libvirt_storage.Free{},
		)
		lv := &mock_libvirt.StaticLibvirt{FindStoragePoolResult: lvPool}
		dirPool := mkDirPool()
		dirPool.l = lv

		vol, err := dirPool.GetVolume(volName)
		must.NoError(t, err)
		must.Eq(t, &storage.Volume{Name: volName, Pool: testPoolName}, vol)
		// Check that the volume was freed
		must.One(t, lvVol.CallCount("Free"), must.Sprint("expected volume Free call"))
	})

	t.Run("missing volume", func(t *testing.T) {
		volName := "test-volume"
		lvPool := mock_libvirt_storage.NewMockStoragePool(t)
		defer lvPool.AssertExpectations()
		lvPool.Expect(
			mock_libvirt_storage.LookupStorageVolByName{Name: volName, Err: libvirt.ERR_NO_STORAGE_VOL},
			mock_libvirt_storage.Free{},
		)
		lv := &mock_libvirt.StaticLibvirt{FindStoragePoolResult: lvPool}
		dirPool := mkDirPool()
		dirPool.l = lv

		vol, err := dirPool.GetVolume(volName)
		must.ErrorIs(t, err, ErrVolumeNotFound)
		must.Nil(t, vol)
	})

	t.Run("error on volume lookup", func(t *testing.T) {
		volName := "test-volume"
		lvPool := mock_libvirt_storage.NewMockStoragePool(t)
		defer lvPool.AssertExpectations()
		lvPool.Expect(
			mock_libvirt_storage.LookupStorageVolByName{Name: volName, Err: errTest},
			mock_libvirt_storage.Free{},
		)
		lv := &mock_libvirt.StaticLibvirt{FindStoragePoolResult: lvPool}
		dirPool := mkDirPool()
		dirPool.l = lv

		vol, err := dirPool.GetVolume(volName)
		must.ErrorIs(t, err, errTest)
		must.Nil(t, vol)
	})
}

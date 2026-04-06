// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/storage"
	mock_libvirt "github.com/hashicorp/nomad-driver-virt/testutil/mock/providers/libvirt"
	mock_libvirt_storage "github.com/hashicorp/nomad-driver-virt/testutil/mock/providers/libvirt/storage"
	mock_storage "github.com/hashicorp/nomad-driver-virt/testutil/mock/storage"
	"github.com/shoenig/test/must"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

func TestPool_GetVolume(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		vol := &libvirtxml.StorageVolume{Type: "testing-volume"}
		volXml, err := vol.Marshal()
		must.NoError(t, err)

		lvVol := &mock_libvirt_storage.StaticStorageVol{
			GetXMLDescResult: volXml,
		}
		lvPool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
			mock_libvirt_storage.GetName{Result: "test-pool"},
			mock_libvirt_storage.LookupStorageVolByName{
				Name:   "test-vol",
				Result: lvVol,
			},
			mock_libvirt_storage.Free{},
		)
		defer lvPool.AssertExpectations()
		pool := mkPool(t.Context(), lvPool)

		result, err := pool.GetVolume("test-vol")
		must.NoError(t, err)
		must.Eq(t, "test-vol", result.Name)
		must.Eq(t, "test-pool", result.Pool)
		must.Eq(t, "testing-volume", result.Kind)

		must.One(t, lvVol.CallCount("Free"), must.Sprint("libvirt volume was not freed"))
	})
}

func TestPool_ListVolumes(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		lvPool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
			mock_libvirt_storage.ListStorageVolumes{
				Result: []string{"vol1", "vol2"},
			},
			mock_libvirt_storage.Free{},
		)
		defer lvPool.AssertExpectations()
		pool := mkPool(t.Context(), lvPool)

		vols, err := pool.ListVolumes()
		must.NoError(t, err)
		must.Eq(t, []string{"vol1", "vol2"}, vols)
	})
}

func TestPool_DeleteVolume(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		lvVol := mock_libvirt_storage.NewStaticStorageVol()
		lvPool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
			mock_libvirt_storage.Refresh{},
			mock_libvirt_storage.LookupStorageVolByName{
				Name:   "test-vol",
				Result: lvVol,
			},
			mock_libvirt_storage.Free{},
		)
		defer lvPool.AssertExpectations()
		pool := mkPool(t.Context(), lvPool)

		must.NoError(t, pool.DeleteVolume("test-vol"))
		must.One(t, lvVol.CallCount("Free"), must.Sprint("libvirt volume was not freed"))
	})

	t.Run("ok - not found", func(t *testing.T) {
		lvPool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
			mock_libvirt_storage.Refresh{},
			mock_libvirt_storage.LookupStorageVolByName{
				Name: "test-vol",
				Err:  ErrVolumeNotFound,
			},
			mock_libvirt_storage.Free{},
		)
		defer lvPool.AssertExpectations()
		pool := mkPool(t.Context(), lvPool)

		must.NoError(t, pool.DeleteVolume("test-vol"))
	})
}

func TestPool_AddVolume(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		t.Run("basic", func(t *testing.T) {
			name := "test-vol"
			opts := storage.Options{
				Size: 200,
				Target: storage.Target{
					Format: "raw",
				},
			}

			expectedVol := &libvirtxml.StorageVolume{
				Name: name,
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
					Value: 200,
				},
			}
			expectedData, err := expectedVol.Marshal()
			must.NoError(t, err)

			lvVol := mock_libvirt_storage.NewMockStorageVol(t).Expect(
				mock_libvirt_storage.GetXMLDesc{
					Result: expectedData,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvVol.AssertExpectations()
			lvPool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{
					Name: name,
					Err:  ErrVolumeNotFound,
				},
				mock_libvirt_storage.StorageVolCreateXML{
					Desc:   expectedData,
					Result: lvVol,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvPool.AssertExpectations()
			pool := mkPool(t.Context(), lvPool)

			vol, err := pool.AddVolume(name, opts)
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: name, Pool: "test-pool", Size: 200, Format: "raw"}, vol)
		})

		t.Run("sparse", func(t *testing.T) {
			name := "test-vol"
			opts := storage.Options{
				Size:   200,
				Sparse: true,
				Target: storage.Target{
					Format: "raw",
				},
			}

			expectedVol := &libvirtxml.StorageVolume{
				Name: name,
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
			expectedData, err := expectedVol.Marshal()
			must.NoError(t, err)

			lvVol := mock_libvirt_storage.NewMockStorageVol(t).Expect(
				mock_libvirt_storage.GetXMLDesc{
					Result: expectedData,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvVol.AssertExpectations()
			lvPool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{
					Name: name,
					Err:  ErrVolumeNotFound,
				},
				mock_libvirt_storage.StorageVolCreateXML{
					Desc:   expectedData,
					Result: lvVol,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvPool.AssertExpectations()
			pool := mkPool(t.Context(), lvPool)

			vol, err := pool.AddVolume(name, opts)
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: name, Pool: "test-pool", Size: 200, Format: "raw"}, vol)
		})

		t.Run("source volume", func(t *testing.T) {
			name := "test-vol"
			opts := storage.Options{
				Size: 200,
				Source: storage.Source{
					Volume: "parent-vol",
				},
				Target: storage.Target{
					Format: "raw",
				},
			}

			expectedVol := &libvirtxml.StorageVolume{
				Name: name,
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
					Value: 200,
				},
			}
			expectedData, err := expectedVol.Marshal()
			must.NoError(t, err)

			lvParentVol := mock_libvirt_storage.NewStaticStorageVol()
			lvVol := mock_libvirt_storage.NewMockStorageVol(t).Expect(
				mock_libvirt_storage.GetInfo{
					Result: &libvirt.StorageVolInfo{
						Capacity: 200,
					},
				},
				mock_libvirt_storage.GetXMLDesc{
					Result: expectedData,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvVol.AssertExpectations()
			lvPool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{
					Name: name,
					Err:  ErrVolumeNotFound,
				},
				mock_libvirt_storage.LookupStorageVolByName{
					Name:   "parent-vol",
					Result: lvParentVol,
				},
				mock_libvirt_storage.StorageVolCreateXMLFrom{
					CloneVol: lvParentVol,
					Desc:     expectedData,
					Result:   lvVol,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvPool.AssertExpectations()
			pool := mkPool(t.Context(), lvPool)

			vol, err := pool.AddVolume(name, opts)
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: name, Pool: "test-pool", Size: 200, Format: "raw"}, vol)
			must.One(t, lvParentVol.CallCount("Free"), must.Sprint("parent volume was not freed"))
		})

		t.Run("source volume resized", func(t *testing.T) {
			name := "test-vol"
			opts := storage.Options{
				Size: 200,
				Source: storage.Source{
					Volume: "parent-vol",
				},
				Target: storage.Target{
					Format: "raw",
				},
			}

			expectedVol := &libvirtxml.StorageVolume{
				Name: name,
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
					Value: 200,
				},
			}
			expectedData, err := expectedVol.Marshal()
			must.NoError(t, err)

			lvParentVol := mock_libvirt_storage.NewStaticStorageVol()
			lvVol := mock_libvirt_storage.NewMockStorageVol(t).Expect(
				mock_libvirt_storage.GetInfo{
					Result: &libvirt.StorageVolInfo{
						Capacity: 100, // NOTE: Smaller capacity will trigger a resize after clone
					},
				},
				mock_libvirt_storage.Resize{
					Size:  200,
					Flags: libvirt.STORAGE_VOL_RESIZE_ALLOCATE,
				},
				mock_libvirt_storage.GetXMLDesc{
					Result: expectedData,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvVol.AssertExpectations()
			lvPool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{
					Name: name,
					Err:  ErrVolumeNotFound,
				},
				mock_libvirt_storage.LookupStorageVolByName{
					Name:   "parent-vol",
					Result: lvParentVol,
				},
				mock_libvirt_storage.StorageVolCreateXMLFrom{
					CloneVol: lvParentVol,
					Desc:     expectedData,
					Result:   lvVol,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvPool.AssertExpectations()
			pool := mkPool(t.Context(), lvPool)

			vol, err := pool.AddVolume(name, opts)
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: name, Pool: "test-pool", Size: 200, Format: "raw"}, vol)
			must.One(t, lvParentVol.CallCount("Free"), must.Sprint("parent volume was not freed"))
		})

		t.Run("sparse source volume resized", func(t *testing.T) {
			name := "test-vol"
			opts := storage.Options{
				Size:   200,
				Sparse: true,
				Source: storage.Source{
					Volume: "parent-vol",
				},
				Target: storage.Target{
					Format: "raw",
				},
			}

			expectedVol := &libvirtxml.StorageVolume{
				Name: name,
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
			expectedData, err := expectedVol.Marshal()
			must.NoError(t, err)

			lvParentVol := mock_libvirt_storage.NewStaticStorageVol()
			lvVol := mock_libvirt_storage.NewMockStorageVol(t).Expect(
				mock_libvirt_storage.GetInfo{
					Result: &libvirt.StorageVolInfo{
						Capacity: 100, // NOTE: Smaller capacity will trigger a resize after clone
					},
				},
				mock_libvirt_storage.Resize{
					Size:  200,
					Flags: libvirtNoFlags,
				},
				mock_libvirt_storage.GetXMLDesc{
					Result: expectedData,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvVol.AssertExpectations()
			lvPool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{
					Name: name,
					Err:  ErrVolumeNotFound,
				},
				mock_libvirt_storage.LookupStorageVolByName{
					Name:   "parent-vol",
					Result: lvParentVol,
				},
				mock_libvirt_storage.StorageVolCreateXMLFrom{
					CloneVol: lvParentVol,
					Desc:     expectedData,
					Result:   lvVol,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvPool.AssertExpectations()
			pool := mkPool(t.Context(), lvPool)

			vol, err := pool.AddVolume(name, opts)
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: name, Pool: "test-pool", Size: 200, Format: "raw"}, vol)
			must.One(t, lvParentVol.CallCount("Free"), must.Sprint("parent volume was not freed"))
		})

		t.Run("chained source volume", func(t *testing.T) {
			name := "test-vol"
			opts := storage.Options{
				Size:    200,
				Chained: true,
				Source: storage.Source{
					Volume: "parent-vol",
				},
				Target: storage.Target{
					Format: "raw",
				},
			}

			parentVol := &libvirtxml.StorageVolume{
				Name: "parent-vol",
				Target: &libvirtxml.StorageVolumeTarget{
					Format: &libvirtxml.StorageVolumeTargetFormat{
						Type: "raw",
					},
				},
			}
			parentData, err := parentVol.Marshal()
			must.NoError(t, err)

			expectedVol := &libvirtxml.StorageVolume{
				Name: name,
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
					Value: 200,
				},
				BackingStore: &libvirtxml.StorageVolumeBackingStore{
					Path: "/dev/null/parent/vol",
					Format: &libvirtxml.StorageVolumeTargetFormat{
						Type: "raw",
					},
				},
			}
			expectedData, err := expectedVol.Marshal()
			must.NoError(t, err)

			lvParentVol := mock_libvirt_storage.NewMockStorageVol(t).Expect(
				mock_libvirt_storage.GetPath{
					Result: "/dev/null/parent/vol",
				},
				mock_libvirt_storage.GetXMLDesc{
					Result: parentData,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvParentVol.AssertExpectations()
			lvVol := mock_libvirt_storage.NewMockStorageVol(t).Expect(
				mock_libvirt_storage.GetXMLDesc{
					Result: expectedData,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvVol.AssertExpectations()
			lvPool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{
					Name: name,
					Err:  ErrVolumeNotFound,
				},
				mock_libvirt_storage.LookupStorageVolByName{
					Name:   "parent-vol",
					Result: lvParentVol,
				},
				mock_libvirt_storage.StorageVolCreateXML{
					Desc:   expectedData,
					Result: lvVol,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvPool.AssertExpectations()
			pool := mkPool(t.Context(), lvPool)

			vol, err := pool.AddVolume(name, opts)
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: name, Pool: "test-pool", Size: 200, Format: "raw"}, vol)
		})

		t.Run("sparse chained source volume", func(t *testing.T) {
			name := "test-vol"
			opts := storage.Options{
				Size:    200,
				Chained: true,
				Sparse:  true,
				Source: storage.Source{
					Volume: "parent-vol",
				},
				Target: storage.Target{
					Format: "raw",
				},
			}

			parentVol := &libvirtxml.StorageVolume{
				Name: "parent-vol",
				Target: &libvirtxml.StorageVolumeTarget{
					Format: &libvirtxml.StorageVolumeTargetFormat{
						Type: "raw",
					},
				},
			}
			parentData, err := parentVol.Marshal()
			must.NoError(t, err)

			expectedVol := &libvirtxml.StorageVolume{
				Name: name,
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
				BackingStore: &libvirtxml.StorageVolumeBackingStore{
					Path: "/dev/null/parent/vol",
					Format: &libvirtxml.StorageVolumeTargetFormat{
						Type: "raw",
					},
				},
			}
			expectedData, err := expectedVol.Marshal()
			must.NoError(t, err)

			lvParentVol := mock_libvirt_storage.NewMockStorageVol(t).Expect(
				mock_libvirt_storage.GetPath{
					Result: "/dev/null/parent/vol",
				},
				mock_libvirt_storage.GetXMLDesc{
					Result: parentData,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvParentVol.AssertExpectations()
			lvVol := mock_libvirt_storage.NewMockStorageVol(t).Expect(
				mock_libvirt_storage.GetXMLDesc{
					Result: expectedData,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvVol.AssertExpectations()
			lvPool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{
					Name: name,
					Err:  ErrVolumeNotFound,
				},
				mock_libvirt_storage.LookupStorageVolByName{
					Name:   "parent-vol",
					Result: lvParentVol,
				},
				mock_libvirt_storage.StorageVolCreateXML{
					Desc:   expectedData,
					Result: lvVol,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvPool.AssertExpectations()
			pool := mkPool(t.Context(), lvPool)

			vol, err := pool.AddVolume(name, opts)
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: name, Pool: "test-pool", Size: 200, Format: "raw"}, vol)
		})

		t.Run("source image", func(t *testing.T) {
			imgPath := filepath.Join(t.TempDir(), "volume-image")
			f, err := os.Create(imgPath)
			must.NoError(t, err)
			f.Close()

			name := "test-vol"
			opts := storage.Options{
				Size: 200,
				Source: storage.Source{
					Path: imgPath,
				},
				Target: storage.Target{
					Format: "raw",
				},
			}

			expectedVol := &libvirtxml.StorageVolume{
				Name: name,
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
					Value: 200,
				},
			}
			expectedData, err := expectedVol.Marshal()
			must.NoError(t, err)

			lvVol := mock_libvirt_storage.NewMockStorageVol(t).Expect(
				mock_libvirt_storage.GetInfo{
					Result: &libvirt.StorageVolInfo{
						Capacity: 200,
					},
				},
				mock_libvirt_storage.GetXMLDesc{
					Result: expectedData,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvVol.AssertExpectations()
			lvPool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{
					Name: name,
					Err:  ErrVolumeNotFound,
				},
				mock_libvirt_storage.StorageVolCreateXML{
					Desc:   expectedData,
					Result: lvVol,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvPool.AssertExpectations()
			pool := mkPool(t.Context(), lvPool)

			vol, err := pool.AddVolume(name, opts)
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: name, Pool: "test-pool", Size: 200, Format: "raw"}, vol)
		})

		t.Run("volume already exists", func(t *testing.T) {
			name := "test-vol"
			opts := storage.Options{
				Size: 200,
				Target: storage.Target{
					Format: "raw",
				},
			}

			expectedVol := &libvirtxml.StorageVolume{
				Name: name,
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
					Value: 200,
				},
			}
			expectedData, err := expectedVol.Marshal()
			must.NoError(t, err)

			lvVol := mock_libvirt_storage.NewMockStorageVol(t).Expect(
				mock_libvirt_storage.GetXMLDesc{
					Result: expectedData,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvVol.AssertExpectations()
			lvPool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{
					Name:   name,
					Result: lvVol,
				},
				mock_libvirt_storage.GetName{
					Result: "test-pool",
				},
				mock_libvirt_storage.Free{},
			)
			defer lvPool.AssertExpectations()
			pool := mkPool(t.Context(), lvPool)

			vol, err := pool.AddVolume(name, opts)
			must.NoError(t, err)
			must.Eq(t, &storage.Volume{Name: name, Pool: "test-pool", Size: 200, Format: "raw"}, vol)
		})
	})

	t.Run("errors", func(t *testing.T) {
		testErr := errors.New("testing error")

		t.Run("volume lookup error", func(t *testing.T) {
			name := "test-vol"
			opts := storage.Options{
				Size: 200,
				Target: storage.Target{
					Format: "raw",
				},
			}
			lvPool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{
					Name: name,
					Err:  testErr,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvPool.AssertExpectations()
			pool := mkPool(t.Context(), lvPool)

			_, err := pool.AddVolume(name, opts)
			must.ErrorIs(t, err, testErr)
		})

		t.Run("chained missing source volume", func(t *testing.T) {
			name := "test-vol"
			opts := storage.Options{
				Size: 200,
				Target: storage.Target{
					Format: "raw",
				},
				Chained: true,
			}
			lvPool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
				mock_libvirt_storage.Refresh{},
				mock_libvirt_storage.LookupStorageVolByName{
					Name: name,
					Err:  ErrVolumeNotFound,
				},
				mock_libvirt_storage.Free{},
			)
			defer lvPool.AssertExpectations()
			pool := mkPool(t.Context(), lvPool)

			_, err := pool.AddVolume(name, opts)
			must.ErrorIs(t, err, ErrVolumeNotFound)
		})
	})
}

func mkPool(ctx context.Context, p *mock_libvirt_storage.MockStoragePool) *pool {
	return &pool{
		ctx:    ctx,
		logger: hclog.NewNullLogger(),
		name:   "test-pool",
		s:      mock_storage.NewStaticStorage(),
		l:      &mock_libvirt.StaticLibvirt{FindStoragePoolResult: p},
	}
}

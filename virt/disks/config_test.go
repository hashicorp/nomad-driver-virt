// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package disks

import (
	//	"errors"

	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	"github.com/hashicorp/nomad-driver-virt/storage"
	mock_storage "github.com/hashicorp/nomad-driver-virt/testutil/mock/storage"
	mock_image_tools "github.com/hashicorp/nomad-driver-virt/testutil/mock/storage/image_tools"
	"github.com/hashicorp/nomad/helper/pluginutils/hclutils"
	"github.com/hashicorp/nomad/helper/pointer"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/shoenig/test/must"
)

func TestValidationOptions(t *testing.T) {
	t.Parallel()

	t.Run("AllowedPaths", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "allowed", "root")
		must.NoError(t, os.MkdirAll(root, 0755))
		nonroot := filepath.Join(t.TempDir(), "allowed", "non-root")
		must.NoError(t, os.MkdirAll(nonroot, 0755))

		dirList := []string{
			filepath.Join(t.TempDir(), "does-not-exist"),
			root,
		}
		vOpts := ValidationOptions{AllowedPaths: dirList}

		t.Run("valid path", func(t *testing.T) {
			path := filepath.Join(root, "valid-file-path")
			f, err := os.OpenFile(path, os.O_CREATE, 0666)
			must.NoError(t, err)
			f.Close()

			must.True(t, vOpts.AllowedPath(path))
		})

		t.Run("invalid path", func(t *testing.T) {
			path := filepath.Join(nonroot, "invalid-file-path")
			f, err := os.OpenFile(path, os.O_CREATE, 0666)
			must.NoError(t, err)
			f.Close()

			must.False(t, vOpts.AllowedPath(path))
		})

		t.Run("path does not exist", func(t *testing.T) {
			must.False(t, vOpts.AllowedPath(filepath.Join(root, "fake-file")))
		})
	})
}

func TestDisk(t *testing.T) {
	t.Parallel()

	t.Run("ApplyCloudInit", func(t *testing.T) {
		t.Run("adds disk", func(t *testing.T) {
			d := NewDisks()
			d = d.ApplyCloudInit("/dev/null/img.iso")
			must.Len(t, 1, d)
			must.Eq(t, "/dev/null/img.iso", d[0].Source.Image)
			must.True(t, d[0].ReadOnly)
		})

		t.Run("creates disks and adds", func(t *testing.T) {
			d := NewDisks()
			d = nil
			d = d.ApplyCloudInit("/dev/null/img.iso")
			must.NotNil(t, d)
			must.Len(t, 1, d)
		})
	})

	t.Run("CompatAddImage", func(t *testing.T) {
		t.Run("adds disk", func(t *testing.T) {
			d := NewDisks()
			d = d.CompatAddImage("/dev/null/img.iso", 25, true)
			must.Len(t, 1, d)
			must.True(t, d[0].Primary, must.Sprint("expected disk to be primary"))
			must.Eq(t, "/dev/null/img.iso", d[0].Source.Image)
			must.Eq(t, "25MiB", d[0].Size)
		})

		t.Run("creates disks and adds", func(t *testing.T) {
			d := NewDisks()
			d = nil
			d = d.CompatAddImage("/dev/null/img.iso", 25, false)
			must.NotNil(t, d)
			must.Len(t, 1, d)
		})
	})

	t.Run("ApplyMounts", func(t *testing.T) {
		t.Run("it sets device name on disk", func(t *testing.T) {
			// TODO: This can be uncommented once https://github.com/hashicorp/nomad/pull/27710
			// has been merged and released.
			// t.Run("using request name", func(t *testing.T) {
			// 	d := Disks{{VolumeName: "nomad-volume"}}
			// 	m := []*drivers.MountConfig{
			// 		{
			// 			RequestName: "nomad-volume",
			// 			HostPath:    "/dev/null/volume/device",
			// 			TaskPath:    "/dev/sda",
			// 		},
			// 	}
			// 	err := d.ApplyMounts(m)
			// 	must.NoError(t, err)
			// 	must.Eq(t, "sda", d[0].Devname)
			// 	must.Eq(t, "/dev/null/volume/device", d[0].blockDevicePath)
			// })

			// t.Run("does not override device name", func(t *testing.T) {
			// 	d := Disks{{VolumeName: "nomad-volume", Devname: "sdc"}}
			// 	m := []*drivers.MountConfig{
			// 		{
			// 			RequestName: "nomad-volume",
			// 			HostPath:    "/dev/null/volume/device",
			// 			TaskPath:    "/dev/sda",
			// 		},
			// 	}
			// 	err := d.ApplyMounts(m)
			// 	must.NoError(t, err)
			// 	must.Eq(t, "sdc", d[0].Devname)
			// 	must.Eq(t, "/dev/null/volume/device", d[0].blockDevicePath)
			// })

			t.Run("using task path", func(t *testing.T) {
				d := Disks{{VolumeName: "/dev/sda"}}
				m := []*drivers.MountConfig{
					{
						// RequestName: "nomad-volume",
						HostPath: "/dev/null/volume/device",
						TaskPath: "/dev/sda",
					},
				}
				err := d.ApplyMounts(m)
				must.NoError(t, err)
				must.Eq(t, "sda", d[0].Devname)
				must.Eq(t, "/dev/null/volume/device", d[0].blockDevicePath)
			})

			t.Run("does not override device name using task path", func(t *testing.T) {
				d := Disks{{VolumeName: "/dev/sda", Devname: "sdc"}}
				m := []*drivers.MountConfig{
					{
						// RequestName: "nomad-volume",
						HostPath: "/dev/null/volume/device",
						TaskPath: "/dev/sda",
					},
				}
				err := d.ApplyMounts(m)
				must.NoError(t, err)
				must.Eq(t, "sdc", d[0].Devname)
				must.Eq(t, "/dev/null/volume/device", d[0].blockDevicePath)
			})
		})
	})

	t.Run("ResolveImages", func(t *testing.T) {
		validDir := filepath.Join(t.TempDir(), "images")
		must.NoError(t, os.MkdirAll(validDir, 0755))
		validName := "test.img"
		validImage := filepath.Join(validDir, validName)
		f, err := os.OpenFile(validImage, os.O_CREATE, 0644)
		must.NoError(t, err)
		f.Close()
		invalidName := "test.img.bad"
		invalidImage := filepath.Join(t.TempDir(), invalidName)
		f, err = os.OpenFile(invalidImage, os.O_CREATE, 0644)
		must.NoError(t, err)
		f.Close()

		t.Run("valid local path", func(t *testing.T) {
			d := Disks{{Source: &Source{Image: validName}}}
			d.ResolveImages([]string{validDir})
			must.Eq(t, validImage, d[0].Source.Image)
		})

		t.Run("valid absolute path", func(t *testing.T) {
			d := Disks{{Source: &Source{Image: validImage}}}
			d.ResolveImages([]string{validDir})
			must.Eq(t, validImage, d[0].Source.Image)
		})

		t.Run("with invalid local path", func(t *testing.T) {
			d := Disks{{Source: &Source{Image: invalidName}}}
			d.ResolveImages([]string{validDir})
			must.Eq(t, invalidName, d[0].Source.Image)
		})

		t.Run("with invalid absolute path", func(t *testing.T) {
			d := Disks{{Source: &Source{Image: invalidImage}}}
			d.ResolveImages([]string{validDir})
			must.Eq(t, invalidImage, d[0].Source.Image)
		})
	})

	t.Run("Prepare", func(t *testing.T) {
		testDriver := "test-driver"
		testDevname := "test-device-name"
		mockStorage := &mock_storage.StaticStorage{
			DefaultDiskDriverResult:  testDriver,
			GenerateDeviceNameResult: testDevname,
			ImageHandlerResult: &mock_image_tools.StaticImageHandler{
				GetImageSizeResult: 22,
			},
		}

		t.Run("defaults", func(t *testing.T) {
			d := Disks{{}}
			must.NoError(t, d.Prepare(mockStorage))
			disk := d[0]

			must.Eq(t, DiskKindDefault, disk.Kind)
			must.Eq(t, testDriver, disk.Driver)
			must.Eq(t, BusTypeDefault, disk.BusType)
			must.Eq(t, testDevname, disk.Devname)
			must.Eq(t, "testing-image-format", disk.Format)
			must.True(t, disk.Primary) // If only single disk, primary should be automatically set
		})

		t.Run("defaults multiple disks", func(t *testing.T) {
			d := Disks{{}, {}, {}}
			must.NoError(t, d.Prepare(mockStorage))

			for _, disk := range d {
				must.Eq(t, DiskKindDefault, disk.Kind)
				must.Eq(t, testDriver, disk.Driver)
				must.Eq(t, BusTypeDefault, disk.BusType)
				must.Eq(t, testDevname, disk.Devname)
				must.Eq(t, "testing-image-format", disk.Format)
				must.False(t, disk.Primary) // Multiple disks, primary should not be automatically set
			}
		})

		t.Run("defaults cdrom", func(t *testing.T) {
			d := Disks{{Kind: storage.DiskKindCdrom}}
			must.NoError(t, d.Prepare(mockStorage))
			disk := d[0]

			must.Eq(t, storage.DiskKindCdrom, disk.Kind)
			must.Eq(t, testDriver, disk.Driver)
			must.Eq(t, storage.BusTypeIde, disk.BusType)
			must.Eq(t, testDevname, disk.Devname)
			must.Eq(t, "testing-image-format", disk.Format)
			must.False(t, disk.Primary) // Single disk cdrom are not automatically set to primary
		})

		t.Run("nomad volume", func(t *testing.T) {
			d := Disks{{VolumeName: "some-volume"}}
			must.NoError(t, d.Prepare(mockStorage))
			disk := d[0]

			must.Eq(t, DiskKindDefault, disk.Kind)
			must.Eq(t, testDriver, disk.Driver)
			must.Eq(t, BusTypeDefault, disk.BusType)
			must.Eq(t, testDevname, disk.Devname)
			must.Eq(t, "", disk.Format)
			must.True(t, disk.Primary)
		})

		t.Run("with source image", func(t *testing.T) {
			srcContent := "source testing content"
			f, err := os.Create(filepath.Join(t.TempDir(), "source-file"))
			must.NoError(t, err)
			_, err = f.WriteString(srcContent)
			must.NoError(t, err)
			f.Close()
			srcPath := f.Name()

			t.Run("disk size is set when unset", func(t *testing.T) {
				d := Disks{{Format: "testing", Source: &Source{Image: srcPath, Format: "testing"}}}
				must.NoError(t, d.Prepare(mockStorage))
				disk := d[0]

				must.Eq(t, fmt.Sprintf("%d", len(srcContent)), disk.Size)
			})

			t.Run("disk size not set when set", func(t *testing.T) {
				d := Disks{{Format: "testing", Source: &Source{Image: srcPath, Format: "testing"}, Size: "200"}}
				must.NoError(t, d.Prepare(mockStorage))
				disk := d[0]

				must.Eq(t, "200", disk.Size)
			})

			t.Run("source format is set when unset", func(t *testing.T) {
				d := Disks{{Format: "testing", Source: &Source{Image: srcPath}}}
				must.NoError(t, d.Prepare(mockStorage))
				disk := d[0]

				must.Eq(t, "testing", disk.Format)
			})

			t.Run("source format is not set when set", func(t *testing.T) {
				d := Disks{{Format: "custom-source-format", Source: &Source{Image: srcPath, Format: "custom-source-format"}}}
				must.NoError(t, d.Prepare(mockStorage))
				disk := d[0]

				must.Eq(t, "custom-source-format", disk.Format)
			})

			t.Run("source image is converted to target format", func(t *testing.T) {
				d := Disks{{Format: "testing", Source: &Source{Image: srcPath, Format: "custom-source-format"}}}
				must.NoError(t, d.Prepare(mockStorage))
				disk := d[0]

				must.Eq(t, "testing", disk.Source.Format)
				must.StrHasSuffix(t, ".testing", disk.Source.Image)
			})

			t.Run("when chained", func(t *testing.T) {
				t.Run("uses existing parent volume", func(t *testing.T) {
					format := "testing-format"
					volName, err := generateIdentifier(srcPath, format)
					must.NoError(t, err)
					d := Disks{{Format: "testing-format", Chained: true, Source: &Source{Image: srcPath, Format: "testing-format"}}}
					p := mock_storage.NewMockPool(t)
					defer p.AssertExpectations()
					p.Expect(mock_storage.GetVolume{Name: volName, Result: &storage.Volume{Name: volName}})
					s := &mock_storage.StaticStorage{
						DefaultPoolResult: p,
					}
					must.NoError(t, d.Prepare(s))
					disk := d[0]

					must.Eq(t, volName, disk.Source.Volume)
					must.Eq(t, "", disk.Source.Image)
				})

				t.Run("creates parent volume", func(t *testing.T) {
					format := "testing-format"
					volName, err := generateIdentifier(srcPath, format)
					must.NoError(t, err)
					d := Disks{{Format: "testing-format", Chained: true, Source: &Source{Image: srcPath, Format: "testing-format"}}}
					p := mock_storage.NewMockPool(t)
					defer p.AssertExpectations()
					p.Expect(
						mock_storage.GetVolume{Name: volName, Err: errs.ErrNotFound},
						mock_storage.AddVolume{
							Name: volName,
							Opts: storage.Options{
								Sparse: true,
								Target: storage.Target{
									Format: format,
								},
								Source: storage.Source{
									Path: srcPath,
								},
								Size: uint64(len(srcContent)),
							},
							Result: &storage.Volume{Name: volName},
						},
					)
					s := &mock_storage.StaticStorage{
						DefaultPoolResult: p,
						ImageHandlerResult: &mock_image_tools.StaticImageHandler{
							GetImageSizeResult: 22,
						},
					}
					must.NoError(t, d.Prepare(s))
					disk := d[0]

					must.Eq(t, volName, disk.Source.Volume)
					must.Eq(t, "", disk.Source.Image)
				})
			})
		})
	})

	t.Run("ValidateNomadVolume", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			d := Disk{VolumeName: "nomad-volume", blockDevicePath: "/dev/null"}
			must.NoError(t, d.ValidateNomadVolume())
		})

		t.Run("set format", func(t *testing.T) {
			d := Disk{VolumeName: "nomad-volume", blockDevicePath: "/dev/null", Format: "not-raw"}
			err := d.ValidateNomadVolume()
			must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
			must.ErrorContains(t, err, "format")
		})

		t.Run("set size", func(t *testing.T) {
			d := Disk{VolumeName: "nomad-volume", blockDevicePath: "/dev/null", Size: "2GB"}
			err := d.ValidateNomadVolume()
			must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
			must.ErrorContains(t, err, "size")
		})

		t.Run("sparse enabled", func(t *testing.T) {
			d := Disk{VolumeName: "nomad-volume", blockDevicePath: "/dev/null", Sparse: pointer.Of(true)}
			err := d.ValidateNomadVolume()
			must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
			must.ErrorContains(t, err, "sparse")
		})

		t.Run("set pool", func(t *testing.T) {
			d := Disk{VolumeName: "nomad-volume", blockDevicePath: "/dev/null", Pool: "some-pool"}
			err := d.ValidateNomadVolume()
			must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
			must.ErrorContains(t, err, "pool")
		})

		t.Run("chained enabled", func(t *testing.T) {
			d := Disk{VolumeName: "nomad-volume", blockDevicePath: "/dev/null", Chained: true}
			err := d.ValidateNomadVolume()
			must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
			must.ErrorContains(t, err, "chained")
		})

		t.Run("set source", func(t *testing.T) {
			d := Disk{VolumeName: "nomad-volume", blockDevicePath: "/dev/null", Source: &Source{Volume: "other-volume"}}
			err := d.ValidateNomadVolume()
			must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
			must.ErrorContains(t, err, "source.volume")
		})

		t.Run("unset device path", func(t *testing.T) {
			d := Disk{VolumeName: "nomad-volume"}
			err := d.ValidateNomadVolume()
			must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
			must.ErrorContains(t, err, "missing Nomad volume")
		})
	})

	t.Run("Generate", func(t *testing.T) {
		t.Run("creates volume", func(t *testing.T) {
			d := Disks{{Kind: storage.DiskKindDisk, Driver: "test-driver", Format: storage.DiskFormatRaw,
				Devname: "test-device", BusType: storage.BusTypeSata, Size: "20MB", Source: &Source{Image: "image-path"}}}

			pool := mock_storage.NewMockPool(t)
			pool.ExpectAddVolume(mock_storage.AddVolume{
				Name: "task_test-device.img",
				Opts: storage.Options{
					Chained: false,
					Size:    20000000,
					Target: storage.Target{
						Format: storage.DiskFormatRaw,
					},
					Source: storage.Source{
						Path: "image-path",
					},
				},
				Result: &storage.Volume{Pool: "test-pool", Name: "test-volume"},
			})
			store := &mock_storage.StaticStorage{
				DefaultPoolResult: pool,
			}

			must.NoError(t, d.Generate("task", store))
			must.NotNil(t, d[0].Volume, must.Sprint("expected volume to be set"))

			expectedVolume := &storage.Volume{
				Pool:       "test-pool",
				Name:       "test-volume",
				Kind:       storage.DiskKindDisk,
				Driver:     "test-driver",
				Format:     storage.DiskFormatRaw,
				DeviceName: "test-device",
				BusType:    storage.BusTypeSata,
				Primary:    false,
			}

			must.Eq(t, expectedVolume, d[0].Volume)
		})

		t.Run("nomad volumes", func(t *testing.T) {
			srcContent := "test source content for volume"
			src, err := os.Create(filepath.Join(t.TempDir(), "source"))
			must.NoError(t, err)
			_, err = src.WriteString(srcContent)
			must.NoError(t, err)
			src.Close()
			srcPath := src.Name()

			t.Run("does not create block volumes", func(t *testing.T) {
				d := Disks{{Kind: storage.DiskKindDisk, Driver: "test-driver", VolumeName: "nomad-volume", Format: storage.DiskFormatRaw,
					Devname: "test-device", BusType: storage.BusTypeSata, blockDevicePath: "/dev/null"}}

				pool := mock_storage.NewStaticPool()
				store := &mock_storage.StaticStorage{
					DefaultPoolResult: pool,
				}

				must.NoError(t, d.Generate("task", store))
				must.NotNil(t, d[0].Volume, must.Sprint("expected volume to be set"))

				expectedVolume := &storage.Volume{
					Block:      "/dev/null",
					Name:       "task_test-device.img",
					Kind:       storage.DiskKindDisk,
					Driver:     "test-driver",
					Format:     storage.DiskFormatRaw,
					DeviceName: "test-device",
					BusType:    storage.BusTypeSata,
					Primary:    false,
				}

				must.Eq(t, expectedVolume, d[0].Volume)
				must.Zero(t, pool.CallCount("AddVolume"))
			})

			t.Run("errors if no block device path is set", func(t *testing.T) {
				d := Disks{{Kind: storage.DiskKindDisk, Driver: "test-driver", VolumeName: "nomad-volume", Format: storage.DiskFormatRaw,
					Devname: "test-device", BusType: storage.BusTypeSata}}

				pool := mock_storage.NewStaticPool()
				store := &mock_storage.StaticStorage{
					DefaultPoolResult: pool,
				}

				err := d.Generate("task", store)
				must.ErrorContains(t, err, "not backed by Nomad volume")
			})

			t.Run("it prepares the volume", func(t *testing.T) {
				device, err := os.Create(filepath.Join(t.TempDir(), "device"))
				must.NoError(t, err)
				device.Close()

				d := Disks{{Kind: storage.DiskKindDisk, Driver: "test-driver", VolumeName: "nomad-volume", Format: storage.DiskFormatRaw,
					Devname: "test-device", BusType: storage.BusTypeSata, blockDevicePath: device.Name(), Source: &Source{Image: srcPath,
						Format: storage.DiskFormatRaw}}}
				pool := mock_storage.NewStaticPool()
				store := &mock_storage.StaticStorage{
					DefaultPoolResult: pool,
				}

				err = d.Generate("task", store)
				must.NoError(t, err)

				device, err = os.Open(device.Name())
				must.NoError(t, err)
				dstContent, err := io.ReadAll(device)
				must.NoError(t, err)
				must.Eq(t, srcContent, string(dstContent))
			})

			t.Run("it converts the source image if needed", func(t *testing.T) {
				device, err := os.Create(filepath.Join(t.TempDir(), "device"))
				must.NoError(t, err)
				device.Close()

				// It will use a source file with the format appended to the suffix
				src, err := os.Create(srcPath + ".raw")
				_, err = src.WriteString(srcContent)
				must.NoError(t, err)

				d := Disks{{Kind: storage.DiskKindDisk, Driver: "test-driver", VolumeName: "nomad-volume", Format: storage.DiskFormatRaw,
					Devname: "test-device", BusType: storage.BusTypeSata, blockDevicePath: device.Name(), Source: &Source{Image: srcPath}}}
				pool := mock_storage.NewStaticPool()
				imageHandler := mock_image_tools.NewStaticImageHandler()
				store := &mock_storage.StaticStorage{
					ImageHandlerResult: imageHandler,
					DefaultPoolResult:  pool,
				}

				err = d.Generate("task", store)
				must.NoError(t, err)

				device, err = os.Open(device.Name())
				must.NoError(t, err)
				dstContent, err := io.ReadAll(device)
				must.NoError(t, err)
				must.Eq(t, srcContent, string(dstContent))

				// Check image handler converted the image
				must.One(t, imageHandler.CallCount("ConvertImage"))
			})
		})
	})

	t.Run("Volumes", func(t *testing.T) {
		t.Run("no disks", func(t *testing.T) {
			d := NewDisks()
			must.Len(t, 0, d.Volumes())
		})

		t.Run("disks with no volumes", func(t *testing.T) {
			d := Disks{{}, {}, {}}
			must.Len(t, 0, d.Volumes())
		})

		t.Run("disks with volumes", func(t *testing.T) {
			expectedVolumes := []storage.Volume{{Name: "test1"}, {Name: "test2"}}
			d := Disks{{Volume: &expectedVolumes[0]}, {Volume: &expectedVolumes[1]}}
			must.Eq(t, expectedVolumes, d.Volumes())
		})

		t.Run("some disks with volumes", func(t *testing.T) {
			expectedVolumes := []storage.Volume{{Name: "test1"}, {Name: "test2"}}
			d := Disks{{Volume: &expectedVolumes[0]}, {Volume: &expectedVolumes[1]}, {}}
			must.Eq(t, expectedVolumes, d.Volumes())
		})
	})

	t.Run("Validate", func(t *testing.T) {
		t.Run("ok - empty disk", func(t *testing.T) {
			d := Disks{{Format: "raw", Size: "200", Devname: "sda", Kind: storage.DiskKindDisk,
				BusType: storage.BusTypeVirtio, Primary: true}}
			err := d.Validate(mock_storage.NewStaticStorage(), ValidationOptions{})
			must.NoError(t, err)
		})

		t.Run("ok - no capacity", func(t *testing.T) {
			d := Disks{{Format: "raw", Size: "0", Devname: "sda", Kind: storage.DiskKindDisk,
				BusType: storage.BusTypeVirtio, Primary: true}}
			err := d.Validate(mock_storage.NewStaticStorage(), ValidationOptions{})
			must.NoError(t, err)
		})

		t.Run("source", func(t *testing.T) {
			opts := ValidationOptions{AllowedPaths: []string{t.TempDir()}}
			imagePath := filepath.Join(opts.AllowedPaths[0], "image")
			imageContent := "test content"
			img, err := os.Create(imagePath)
			must.NoError(t, err)
			_, err = img.WriteString(imageContent)
			must.NoError(t, err)
			must.NoError(t, img.Close())

			volumeName := "test-volume"

			t.Run("image", func(t *testing.T) {
				t.Run("ok", func(t *testing.T) {
					d := Disks{{Format: "raw", Size: "100", Devname: "sda", Kind: storage.DiskKindDisk,
						BusType: storage.BusTypeVirtio, Primary: true, Source: &Source{Image: imagePath, Format: "raw"}}}
					must.NoError(t, d.Validate(mock_storage.NewStaticStorage(), opts))
				})

				t.Run("missing format", func(t *testing.T) {
					d := Disks{{Format: "raw", Size: "100", Devname: "sda", Kind: storage.DiskKindDisk,
						BusType: storage.BusTypeVirtio, Primary: true, Source: &Source{Image: imagePath}}}
					err := d.Validate(mock_storage.NewStaticStorage(), opts)
					must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
					must.ErrorContains(t, err, "format")
				})

				t.Run("path does not exist", func(t *testing.T) {
					d := Disks{{Format: "raw", Size: "100", Devname: "sda", Kind: storage.DiskKindDisk,
						BusType: storage.BusTypeVirtio, Primary: true, Source: &Source{Image: filepath.Join(t.TempDir(), "image"), Format: "raw"}}}
					err := d.Validate(mock_storage.NewStaticStorage(), opts)
					must.ErrorIs(t, err, ErrPathNotFound)
				})

				t.Run("path is not allowed", func(t *testing.T) {
					d := Disks{{Format: "raw", Size: "100", Devname: "sda", Kind: storage.DiskKindDisk,
						BusType: storage.BusTypeVirtio, Primary: true, Source: &Source{Image: filepath.Join(t.TempDir(), "image"), Format: "raw"}}}
					err := d.Validate(mock_storage.NewStaticStorage(), ValidationOptions{})
					must.ErrorIs(t, err, ErrDisallowedPath)
				})

				t.Run("disk is too small", func(t *testing.T) {
					d := Disks{{Format: "raw", Size: "1", Devname: "sda", Kind: storage.DiskKindDisk,
						BusType: storage.BusTypeVirtio, Primary: true, Source: &Source{Image: imagePath, Format: "raw"}}}
					err := d.Validate(mock_storage.NewStaticStorage(), opts)
					must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
					must.ErrorContains(t, err, "size of disk must")
				})
			})

			t.Run("volume", func(t *testing.T) {
				t.Run("ok", func(t *testing.T) {
					pool := mock_storage.NewMockPool(t).Expect(
						mock_storage.GetVolume{
							Name: volumeName,
							Result: &storage.Volume{
								Size: 100,
							},
						},
					)
					defer pool.AssertExpectations()
					d := Disks{{Format: "raw", Size: "200", Devname: "sda", Kind: storage.DiskKindDisk,
						BusType: storage.BusTypeVirtio, Primary: true, Source: &Source{Volume: volumeName, Format: "raw"}}}
					err := d.Validate(&mock_storage.StaticStorage{DefaultPoolResult: pool}, opts)
					must.NoError(t, err)
				})

				t.Run("disk is too small", func(t *testing.T) {
					pool := mock_storage.NewMockPool(t).Expect(
						mock_storage.GetVolume{
							Name: volumeName,
							Result: &storage.Volume{
								Size: 500,
							},
						},
					)
					defer pool.AssertExpectations()
					d := Disks{{Format: "raw", Size: "200", Devname: "sda", Kind: storage.DiskKindDisk,
						BusType: storage.BusTypeVirtio, Primary: true, Source: &Source{Volume: volumeName, Format: "raw"}}}
					err := d.Validate(&mock_storage.StaticStorage{DefaultPoolResult: pool}, opts)
					must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
					must.ErrorContains(t, err, "size of disk must")
				})
			})

			t.Run("image and volume defined", func(t *testing.T) {
				d := Disks{{Format: "raw", Size: "200", Devname: "sda", Kind: storage.DiskKindDisk,
					BusType: storage.BusTypeVirtio, Primary: true, Source: &Source{Volume: volumeName, Image: imagePath, Format: "raw"}}}
				err := d.Validate(mock_storage.NewStaticStorage(), opts)
				must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
				must.ErrorContains(t, err, "source.volume and source.image")
			})
		})

		t.Run("size", func(t *testing.T) {
			t.Run("ok bytes", func(t *testing.T) {
				d := Disks{{Format: "raw", Size: "200", Devname: "sda", Kind: storage.DiskKindDisk,
					BusType: storage.BusTypeVirtio, Primary: true}}
				must.NoError(t, d.Validate(mock_storage.NewStaticStorage(), ValidationOptions{}))
			})

			t.Run("ok gigabytes", func(t *testing.T) {
				d := Disks{{Format: "raw", Size: "200GB", Devname: "sda", Kind: storage.DiskKindDisk,
					BusType: storage.BusTypeVirtio, Primary: true}}
				must.NoError(t, d.Validate(mock_storage.NewStaticStorage(), ValidationOptions{}))
			})

			t.Run("ok gibibytes", func(t *testing.T) {
				d := Disks{{Format: "raw", Size: "200GiB", Devname: "sda", Kind: storage.DiskKindDisk,
					BusType: storage.BusTypeVirtio, Primary: true}}
				must.NoError(t, d.Validate(mock_storage.NewStaticStorage(), ValidationOptions{}))
			})

			t.Run("unknown suffix", func(t *testing.T) {
				d := Disks{{Format: "raw", Size: "200G", Devname: "sda", Kind: storage.DiskKindDisk,
					BusType: storage.BusTypeVirtio, Primary: true}}
				err := d.Validate(mock_storage.NewStaticStorage(), ValidationOptions{})
				must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
				must.ErrorContains(t, err, "size value")
			})

			t.Run("not a size", func(t *testing.T) {
				d := Disks{{Format: "raw", Size: "hello world", Devname: "sda", Kind: storage.DiskKindDisk,
					BusType: storage.BusTypeVirtio, Primary: true}}
				err := d.Validate(mock_storage.NewStaticStorage(), ValidationOptions{})
				must.ErrorIs(t, err, errs.ErrInvalidConfiguration)
				must.ErrorContains(t, err, "size value")
			})
		})

		t.Run("missing required", func(t *testing.T) {
			d := Disks{{}}
			err := d.Validate(mock_storage.NewStaticStorage(), ValidationOptions{})
			must.ErrorIs(t, err, errs.ErrMissingAttribute)
			must.ErrorContains(t, err, "bus_type")
			must.ErrorContains(t, err, "kind")
			must.ErrorContains(t, err, "devname")
			must.ErrorContains(t, err, "format")
			must.ErrorContains(t, err, "size")
			must.ErrorContains(t, err, "primary")
		})
	})
}

func TestConfig(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		expected := Disks{
			{
				Pool: "test-pool",
				Kind: "disk",
				Size: "1GiB",
				Source: &Source{
					Image: "http://example.com/task.img",
				},
			},
		}
		parser := hclutils.NewConfigParser(configSpec)
		validHcl := `
config {
  disk {
	pool = "test-pool"
    kind = "disk"
    size = "1GiB"
    source {
      image = "http://example.com/task.img"
    }
  }
}`
		var disks Disks
		parser.ParseHCL(t, validHcl, &disks)
		must.Eq(t, expected, disks)
	})

	t.Run("valid - multiple", func(t *testing.T) {
		expected := Disks{
			{
				Pool: "test-pool",
				Kind: "cdrom",
				Source: &Source{
					Image: "http://example.com/cd.img",
				},
			},
			{
				Pool: "test-pool",
				Kind: "disk",
				Size: "1GiB",
				Source: &Source{
					Image: "http://example.com/task.img",
				},
			},
		}
		parser := hclutils.NewConfigParser(configSpec)
		validHcl := `
config {
  disk {
	pool = "test-pool"
    kind = "disk"
    size = "1GiB"
    source {
      image = "http://example.com/task.img"
    }
  }

  disk {
    pool = "test-pool"
    kind = "cdrom"
    source {
      image = "http://example.com/cd.img"
    }
  }
}`
		var disks Disks
		parser.ParseHCL(t, validHcl, &disks)
		must.Eq(t, expected, disks)
	})
}

type poolValidator struct {
	*mock_storage.StaticPool
	ValidateDiskResult error
}

func (p *poolValidator) ValidateDisk(*Disk) error {
	return p.ValidateDiskResult
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package disks

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

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

		t.Run("includes size if available", func(t *testing.T) {
			d := NewDisks()
			path := filepath.Join(t.TempDir(), "testing.iso")
			f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0666)
			must.NoError(t, err)
			_, err = f.Write([]byte("test"))
			must.NoError(t, err)
			f.Close()

			d = d.ApplyCloudInit(path)
			must.Len(t, 1, d)
			must.Eq(t, "4", d[0].Size)
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

	t.Run("SetDefaults", func(t *testing.T) {
		mockStorage := &mock_storage.StaticStorage{
			DefaultDiskDriverResult:  "test-driver",
			GenerateDeviceNameResult: "test-device-name",
		}

		t.Run("sets defaults", func(t *testing.T) {
			d := Disks{{Kind: DiskKindCdrom}}
			d.SetDefaults(mockStorage)
			expected := &Disk{
				Kind:    DiskKindCdrom,
				Driver:  "test-driver",
				BusType: BusTypeIde,
				Devname: "test-device-name",
				Format:  "testing-image-format",
				Primary: false,
			}

			must.Eq(t, expected, d[0])
		})

		t.Run("does not override", func(t *testing.T) {
			disk := &Disk{
				Kind:    DiskKindDisk,
				Driver:  "custom-driver",
				BusType: BusTypeSata,
				Format:  DiskFormatQcow2,
				Devname: "custom-device-name",
				Primary: true,
			}
			var expected Disk
			expected = *disk

			d := Disks{disk}
			d.SetDefaults(mockStorage)
			must.Eq(t, &expected, d[0])
		})

		t.Run("set primary if only one disk", func(t *testing.T) {
			d := Disks{{Kind: DiskKindCdrom}, {Kind: DiskKindDisk}}
			d.SetDefaults(mockStorage)

			must.True(t, d[1].Primary)
		})

		t.Run("does not set primary if multiple disks", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk}, {Kind: DiskKindCdrom}, {Kind: DiskKindDisk}}
			d.SetDefaults(mockStorage)

			for _, disk := range d {
				must.False(t, disk.Primary, must.Sprint("disk should not be marked primary"))
			}
		})
	})

	t.Run("Validate", func(t *testing.T) {
		mockStorage := &mock_storage.StaticStorage{
			DefaultDiskDriverResult:  "test-driver",
			GenerateDeviceNameResult: "test-device-name",
		}
		imageDir := filepath.Join(t.TempDir(), "images")
		must.NoError(t, os.MkdirAll(imageDir, 0755))
		validImage := filepath.Join(imageDir, "test.img")
		f, err := os.OpenFile(validImage, os.O_CREATE, 0644)
		must.NoError(t, err)
		f.Close()
		mockStore := mock_storage.NewStaticStorage()

		t.Run("valid", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk, Size: "1"}}
			d.SetDefaults(mockStorage)

			must.NoError(t, d.Validate(mockStore, ValidationOptions{}))
		})

		t.Run("valid with source image", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk, Source: &Source{Image: validImage}}}
			d.SetDefaults(mockStorage)

			must.NoError(t, d.Validate(mockStore, ValidationOptions{AllowedPaths: []string{imageDir}}))
		})

		t.Run("valid with invalid source image", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk, Size: "1", Source: &Source{Image: "unknown.img"}}}
			d.SetDefaults(mockStorage)

			must.ErrorIs(t, d.Validate(mockStore, ValidationOptions{AllowedPaths: []string{imageDir}}), ErrDisallowedPath)
		})

		t.Run("invalid", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk}}
			must.ErrorIs(t, d.Validate(mockStore, ValidationOptions{}), ErrMissingAttribute)
		})

		t.Run("missing bus type", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk}}
			d.SetDefaults(mockStorage)
			d[0].BusType = ""

			must.ErrorContains(t, d.Validate(mockStore, ValidationOptions{}), "bus_type")
		})

		t.Run("missing kind", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk}}
			d.SetDefaults(mockStorage)
			d[0].Kind = ""

			must.ErrorContains(t, d.Validate(mockStore, ValidationOptions{}), "kind")
		})

		t.Run("missing device name", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk}}
			d.SetDefaults(mockStorage)
			d[0].Devname = ""

			must.ErrorContains(t, d.Validate(mockStore, ValidationOptions{}), "devname")
		})

		t.Run("missing size", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk}}
			d.SetDefaults(mockStorage)
			d[0].Size = ""

			must.ErrorContains(t, d.Validate(mockStore, ValidationOptions{}), "size")
		})

		t.Run("missing format", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk}}
			d.SetDefaults(mockStorage)
			d[0].Format = ""

			must.ErrorContains(t, d.Validate(mockStore, ValidationOptions{}), "format")
		})

		t.Run("source volume and image set", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk, Source: &Source{Image: "image-path", Volume: "parent"}}}
			d.SetDefaults(mockStorage)

			err := d.Validate(mockStore, ValidationOptions{})
			must.ErrorIs(t, err, ErrInvalidConfiguration)
			must.ErrorContains(t, err, "mutually exclusive")
		})

		t.Run("primaries", func(t *testing.T) {
			t.Run("no primary defined", func(t *testing.T) {
				d := Disks{{Kind: DiskKindDisk}, {Kind: DiskKindDisk}}
				d.SetDefaults(mockStorage)

				must.ErrorIs(t, d.Validate(mockStore, ValidationOptions{}), ErrNoPrimary)
			})

			t.Run("multiple primaries defined", func(t *testing.T) {
				d := Disks{{Kind: DiskKindDisk, Primary: true}, {Kind: DiskKindDisk}, {Kind: DiskKindDisk, Primary: true}}
				d.SetDefaults(mockStorage)

				err := d.Validate(mockStore, ValidationOptions{})
				must.ErrorIs(t, err, ErrMultiplePrimary)
				must.ErrorContains(t, err, "disks: 1, 3")
			})
		})

		t.Run("uses pool validator if available", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk}}
			testErr := errors.New("test validation error")
			pool := &poolValidator{StaticPool: mock_storage.NewStaticPool(), ValidateDiskResult: testErr}
			s := &mock_storage.StaticStorage{DefaultPoolResult: pool}

			err := d.Validate(s, ValidationOptions{})
			must.ErrorIs(t, err, testErr)
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
			must.ErrorIs(t, err, ErrInvalidConfiguration)
			must.ErrorContains(t, err, "format")
		})

		t.Run("set size", func(t *testing.T) {
			d := Disk{VolumeName: "nomad-volume", blockDevicePath: "/dev/null", Size: "2GB"}
			err := d.ValidateNomadVolume()
			must.ErrorIs(t, err, ErrInvalidConfiguration)
			must.ErrorContains(t, err, "size")
		})

		t.Run("sparse enabled", func(t *testing.T) {
			d := Disk{VolumeName: "nomad-volume", blockDevicePath: "/dev/null", Sparse: pointer.Of(true)}
			err := d.ValidateNomadVolume()
			must.ErrorIs(t, err, ErrInvalidConfiguration)
			must.ErrorContains(t, err, "sparse")
		})

		t.Run("set pool", func(t *testing.T) {
			d := Disk{VolumeName: "nomad-volume", blockDevicePath: "/dev/null", Pool: "some-pool"}
			err := d.ValidateNomadVolume()
			must.ErrorIs(t, err, ErrInvalidConfiguration)
			must.ErrorContains(t, err, "pool")
		})

		t.Run("chained enabled", func(t *testing.T) {
			d := Disk{VolumeName: "nomad-volume", blockDevicePath: "/dev/null", Chained: true}
			err := d.ValidateNomadVolume()
			must.ErrorIs(t, err, ErrInvalidConfiguration)
			must.ErrorContains(t, err, "chained")
		})

		t.Run("set source", func(t *testing.T) {
			d := Disk{VolumeName: "nomad-volume", blockDevicePath: "/dev/null", Source: &Source{Volume: "other-volume"}}
			err := d.ValidateNomadVolume()
			must.ErrorIs(t, err, ErrInvalidConfiguration)
			must.ErrorContains(t, err, "source.volume")
		})

		t.Run("unset device path", func(t *testing.T) {
			d := Disk{VolumeName: "nomad-volume"}
			err := d.ValidateNomadVolume()
			must.ErrorIs(t, err, ErrInvalidConfiguration)
			must.ErrorContains(t, err, "missing Nomad volume")
		})
	})

	t.Run("Prepare", func(t *testing.T) {
		t.Run("creates volume", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk, Driver: "test-driver", Format: DiskFormatRaw,
				Devname: "test-device", BusType: BusTypeSata, Size: "20MB", Source: &Source{Image: "image-path"}}}

			pool := mock_storage.NewMockPool(t)
			pool.ExpectAddVolume(mock_storage.AddVolume{
				Name: "task_test-device.img",
				Opts: storage.Options{
					Chained: false,
					Size:    20000000,
					Target: storage.Target{
						Format: DiskFormatRaw,
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

			must.NoError(t, d.Prepare("task", store))
			must.NotNil(t, d[0].Volume, must.Sprint("expected volume to be set"))

			expectedVolume := &storage.Volume{
				Pool:       "test-pool",
				Name:       "test-volume",
				Kind:       DiskKindDisk,
				Driver:     "test-driver",
				Format:     DiskFormatRaw,
				DeviceName: "test-device",
				BusType:    BusTypeSata,
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
				d := Disks{{Kind: DiskKindDisk, Driver: "test-driver", VolumeName: "nomad-volume", Format: DiskFormatRaw,
					Devname: "test-device", BusType: BusTypeSata, blockDevicePath: "/dev/null"}}

				pool := mock_storage.NewStaticPool()
				store := &mock_storage.StaticStorage{
					DefaultPoolResult: pool,
				}

				must.NoError(t, d.Prepare("task", store))
				must.NotNil(t, d[0].Volume, must.Sprint("expected volume to be set"))

				expectedVolume := &storage.Volume{
					Block:      "/dev/null",
					Name:       "task_test-device.img",
					Kind:       DiskKindDisk,
					Driver:     "test-driver",
					Format:     DiskFormatRaw,
					DeviceName: "test-device",
					BusType:    BusTypeSata,
					Primary:    false,
				}

				must.Eq(t, expectedVolume, d[0].Volume)
				must.Zero(t, pool.CallCount("AddVolume"))
			})

			t.Run("errors if no block device path is set", func(t *testing.T) {
				d := Disks{{Kind: DiskKindDisk, Driver: "test-driver", VolumeName: "nomad-volume", Format: DiskFormatRaw,
					Devname: "test-device", BusType: BusTypeSata}}

				pool := mock_storage.NewStaticPool()
				store := &mock_storage.StaticStorage{
					DefaultPoolResult: pool,
				}

				err := d.Prepare("task", store)
				must.ErrorContains(t, err, "not backed by Nomad volume")
			})

			t.Run("it prepares the volume", func(t *testing.T) {
				device, err := os.Create(filepath.Join(t.TempDir(), "device"))
				must.NoError(t, err)
				device.Close()

				d := Disks{{Kind: DiskKindDisk, Driver: "test-driver", VolumeName: "nomad-volume", Format: DiskFormatRaw,
					Devname: "test-device", BusType: BusTypeSata, blockDevicePath: device.Name(), Source: &Source{Image: srcPath,
						Format: DiskFormatRaw}}}
				pool := mock_storage.NewStaticPool()
				store := &mock_storage.StaticStorage{
					DefaultPoolResult: pool,
				}

				err = d.Prepare("task", store)
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

				d := Disks{{Kind: DiskKindDisk, Driver: "test-driver", VolumeName: "nomad-volume", Format: DiskFormatRaw,
					Devname: "test-device", BusType: BusTypeSata, blockDevicePath: device.Name(), Source: &Source{Image: srcPath}}}
				pool := mock_storage.NewStaticPool()
				imageHandler := mock_image_tools.NewStaticImageHandler()
				store := &mock_storage.StaticStorage{
					ImageHandlerResult: imageHandler,
					DefaultPoolResult:  pool,
				}

				err = d.Prepare("task", store)
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

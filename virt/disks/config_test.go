// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package disks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/nomad-driver-virt/storage"
	mock_storage "github.com/hashicorp/nomad-driver-virt/testutil/mock/storage"
	"github.com/hashicorp/nomad/helper/pluginutils/hclutils"
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

		t.Run("does nothing if nil", func(t *testing.T) {
			d := NewDisks()
			d = nil
			d = d.ApplyCloudInit("/dev/null/img.iso")
			must.Nil(t, d)
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

		t.Run("does nothing if nil", func(t *testing.T) {
			d := NewDisks()
			d = nil
			d = d.CompatAddImage("/dev/null/img.iso", 25, false)
			must.Nil(t, d)
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
				Format:  DiskFormatDefault,
				Devname: "test-device-name",
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

		t.Run("valid", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk}}
			d.SetDefaults(mockStorage)

			must.NoError(t, d.Validate(ValidationOptions{}))
		})

		t.Run("valid with source image", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk, Source: &Source{Image: validImage}}}
			d.SetDefaults(mockStorage)

			must.NoError(t, d.Validate(ValidationOptions{AllowedPaths: []string{imageDir}}))
		})

		t.Run("valid with invalid source image", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk, Source: &Source{Image: "unknown.img"}}}
			d.SetDefaults(mockStorage)

			must.ErrorIs(t, d.Validate(ValidationOptions{AllowedPaths: []string{imageDir}}), ErrDisallowedPath)
		})

		t.Run("invalid", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk}}
			must.ErrorIs(t, d.Validate(ValidationOptions{}), ErrMissingAttribute)
		})

		t.Run("missing bus type", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk}}
			d.SetDefaults(mockStorage)
			d[0].BusType = ""

			must.ErrorContains(t, d.Validate(ValidationOptions{}), "BusType")
		})

		t.Run("missing kind", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk}}
			d.SetDefaults(mockStorage)
			d[0].Kind = ""

			must.ErrorContains(t, d.Validate(ValidationOptions{}), "Kind")
		})

		t.Run("missing device name", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk}}
			d.SetDefaults(mockStorage)
			d[0].Devname = ""

			must.ErrorContains(t, d.Validate(ValidationOptions{}), "Devname")
		})

		t.Run("no primary defined", func(t *testing.T) {
			d := Disks{{Kind: DiskKindDisk}, {Kind: DiskKindDisk}}
			d.SetDefaults(mockStorage)

			must.ErrorIs(t, d.Validate(ValidationOptions{}), ErrNoPrimary)
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
					Size:    "20MB",
					Target: storage.Target{
						Format: DiskFormatRaw,
					},
					Source: storage.Source{
						Path: "image-path",
					},
				},
				Result: &storage.Volume{Pool: "test-pool", Name: "test-volume"},
			})

			must.NoError(t, d.Prepare("task", pool))
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

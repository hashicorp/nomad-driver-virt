// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"testing"

	"github.com/hashicorp/nomad-driver-virt/providers/libvirt"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"github.com/hashicorp/nomad-driver-virt/virt/disks"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/helper/pluginutils/hclutils"
	"github.com/shoenig/test/must"
)

func TestConfig_Task(t *testing.T) {
	t.Parallel()

	parser := hclutils.NewConfigParser(taskConfigSpec)

	expectedHostname := "test-hostname"
	expectedImg := "/path/to/image/here"
	expectedUserData := "/path/to/user/data"
	expectedCmds := []string{"redis"}
	expectedDefaultUserSSHKey := "ssh-ed25519 testtesttest..."
	expectedDefaultUserPassword := "password"
	expectedUseThinCopy := true
	expectedARCH := "arm78"
	expectedMachine := "R2D2"

	validHCL := `
config {
	image = "/path/to/image/here"
	primary_disk_size = 26000
	cmds = ["redis"]
	hostname = "test-hostname"
	user_data = "/path/to/user/data"
	default_user_authorized_ssh_key =  "ssh-ed25519 testtesttest..."
	default_user_password = "password"
	use_thin_copy = true
	os {
		arch = "arm78"
		machine = "R2D2"
	}
}
`

	var tc *TaskConfig
	parser.ParseHCL(t, validHCL, &tc)
	must.SliceContainsAll(t, expectedCmds, tc.CMDs)
	must.StrContains(t, expectedImg, tc.ImagePath)
	must.Eq(t, expectedUseThinCopy, tc.UseThinCopy)
	must.StrContains(t, expectedDefaultUserSSHKey, tc.DefaultUserSSHKey)
	must.StrContains(t, expectedDefaultUserPassword, tc.DefaultUserPassword)
	must.StrContains(t, expectedHostname, tc.Hostname)
	must.StrContains(t, expectedUserData, tc.UserData)
	must.StrContains(t, expectedARCH, tc.OS.Arch)
	must.StrContains(t, expectedMachine, tc.OS.Machine)
	must.Eq(t, 26000, tc.PrimaryDiskSize)
}

func TestConfig_Plugin(t *testing.T) {
	t.Parallel()

	parser := hclutils.NewConfigParser(configSpec)

	t.Run("ok", func(t *testing.T) {
		expected := &Config{
			Provider: &Provider{
				Libvirt: &libvirt.Config{
					URI:      "qemu:///user",
					User:     "test-user",
					Password: "test-password",
				},
			},
			ImagePaths: []string{"/path/one", "/path/two"},
			DataDir:    "/path/to/blah",
			StoragePools: &storage.Config{
				Default: "test-pool",
				Directory: map[string]storage.Directory{
					"test-pool": {Path: "/test/pool/path"},
				},
				Ceph: map[string]storage.Ceph{
					"test-ceph-pool": {
						Pool:  "ceph-pool",
						Hosts: []string{"localhost:6789"},
						Authentication: storage.Authentication{
							Username: "test-user",
							Secret:   "test-password",
						},
					},
				},
			},
		}

		validHCL := `
config {
	data_dir = "/path/to/blah"
	image_paths = ["/path/one", "/path/two"]
	provider "libvirt" {
		uri = "qemu:///user"
		user = "test-user"
		password = "test-password"
	}
	storage_pools {
        default = "test-pool"

		directory "test-pool" {
			path = "/test/pool/path"
		}
		ceph "test-ceph-pool" {
			pool = "ceph-pool"
			hosts = ["localhost:6789"],
			authentication {
				username = "test-user"
				secret = "test-password"
			}
		}
	}
}
`
		var result *Config
		parser.ParseHCL(t, validHCL, &result)
		must.Eq(t, expected, result)
	})

	t.Run("compat", func(t *testing.T) {
		expected := &Config{
			Emulator: &Emulator{
				URI:      "qemu:///user",
				User:     "test-user",
				Password: "test-password",
			},
			Provider: &Provider{
				Libvirt: &libvirt.Config{
					URI:      "qemu:///user",
					User:     "test-user",
					Password: "test-password",
				},
			},
			ImagePaths: []string{"/path/one", "/path/two"},
			DataDir:    "/path/to/blah",
			StoragePools: &storage.Config{
				Directory: map[string]storage.Directory{
					"virt-sp": {Path: "/path/to/blah/virt-sp"},
				},
				Ceph: make(map[string]storage.Ceph),
			},
		}

		validHCL := `
config {
	data_dir = "/path/to/blah"
	image_paths = ["/path/one", "/path/two"]
    emulator {
		uri = "qemu:///user"
		user = "test-user"
		password = "test-password"
    
    }
}
`
		var result *Config
		parser.ParseHCL(t, validHCL, &result)
		result.Compat()
		must.Eq(t, expected, result)
	})
}

func Test_taskConfigSpec(t *testing.T) {
	testCases := []struct {
		name           string
		inputConfig    string
		expectedOutput TaskConfig
	}{
		{
			name: "network interface with required",
			inputConfig: `
config {
	image = "/path/to/image/here"
	primary_disk_size = 26000
	os {
		arch    = "x86_64"
		machine = "pc-i440fx-jammy"
	}
	network_interface {
		bridge {
			name  = "virbr0"
			ports = ["ssh"]
		}
	}
}
`,
			expectedOutput: TaskConfig{
				ImagePath:       "/path/to/image/here",
				PrimaryDiskSize: 26000,
				OS: &OS{
					Arch:    "x86_64",
					Machine: "pc-i440fx-jammy",
				},
				Disks: disks.NewDisks(),
				NetworkInterfacesConfig: []*net.NetworkInterfaceConfig{
					{
						Bridge: &net.NetworkInterfaceBridgeConfig{
							Name:  "virbr0",
							Ports: []string{"ssh"},
						},
					},
				}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			var actualOutput TaskConfig

			hclutils.NewConfigParser(taskConfigSpec).ParseHCL(t, tc.inputConfig, &actualOutput)
			must.Eq(t, tc.expectedOutput, actualOutput)
		})
	}
}

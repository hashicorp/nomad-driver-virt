// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"github.com/hashicorp/nomad-driver-virt/virt/disks"
	"testing"

	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/helper/pluginutils/hclutils"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/drivers/fsisolation"
	"github.com/shoenig/test/must"
)

func Test_capabilities(t *testing.T) {
	t.Parallel()

	expectedCapabilities := drivers.Capabilities{
		SendSignals:          false,
		Exec:                 false,
		DisableLogCollection: true,
		FSIsolation:          fsisolation.Image,
		NetIsolationModes:    []drivers.NetIsolationMode{drivers.NetIsolationModeHost},
		MustInitiateNetwork:  false,
		MountConfigs:         drivers.MountConfigSupportNone,
	}
	must.Eq(t, &expectedCapabilities, capabilities)
}

func TestConfig_Task(t *testing.T) {
	t.Parallel()

	parser := hclutils.NewConfigParser(taskConfigSpec)

	expectedHostname := "test-hostname"
	expectedUserData := "/path/to/user/data"
	expectedCmds := []string{"redis"}
	expectedDefaultUserSSHKey := "ssh-ed25519 testtesttest..."
	expectedDefaultUserPassword := "password"
	expectedARCH := "arm78"
	expectedMachine := "R2D2"
	expectedFileDiskLabel := "vda"
	expectedFileDiskFmt := "qcow2"
	expectedFileDiskPath := "/path/to/image"
	expectedRbdDiskLabel := "vdb"
	expectedRbdDiskName := "pool/image"

	validHCL := `
  config {
	cmds = ["redis"]
	hostname = "test-hostname"
	user_data = "/path/to/user/data"
	default_user_authorized_ssh_key =  "ssh-ed25519 testtesttest..."
	default_user_password = "password"
	os {
		arch = "arm78"
		machine = "R2D2"
	}
	disks {
		file "vda" {
			path = "/path/to/image"
			fmt = "qcow2"
		}
		rbd "vdb" {
			name = "pool/image"
			host "ceph01.example.org" {
				port = 6789
			}
			host "ceph02.example.org" {}
		}
	}
  }
`

	var tc *TaskConfig
	parser.ParseHCL(t, validHCL, &tc)
	println(tc)
	must.SliceContainsAll(t, expectedCmds, tc.CMDs)
	must.StrContains(t, expectedDefaultUserSSHKey, tc.DefaultUserSSHKey)
	must.StrContains(t, expectedDefaultUserPassword, tc.DefaultUserPassword)
	must.StrContains(t, expectedHostname, tc.Hostname)
	must.StrContains(t, expectedUserData, tc.UserData)
	must.StrContains(t, expectedARCH, tc.OS.Arch)
	must.StrContains(t, expectedMachine, tc.OS.Machine)
	must.StrContains(t, expectedMachine, tc.OS.Machine)
	must.NotNil(t, tc.DisksConfig)
	must.MapContainsKey(t, *tc.DisksConfig.FileDisksConfig, expectedFileDiskLabel)
	must.StrContains(t, expectedFileDiskFmt, (*tc.DisksConfig.FileDisksConfig)[expectedFileDiskLabel].Fmt)
	must.StrContains(t, expectedFileDiskPath, (*tc.DisksConfig.FileDisksConfig)[expectedFileDiskLabel].Path)
	must.MapContainsKey(t, *tc.DisksConfig.RbdDisksConfig, expectedRbdDiskLabel)
	must.StrContains(t, (*tc.DisksConfig.RbdDisksConfig)[expectedRbdDiskLabel].Name, expectedRbdDiskName)
}

func TestConfig_Plugin(t *testing.T) {
	t.Parallel()

	parser := hclutils.NewConfigParser(configSpec)

	expectedDataDir := "/path/to/blah"
	expectedImgPaths := []string{"/path/one", "/path/two"}
	expectedURI := "qume:///user"
	expectedUser := "test-user"
	expectedPassword := "test-password"

	validHCL := `
  config {
	data_dir = "/path/to/blah"
	image_paths = ["/path/one", "/path/two"]
	emulator {
		uri = "qume:///user"
		user = "test-user"
		password = "test-password"
	}
  }
`

	var cs *Config
	parser.ParseHCL(t, validHCL, &cs)

	must.SliceContainsAll(t, expectedImgPaths, cs.ImagePaths)
	must.StrContains(t, expectedDataDir, cs.DataDir)
	must.StrContains(t, expectedURI, cs.Emulator.URI)
	must.StrContains(t, expectedUser, cs.Emulator.User)
	must.StrContains(t, expectedPassword, cs.Emulator.Password)
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
				OS: &OS{
					Arch:    "x86_64",
					Machine: "pc-i440fx-jammy",
				},
				NetworkInterfacesConfig: []*net.NetworkInterfaceConfig{
					{
						Bridge: &net.NetworkInterfaceBridgeConfig{
							Name:  "virbr0",
							Ports: []string{"ssh"},
						},
					},
				},
				DisksConfig: disks.DisksConfig{},
			},
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

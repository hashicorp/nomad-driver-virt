// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
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
  image = "/path/to/image/here"
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
				ImagePath: "/path/to/image/here",
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

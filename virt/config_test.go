// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"testing"

	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/helper/pluginutils/hclutils"
	"github.com/shoenig/test/must"
)

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
  type = "kvm"
  os {
    arch    = "x86_64"
    machine = "pc-i440fx-jammy"
    type    = "qemu"
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
				Disk: []Disk{},
				Type: "kvm",
				OSVariant: OS{
					Arch:    "x86_64",
					Machine: "pc-i440fx-jammy",
					Type:    "qemu",
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

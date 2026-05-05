// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package net

import (
	"errors"
	"testing"

	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	"github.com/hashicorp/nomad/helper/pluginutils/hclutils"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
	"github.com/shoenig/test/must"
)

func TestNetworkInterfaces_Validate(t *testing.T) {
	testCases := []struct {
		name                   string
		inputNetworkInterfaces *NetworkInterfacesConfig
		errorTarget            error
		expectedOutput         error
	}{
		{
			name:                   "nil",
			inputNetworkInterfaces: nil,
			expectedOutput:         nil,
		},
		{
			name:                   "empty list",
			inputNetworkInterfaces: &NetworkInterfacesConfig{},
			expectedOutput:         nil,
		},
		{
			name: "one interface",
			inputNetworkInterfaces: &NetworkInterfacesConfig{
				{
					Bridge: &NetworkInterfaceBridgeConfig{
						Name:  "virbr0",
						Ports: []string{"ssh"},
					},
				},
			},
			expectedOutput: nil,
		},
		{
			name: "two interfaces",
			inputNetworkInterfaces: &NetworkInterfacesConfig{
				{
					Bridge: &NetworkInterfaceBridgeConfig{
						Name:  "virbr0",
						Ports: []string{"ssh"},
					},
				},
				{
					Bridge: &NetworkInterfaceBridgeConfig{
						Name:  "br0",
						Ports: []string{"http"},
					},
				},
			},
			errorTarget:    errs.ErrInvalidConfiguration,
			expectedOutput: errors.New("only one network interface can be configured"),
		},
		{
			name: "no bridge name",
			inputNetworkInterfaces: &NetworkInterfacesConfig{
				{
					Bridge: &NetworkInterfaceBridgeConfig{
						Name:  "",
						Ports: []string{"ssh"},
					},
				},
			},
			errorTarget:    errs.ErrMissingAttribute,
			expectedOutput: errors.New("bridge.name"),
		},
		{
			name: "no macvtap device",
			inputNetworkInterfaces: &NetworkInterfacesConfig{
				{
					Macvtap: &NetworkInterfaceMacvtapConfig{
						Mode: "bridge",
					},
				},
			},
			errorTarget:    errs.ErrMissingAttribute,
			expectedOutput: errors.New("macvtap.device"),
		},
		{
			name: "invalid macvtap mode",
			inputNetworkInterfaces: &NetworkInterfacesConfig{
				{
					Macvtap: &NetworkInterfaceMacvtapConfig{
						Mode:   "unknown",
						Device: "eth0",
					},
				},
			},
			errorTarget:    errs.ErrInvalidConfiguration,
			expectedOutput: errors.New(`macvtap has invalid mode "unknown"; must be one of: bridge, private, vepa, passthrough`),
		},
		{
			name: "macvtap and bridge defined",
			inputNetworkInterfaces: &NetworkInterfacesConfig{
				{
					Macvtap: &NetworkInterfaceMacvtapConfig{
						Mode:   "bridge",
						Device: "eth0",
					},
					Bridge: &NetworkInterfaceBridgeConfig{
						Name: "br0",
					},
				},
			},
			errorTarget:    errs.ErrInvalidConfiguration,
			expectedOutput: errors.New(`bridge and macvtap are mutually exclusive`),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualOutput := tc.inputNetworkInterfaces.Validate()

			if tc.errorTarget != nil {
				must.ErrorIs(t, actualOutput, tc.errorTarget)
			}

			if tc.expectedOutput == nil {
				must.NoError(t, actualOutput)
			} else {
				must.ErrorContains(t, actualOutput, tc.expectedOutput.Error())
			}
		})
	}
}

func Test_InterfaceHCLSpecification(t *testing.T) {

	// Create the spec as it would look from the main driver respective, so we
	// can test properly.
	spec := hclspec.NewObject(map[string]*hclspec.Spec{
		"network_interface": NetworkInterfaceHCLSpec(),
	})

	// Create the task config, which includes the partial config provided by
	// the network config package.
	type TaskConfig struct {
		NetworkInterfacesConfig `codec:"network_interface"`
	}

	testCases := []struct {
		name           string
		inputConfig    string
		expectedOutput TaskConfig
	}{
		{
			name: "full bridge",
			inputConfig: `
config {
  network_interface {
    bridge {
      name  = "virbr0"
      ports = ["ssh"]
    }
  }
}
`,
			expectedOutput: TaskConfig{
				NetworkInterfacesConfig: []*NetworkInterfaceConfig{
					{
						Bridge: &NetworkInterfaceBridgeConfig{
							Name:  "virbr0",
							Ports: []string{"ssh"},
						},
					},
				}},
		},
		{
			name: "bridge no ports",
			inputConfig: `
config {
  network_interface {
    bridge {
      name = "virbr0"
    }
  }
}
`,
			expectedOutput: TaskConfig{
				NetworkInterfacesConfig: []*NetworkInterfaceConfig{
					{
						Bridge: &NetworkInterfaceBridgeConfig{
							Name:  "virbr0",
							Ports: nil,
						},
					},
				}},
		},
		{
			name: "full macvtap",
			inputConfig: `
config {
  network_interface {
    macvtap {
      device  = "eth0"
      mode    = "private"
    }
  }
}
`,
			expectedOutput: TaskConfig{
				NetworkInterfacesConfig: []*NetworkInterfaceConfig{
					{
						Macvtap: &NetworkInterfaceMacvtapConfig{
							Device: "eth0",
							Mode:   MacvtapModePrivate,
						},
					},
				}},
		},
		{
			name: "macvtap without mode",
			inputConfig: `
config {
  network_interface {
    macvtap {
      device  = "eth0"
    }
  }
}
`,
			expectedOutput: TaskConfig{
				NetworkInterfacesConfig: []*NetworkInterfaceConfig{
					{
						Macvtap: &NetworkInterfaceMacvtapConfig{
							Device: "eth0",
							Mode:   MacvtapModeBridge,
						},
					},
				}},
		},
		{
			name:           "no interface",
			inputConfig:    `config {}`,
			expectedOutput: TaskConfig{NetworkInterfacesConfig: []*NetworkInterfaceConfig{}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			var actualOutput TaskConfig

			hclutils.NewConfigParser(spec).ParseHCL(t, tc.inputConfig, &actualOutput)
			must.Eq(t, tc.expectedOutput, actualOutput)
		})
	}
}

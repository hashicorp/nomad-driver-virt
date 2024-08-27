// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package mock

import (
	"fmt"

	"github.com/hashicorp/nomad-driver-virt/virt/shim"
)

// Connect is the primary mock interface that has default values for testing.
// It implements the shim.Connect interface.
type Connect struct{}

func (c *Connect) ListNetworks() ([]string, error) {
	return []string{"default", "routed"}, nil
}

func (c *Connect) LookupNetworkByName(name string) (shim.Network, error) {
	switch name {
	case "default":
		return &ConnectNetwork{
			name:       "default",
			active:     true,
			bridgeName: "virbr0",
		}, nil
	case "routed":
		return &ConnectNetwork{
			name:       "routed",
			active:     false,
			bridgeName: "br0",
		}, nil
	default:
		return nil, fmt.Errorf("unknown network: %q", name)
	}
}

// ConnectEmpty is a secondary mock that can be used to mimic a host where no
// libvirt networks or other resources are available. It implements the
// shim.Connect interface.
type ConnectEmpty struct{}

func (c *ConnectEmpty) ListNetworks() ([]string, error) {
	return []string{}, nil
}

func (c *ConnectEmpty) LookupNetworkByName(name string) (shim.Network, error) {
	return nil, fmt.Errorf("unknown network: %q", name)
}

// ConnectNetwork implements the shim.Network interface for testing.
type ConnectNetwork struct {
	name       string
	active     bool
	bridgeName string
}

func (cn *ConnectNetwork) IsActive() (bool, error) { return cn.active, nil }

func (cn *ConnectNetwork) GetBridgeName() (string, error) { return cn.bridgeName, nil }

// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"fmt"
)

// ConnectMock is the primary mock interface that has default values for
// testing. It implements the ConnectShim interface.
type ConnectMock struct{}

func (cm *ConnectMock) ListNetworks() ([]string, error) {
	return []string{"default", "routed"}, nil
}

func (cm *ConnectMock) LookupNetworkByName(name string) (ConnectNetworkShim, error) {
	switch name {
	case "default":
		return &ConnectNetworkMock{
			name:       "default",
			active:     true,
			bridgeName: "virbr0",
		}, nil
	case "routed":
		return &ConnectNetworkMock{
			name:       "routed",
			active:     false,
			bridgeName: "br0",
		}, nil
	default:
		return nil, fmt.Errorf("unknown network: %q", name)
	}
}

// ConnectMockEmpty is a secondary mock that can be used to mimic a host where
// no libvirt networks or other resources are available. It implements the
// ConnectShim interface.
type ConnectMockEmpty struct{}

func (cme *ConnectMockEmpty) ListNetworks() ([]string, error) {
	return []string{}, nil
}

func (cme *ConnectMockEmpty) LookupNetworkByName(name string) (ConnectNetworkShim, error) {
	return nil, fmt.Errorf("unknown network: %q", name)
}

// ConnectNetworkMock implements the shim.Network interface for testing.
type ConnectNetworkMock struct {
	name       string
	active     bool
	bridgeName string
}

func (cnm *ConnectNetworkMock) IsActive() (bool, error) { return cnm.active, nil }

func (cnm *ConnectNetworkMock) GetBridgeName() (string, error) { return cnm.bridgeName, nil }

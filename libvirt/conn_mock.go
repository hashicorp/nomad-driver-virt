// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"fmt"
	"time"

	"libvirt.org/go/libvirt"
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
			dhcpLeases: []libvirt.NetworkDHCPLease{
				{
					Iface:      "virbr0",
					ExpiryTime: time.Now(),
					Type:       libvirt.IP_ADDR_TYPE_IPV4,
					Mac:        "52:54:00:1c:7c:14",
					IPaddr:     "192.168.122.58",
					Hostname:   "nomad-0ea818bc",
					Clientid:   "ff:08:24:45:0e:00:02:00:00:ab:11:35:ab:f3:c7:ac:54:9e:c8",
				},
			},
		}, nil
	case "routed":
		return &ConnectNetworkMock{
			name:       "routed",
			active:     false,
			bridgeName: "br0",
			dhcpLeases: []libvirt.NetworkDHCPLease{},
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
	dhcpLeases []libvirt.NetworkDHCPLease
}

func (cnm *ConnectNetworkMock) IsActive() (bool, error) { return cnm.active, nil }

func (cnm *ConnectNetworkMock) GetBridgeName() (string, error) { return cnm.bridgeName, nil }

func (cnm *ConnectNetworkMock) GetDHCPLeases() ([]libvirt.NetworkDHCPLease, error) {
	return cnm.dhcpLeases, nil
}

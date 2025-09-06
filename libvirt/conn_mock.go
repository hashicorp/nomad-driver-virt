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
					ExpiryTime: time.Now().Add(1 * time.Hour),
					Type:       libvirt.IP_ADDR_TYPE_IPV4,
					Mac:        "52:54:00:1c:7c:14",
					IPaddr:     "192.168.122.58",
					Hostname:   "nomad-0ea818bc",
					Clientid:   "ff:08:24:45:0e:00:02:00:00:ab:11:35:ab:f3:c7:ac:54:9e:c8",
				},
				{
					Iface:      "virbr0",
					ExpiryTime: time.Now().Add(4 * time.Hour),
					Type:       libvirt.IP_ADDR_TYPE_IPV4,
					Mac:        "11:22:33:44:55:66",
					IPaddr:     "192.168.122.65",
					Hostname:   "nomad-3edc43aa",
					Clientid:   "ff:08:24:45:0e:00:02:00:00:ab:11:35:ab:f3:c7:ac:54:9e:dd",
				},
				{
					Iface:      "virbr0",
					ExpiryTime: time.Now().Add(1 * time.Hour),
					Type:       libvirt.IP_ADDR_TYPE_IPV4,
					Mac:        "11:22:33:44:55:66",
					IPaddr:     "192.168.122.42",
					Hostname:   "nomad-3bab44da",
					Clientid:   "ff:08:24:45:0e:00:02:00:00:ab:11:35:ab:f3:c7:ac:54:9e:ee",
				},
				{
					Iface:      "virbr0",
					ExpiryTime: time.Now().Add(5 * time.Minute),
					Type:       libvirt.IP_ADDR_TYPE_IPV4,
					Mac:        "11:22:33:44:55:66",
					IPaddr:     "192.168.122.39",
					Hostname:   "nomad-9aa018de",
					Clientid:   "ff:08:24:45:0e:00:02:00:00:ab:11:35:ab:f3:c7:ac:54:9e:aa",
				},
				{
					Iface:      "virbr0",
					ExpiryTime: time.Now().Add(-5 * time.Minute),
					Type:       libvirt.IP_ADDR_TYPE_IPV4,
					Mac:        "66:55:44:33:22:11",
					IPaddr:     "192.168.122.11",
					Hostname:   "nomad-eabba892",
					Clientid:   "ff:08:24:45:0e:00:02:00:00:ab:11:35:ab:f3:c7:ac:54:9e:ff",
				},
				{
					Iface:      "virbr0",
					ExpiryTime: time.Now().Add(5 * time.Minute),
					Type:       libvirt.IP_ADDR_TYPE_IPV4,
					Mac:        "11:22:11:22:11:22",
					IPaddr:     "192.168.122.99",
					Hostname:   "",
					Clientid:   "ff:08:24:45:0e:00:02:00:00:ab:11:35:ab:f3:c7:ac:54:9e:bb",
				},
			},
			xmlDesc: `<network>
  <name>default</name>
  <uuid>dd8fe884-6c02-601e-7551-cca97df1c5df</uuid>
  <forward mode='nat'/>
  <bridge name='virbr0' stp='on' delay='0'/>
  <ip address='192.168.122.1' netmask='255.255.255.0'>
    <dhcp>
      <range start='192.168.122.2' end='192.168.122.254'/>
      <host mac="00:11:22:33:44:55" name="test-hostname" ip="192.168.122.45" />
    </dhcp>
  </ip>
</network>`,
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
	xmlDesc    string
}

func (cnm *ConnectNetworkMock) IsActive() (bool, error) { return cnm.active, nil }

func (cnm *ConnectNetworkMock) GetBridgeName() (string, error) { return cnm.bridgeName, nil }

func (cnm *ConnectNetworkMock) GetDHCPLeases() ([]libvirt.NetworkDHCPLease, error) {
	return cnm.dhcpLeases, nil
}

func (cnm *ConnectNetworkMock) GetXMLDesc(flags libvirt.NetworkXMLFlags) (string, error) {
	return cnm.xmlDesc, nil
}

func (cnm *ConnectNetworkMock) Update(cmd libvirt.NetworkUpdateCommand, section libvirt.NetworkUpdateSection, parentIndex int, xml string, flags libvirt.NetworkUpdateFlags) error {
	return nil
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import "libvirt.org/go/libvirt"

// ConnectShim is the shim interface that wraps libvirt connectivity. This
// allows us to create a mock implementation for testing, as we cannot assume
// we will always have expensive bare-metal hosts to run CI, especially on a
// public repository. Functions should be added as required and match only
// those provided by libvirt.Connect.
//
// Each implementation should be lightweight and not include any business
// logic. This allows us to have more confidence in the mock behaving like the
// libvirt backend and avoid bugs due to differences.
type ConnectShim interface {

	// ListNetworks returns an array of network names.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-network.html#virConnectListNetworks
	ListNetworks() ([]string, error)

	// LookupNetworkByName returns a handle to the network object as defined by
	// the name argument. If the network is not found, an error will be
	// returned.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-network.html#virNetworkLookupByName
	LookupNetworkByName(name string) (ConnectNetworkShim, error)
}

// ConnectNetworkShim is the shim interface that wraps libvirt connectivity
// specific to a named network. This allows us to create a mock implementation
// for testing, as we cannot assume we will always have expensive bare-metal
// hosts to run CI, especially on a public repository. Functions should be
// added as required and match only those provided by libvirt.Network.
type ConnectNetworkShim interface {

	// IsActive returns whether the named network is active.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-network.html#virNetworkIsActive
	IsActive() (bool, error)

	// GetBridgeName returns the bridge interface name assigned to the named
	// network.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-network.html#virNetworkGetBridgeName
	GetBridgeName() (string, error)

	// GetDHCPLeases returns an array of DHCP leases for the named network.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-network.html#virNetworkGetDHCPLeases
	GetDHCPLeases() ([]libvirt.NetworkDHCPLease, error)

	// Update the definition of an existing network, either its live running state, its persistent configuration, or both.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-network.html#virNetworkUpdate
	Update(cmd libvirt.NetworkUpdateCommand, section libvirt.NetworkUpdateSection, parentIndex int, xml string, flags libvirt.NetworkUpdateFlags) error

	// Provide an XML description of the network.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-network.html#virNetworkGetXMLDesc
	GetXMLDesc(flags libvirt.NetworkXMLFlags) (string, error)
}

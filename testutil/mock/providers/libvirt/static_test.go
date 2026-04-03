// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"testing"

	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
	"github.com/shoenig/test/must"
)

var (
	_ shims.Connect        = &StaticConnect{}
	_ shims.Connect        = &ConnectEmpty{}
	_ shims.ConnectNetwork = &StaticNetwork{}
)

func TestMock_ListNetworks(t *testing.T) {
	mockConnect := &StaticConnect{}

	netList, err := mockConnect.ListNetworks()
	must.NoError(t, err)
	must.Eq(t, []string{"default", "routed"}, netList)
}

func TestMock_LookupNetworkByName(t *testing.T) {
	mockConnect := &StaticConnect{}

	// Try looking up a network that doesn't exist.
	net, err := mockConnect.LookupNetworkByName("no-found")
	must.ErrorContains(t, err, "unknown network")
	must.Nil(t, net)

	// Lookup the default network.
	net, err = mockConnect.LookupNetworkByName("default")
	must.NoError(t, err)
	must.NotNil(t, net)

	// Lookup the routed network.
	net, err = mockConnect.LookupNetworkByName("routed")
	must.NoError(t, err)
	must.NotNil(t, net)
}

func TestMockEmpty_ListNetworks(t *testing.T) {
	mockConnect := &ConnectEmpty{}

	netList, err := mockConnect.ListNetworks()
	must.NoError(t, err)
	must.Eq(t, []string{}, netList)
}

func TestMockEmpty_LookupNetworkByName(t *testing.T) {
	mockConnect := &ConnectEmpty{}

	// Try looking up a network that doesn't exist.
	net, err := mockConnect.LookupNetworkByName("no-found")
	must.ErrorContains(t, err, "unknown network")
	must.Nil(t, net)

	// Lookup the default network that doesn't exist.
	net, err = mockConnect.LookupNetworkByName("default")
	must.ErrorContains(t, err, "unknown network")
	must.Nil(t, net)

	// Lookup the routed network that doesn't exist.
	net, err = mockConnect.LookupNetworkByName("routed")
	must.ErrorContains(t, err, "unknown network")
	must.Nil(t, net)
}

func TestMockNetwork(t *testing.T) {
	mockNetwork := &StaticNetwork{Name: "default", Active: true, BridgeName: "virbr0"}

	bridgeName, err := mockNetwork.GetBridgeName()
	must.NoError(t, err)
	must.Eq(t, mockNetwork.BridgeName, bridgeName)

	active, err := mockNetwork.IsActive()
	must.NoError(t, err)
	must.Eq(t, mockNetwork.Active, active)
}

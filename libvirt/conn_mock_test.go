// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"testing"

	"github.com/shoenig/test/must"
)

var (
	_ ConnectShim        = &ConnectMock{}
	_ ConnectShim        = &ConnectMockEmpty{}
	_ ConnectNetworkShim = &ConnectNetworkMock{}
)

func TestMock_ListNetworks(t *testing.T) {
	mockConnect := &ConnectMock{}

	netList, err := mockConnect.ListNetworks()
	must.NoError(t, err)
	must.Eq(t, []string{"default", "routed"}, netList)
}

func TestMock_LookupNetworkByName(t *testing.T) {
	mockConnect := &ConnectMock{}

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
	mockConnect := &ConnectMockEmpty{}

	netList, err := mockConnect.ListNetworks()
	must.NoError(t, err)
	must.Eq(t, []string{}, netList)
}

func TestMockEmpty_LookupNetworkByName(t *testing.T) {
	mockConnect := &ConnectMockEmpty{}

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
	mockNetwork := &ConnectNetworkMock{name: "default", active: true, bridgeName: "virbr0"}

	bridgeName, err := mockNetwork.GetBridgeName()
	must.NoError(t, err)
	must.Eq(t, mockNetwork.bridgeName, bridgeName)

	active, err := mockNetwork.IsActive()
	must.NoError(t, err)
	must.Eq(t, mockNetwork.active, active)
}

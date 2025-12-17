// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"testing"

	iface "github.com/hashicorp/nomad-driver-virt/libvirt"
	"github.com/hashicorp/nomad-driver-virt/testutil/mock"
	"github.com/shoenig/test/must"
)

func TestConnect_ListNetworks(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		connect := NewConnect(t)
		connect.ExpectListNetworks(ListNetworks{})

		result, err := connect.ListNetworks()
		must.NoError(t, err)
		must.SliceEmpty(t, result)
	})

	t.Run("error", func(t *testing.T) {
		connect := NewConnect(t)
		connect.ExpectListNetworks(ListNetworks{
			Err: mock.MockTestErr,
		})

		result, err := connect.ListNetworks()
		must.ErrorIs(t, err, mock.MockTestErr)
		must.SliceEmpty(t, result)
	})

	t.Run("unexpected", func(t *testing.T) {
		connect := NewConnect(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "ListNetworks")

		connect.ListNetworks()
	})
}

func TestConnect_LookupNetworkByName(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		network := NewNetwork(t)
		connect := NewConnect(t)
		connect.ExpectLookupNetworkByName(LookupNetworkByName{
			Name:   "default",
			Result: network,
		})

		net, err := connect.LookupNetworkByName("default")
		must.NoError(t, err)
		must.Eq(t, iface.ConnectNetworkShim(network), net)
	})

	t.Run("error", func(t *testing.T) {
		connect := NewConnect(t)
		connect.ExpectLookupNetworkByName(LookupNetworkByName{
			Name: "default",
			Err:  mock.MockTestErr,
		})

		net, err := connect.LookupNetworkByName("default")
		must.ErrorIs(t, err, mock.MockTestErr)
		must.Nil(t, net)
	})

	t.Run("incorrect arguments", func(t *testing.T) {
		connect := NewConnect(mock.MockT())
		connect.ExpectLookupNetworkByName(LookupNetworkByName{
			Name: "default",
		})
		defer mock.AssertIncorrectArguments(t, "LookupNetworkByName")

		connect.LookupNetworkByName("not-default")
	})

	t.Run("unexpected", func(t *testing.T) {
		connect := NewConnect(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "LookupNetworkByName")

		connect.LookupNetworkByName("default")
	})
}

func TestConnect_AssertExpectations(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		net := NewConnect(t)
		net.AssertExpectations()
	})

	t.Run("missing ListNetworks", func(t *testing.T) {
		connect := NewConnect(mock.MockT())
		connect.ExpectListNetworks(ListNetworks{})
		defer mock.AssertExpectations(t, "ListNetworks")

		connect.AssertExpectations()
	})

	t.Run("missing LookupNetworkByName", func(t *testing.T) {
		connect := NewConnect(mock.MockT())
		connect.ExpectLookupNetworkByName(LookupNetworkByName{})
		defer mock.AssertExpectations(t, "LookupNetworkByName")

		connect.AssertExpectations()
	})
}

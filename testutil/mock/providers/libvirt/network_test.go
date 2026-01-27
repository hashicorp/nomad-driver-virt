// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"testing"

	"github.com/hashicorp/nomad-driver-virt/testutil/mock"
	"github.com/shoenig/test/must"
	"libvirt.org/go/libvirt"
)

func TestNetwork_IsActive(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		net := NewNetwork(t)
		net.ExpectIsActive(IsActive{Result: true})

		result, err := net.IsActive()
		must.NoError(t, err)
		must.True(t, result)
	})

	t.Run("error", func(t *testing.T) {
		net := NewNetwork(t)
		net.ExpectIsActive(IsActive{Err: mock.MockTestErr})

		result, err := net.IsActive()
		must.ErrorIs(t, err, mock.MockTestErr)
		must.False(t, result)
	})

	t.Run("unexpected", func(t *testing.T) {
		net := NewNetwork(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "IsActive")

		net.IsActive()
	})
}

func TestNetwork_GetBridgeName(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		net := NewNetwork(t)
		net.ExpectGetBridgeName(GetBridgeName{Result: "default"})

		result, err := net.GetBridgeName()
		must.NoError(t, err)
		must.Eq(t, "default", result)
	})

	t.Run("error", func(t *testing.T) {
		net := NewNetwork(t)
		net.ExpectGetBridgeName(GetBridgeName{Err: mock.MockTestErr})

		result, err := net.GetBridgeName()
		must.ErrorIs(t, err, mock.MockTestErr)
		must.Eq(t, "", result)
	})

	t.Run("unexpected", func(t *testing.T) {
		net := NewNetwork(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "GetBridgeName")

		net.GetBridgeName()
	})
}

func TestNetwork_GetDHCPLeases(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		net := NewNetwork(t)
		net.ExpectGetDHCPLeases(GetDHCPLeases{})

		result, err := net.GetDHCPLeases()
		must.NoError(t, err)
		must.SliceEmpty(t, result)
	})

	t.Run("error", func(t *testing.T) {
		net := NewNetwork(t)
		net.ExpectGetDHCPLeases(GetDHCPLeases{Err: mock.MockTestErr})

		result, err := net.GetDHCPLeases()
		must.ErrorIs(t, err, mock.MockTestErr)
		must.SliceEmpty(t, result)
	})

	t.Run("unexpected", func(t *testing.T) {
		net := NewNetwork(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "GetDHCPLeases")

		net.GetDHCPLeases()
	})
}

func TestNetwork_Update(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		net := NewNetwork(t)
		net.ExpectUpdate(Update{
			Cmd:         libvirt.NETWORK_UPDATE_COMMAND_ADD_FIRST,
			Section:     libvirt.NETWORK_SECTION_BRIDGE,
			ParentIndex: -1,
			Xml:         "<testing />",
			Flags:       libvirt.NETWORK_UPDATE_AFFECT_CURRENT,
		})

		err := net.Update(libvirt.NETWORK_UPDATE_COMMAND_ADD_FIRST,
			libvirt.NETWORK_SECTION_BRIDGE, -1, "<testing />",
			libvirt.NETWORK_UPDATE_AFFECT_CURRENT)
		must.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		net := NewNetwork(t)
		net.ExpectUpdate(Update{
			Cmd:         libvirt.NETWORK_UPDATE_COMMAND_ADD_FIRST,
			Section:     libvirt.NETWORK_SECTION_BRIDGE,
			ParentIndex: -1,
			Xml:         "<testing />",
			Flags:       libvirt.NETWORK_UPDATE_AFFECT_CURRENT,
			Err:         mock.MockTestErr,
		})

		err := net.Update(libvirt.NETWORK_UPDATE_COMMAND_ADD_FIRST,
			libvirt.NETWORK_SECTION_BRIDGE, -1, "<testing />",
			libvirt.NETWORK_UPDATE_AFFECT_CURRENT)
		must.ErrorIs(t, err, mock.MockTestErr)
	})

	t.Run("unexpected", func(t *testing.T) {
		net := NewNetwork(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "Update")

		net.Update(libvirt.NETWORK_UPDATE_COMMAND_ADD_FIRST,
			libvirt.NETWORK_SECTION_BRIDGE, -1, "<testing />",
			libvirt.NETWORK_UPDATE_AFFECT_CURRENT)
	})
}

func TestNetwork_GetXMLDesc(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		net := NewNetwork(t)
		net.ExpectGetXMLDesc(GetXMLDesc{
			Result: "<testing />",
		})

		result, err := net.GetXMLDesc(0)
		must.NoError(t, err)
		must.Eq(t, "<testing />", result)
	})

	t.Run("error", func(t *testing.T) {
		net := NewNetwork(t)
		net.ExpectGetXMLDesc(GetXMLDesc{
			Err: mock.MockTestErr,
		})

		result, err := net.GetXMLDesc(0)
		must.ErrorIs(t, err, mock.MockTestErr)
		must.Eq(t, "", result)
	})

	t.Run("incorrect arguments", func(t *testing.T) {
		net := NewNetwork(mock.MockT())
		net.ExpectGetXMLDesc(GetXMLDesc{
			Flags: libvirt.NETWORK_XML_INACTIVE,
		})
		defer mock.AssertIncorrectArguments(t, "GetXMLDesc")

		net.GetXMLDesc(0)
	})

	t.Run("unexpected", func(t *testing.T) {
		net := NewNetwork(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "GetXMLDesc")

		net.GetXMLDesc(0)
	})
}

func TestNetwork_AssertExpectations(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		net := NewNetwork(t)
		net.AssertExpectations()
	})

	t.Run("missing IsActive", func(t *testing.T) {
		net := NewNetwork(mock.MockT())
		net.ExpectIsActive(IsActive{})
		defer mock.AssertExpectations(t, "IsActive")

		net.AssertExpectations()
	})

	t.Run("missing GetBridgeNames", func(t *testing.T) {
		net := NewNetwork(mock.MockT())
		net.ExpectGetBridgeName(GetBridgeName{})
		defer mock.AssertExpectations(t, "GetBridgeNames")

		net.AssertExpectations()
	})

	t.Run("missing GetDHCPLeases", func(t *testing.T) {
		net := NewNetwork(mock.MockT())
		net.ExpectGetDHCPLeases(GetDHCPLeases{})
		defer mock.AssertExpectations(t, "GetDHCPLeases")

		net.AssertExpectations()
	})

	t.Run("missing Update", func(t *testing.T) {
		net := NewNetwork(mock.MockT())
		net.ExpectUpdate(Update{})
		defer mock.AssertExpectations(t, "Update")

		net.AssertExpectations()
	})

	t.Run("missing GetXMLDesc", func(t *testing.T) {
		net := NewNetwork(mock.MockT())
		net.ExpectGetXMLDesc(GetXMLDesc{})
		defer mock.AssertExpectations(t, "GetXMLDesc")

		net.AssertExpectations()
	})
}

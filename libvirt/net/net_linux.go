// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package net

import (
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

func (c *Controller) Fingerprint(attr map[string]*structs.Attribute) {

	// List the network names. This is terminal to the fingerprint process, as
	// without this, we have nothing to query.
	networkNames, err := c.netConn.ListNetworks()
	if err != nil {
		c.logger.Error("failed to list networks", "error", err)
		return
	}

	// Iterate the list of network names getting a network handle, so we can
	// query whether it is active.
	for _, networkName := range networkNames {

		networkInfo, err := c.netConn.LookupNetworkByName(networkName)
		if err != nil {
			c.logger.Error("failed to lookup network",
				"network", networkName, "error", err)
			continue
		}

		active, err := networkInfo.IsActive()
		if err != nil {
			c.logger.Error("failed to check network state",
				"network", networkName, "error", err)
			continue
		}

		// Populate the attributes mapping with our network state. Libvirt does
		// not allow two networks of the same name, so there should be no
		// concern about overwriting or duplicates.
		netStateKey := net.FingerprintAttributeKeyPrefix + networkName + ".state"
		attr[netStateKey] = structs.NewStringAttribute(net.IsActiveString(active))

		bridgeName, err := networkInfo.GetBridgeName()
		if err != nil {
			c.logger.Error("failed to get network bridge name",
				"network", networkName, "error", err)
			continue
		}

		// Populate the attributes mapping with our bridge name. Only one
		// bridge can be configured per network, so there should be no concern
		// about overwriting or duplicates.
		netBridgeNameKey := net.FingerprintAttributeKeyPrefix + networkName + ".bridge_name"
		attr[netBridgeNameKey] = structs.NewStringAttribute(bridgeName)
	}
}

// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package net

import (
	"errors"
	"fmt"

	"github.com/coreos/go-iptables/iptables"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

const (
	// preroutingIPTablesChainName is the IPTables chain name used by the
	// driver for prerouting rules. This is currently used for entries within
	// the filter table.
	preroutingIPTablesChainName = "NOMAD_VT_PRT"

	// forwardIPTablesChainName is the IPTables chain name used by the driver
	// for forwarding rules. This is currently used for entries within the NAT
	// table.
	forwardIPTablesChainName = "NOMAD_VT_FW"

	// iptablesNATTableName is the name of the nat table within iptables.
	iptablesNATTableName = "nat"

	// iptablesFilterTableName is the name of the filter table within iptables.
	iptablesFilterTableName = "filter"
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

func (c *Controller) Init() error {
	// The function currently only calls another single function, but is
	// intended to be easy and obvious to expand in the future if needed.
	return c.ensureIPTables()
}

// ensureIPTables is responsible for ensuring the local host machine iptables
// are configured with the chains and rules needed by the driver.
//
// On a new machine, this function creates the "NOMAD_VT_PRT" and "NOMAD_VT_FW"
// chains. The "NOMAD_VT_PRT" chain then has a jump rule added to the "nat"
// table; the "NOMAD_VT_FW" chain has a jump rule added to the "filter" table.
func (c *Controller) ensureIPTables() error {

	ipt, err := iptables.New()
	if err != nil {
		return fmt.Errorf("failed to create iptables handle: %w", err)
	}

	// Ensure the NAT prerouting chain is available and create the jump rule if
	// needed.
	natCreated, err := ensureIPTablesChain(ipt, iptablesNATTableName, preroutingIPTablesChainName)
	if err != nil {
		return fmt.Errorf("failed to create iptables chain %q: %w",
			preroutingIPTablesChainName, err)
	}
	if natCreated {
		if err := ipt.Insert(iptablesNATTableName, "PREROUTING", 1, []string{"-j", preroutingIPTablesChainName}...); err != nil {
			return err
		}
		c.logger.Info("successfully created NAT prerouting iptables chain",
			"name", preroutingIPTablesChainName)
	}

	// Ensure the filter forward chain is available and create the jump rule if
	// needed.
	filterCreated, err := ensureIPTablesChain(ipt, iptablesFilterTableName, forwardIPTablesChainName)
	if err != nil {
		return fmt.Errorf("failed to create iptables chain %q: %w",
			forwardIPTablesChainName, err)
	}
	if filterCreated {
		if err := ipt.Insert(iptablesFilterTableName, "FORWARD", 1, []string{"-j", forwardIPTablesChainName}...); err != nil {
			return err
		}
		c.logger.Info("successfully created filter forward iptables chain",
			"name", forwardIPTablesChainName)
	}

	return nil
}

func ensureIPTablesChain(ipt *iptables.IPTables, table, chain string) (bool, error) {

	// List and iterate the currently configured iptables chains, so we can
	// identify whether the chain already exist.
	chains, err := ipt.ListChains(table)
	if err != nil {
		return false, err
	}
	for _, ch := range chains {
		if ch == chain {
			return false, nil
		}
	}

	err = ipt.NewChain(table, chain)

	// The error returned needs to be carefully checked as an exit code of 1
	// indicates the chain exists. This might happen when another routine has
	// created it.
	var e *iptables.Error

	if errors.As(err, &e) && e.ExitStatus() == 1 {
		return false, nil
	}

	return true, err
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"sync"

	"github.com/coreos/go-iptables/iptables"
	"github.com/hashicorp/go-hclog"
	virtnet "github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/drivers"
)

var (
	loadLock  sync.Mutex
	singleton *nomadTables
)

const (
	// postroutingIPTablesChainName is the IPTables chain name used by the
	// driver for postrouting rules. This is currently used for entries within
	// the nat table specifically for handling the special case of loopback
	// addresses.
	postroutingIPTablesChainName = "NOMAD_VT_PST"

	// preroutingIPTablesChainName is the IPTables chain name used by the
	// driver for prerouting rules. This is currently used for entries within
	// the nat table.
	preroutingIPTablesChainName = "NOMAD_VT_PRT"

	// forwardIPTablesChainName is the IPTables chain name used by the driver
	// for forwarding rules. This is currently used for entries within the
	// filter table.
	forwardIPTablesChainName = "NOMAD_VT_FW"

	// outputIPTablesChainName is the IPTables chain name used by the driver
	// for output rules. This is currently used for entries within the nat
	// table specifically for handling the special case of loopback addresses.
	outputIPTablesChainName = "NOMAD_VT_OUT"

	// iptablesNATTableName is the name of the nat table within iptables.
	iptablesNATTableName = "nat"

	// iptablesFilterTableName is the name of the filter table within iptables.
	iptablesFilterTableName = "filter"

	// routeLocalnetPathTemplate is the template for generating the path to check for device specific routing support.
	routeLocalnetPathTemplate = "/proc/sys/net/ipv4/conf/%s/route_localnet"

	// routeLocalnetGlobalName is the name of the global kernel configuration for localnet routing.
	routeLocalnetGlobalName = "all"
)

// New returns the NomadTables instance.
func New(logger hclog.Logger) (NomadTables, error) {
	loadLock.Lock()
	defer loadLock.Unlock()

	if singleton != nil {
		return singleton, nil
	}

	ipt, err := iptables.New()
	if err != nil {
		return nil, err
	}

	nt := &nomadTables{
		ipt:                        ipt,
		interfaceByIPGetter:        getInterfaceByIP,
		routingInterfaceByIPGetter: getRoutingInterfaceByIP,
		logger:                     logger.Named("iptables"),
	}

	if err := nt.setup(); err != nil {
		return nil, err
	}

	singleton = nt
	return singleton, nil
}

// Interface provided to modify and cleanup IPTables for tasks.
type NomadTables interface {
	Configure(*drivers.Resources, *virtnet.NetworkInterfaceBridgeConfig, string) (Rules, error)
	Teardown(Rules) error
}

// Interface for iptables which defines the subset of functions
// that are currently used. This allows for easily swapping out
// implementations for testing.
type IPTables interface {
	Append(table, chain string, rulespec ...string) error
	ClearChain(table, chain string) error
	Delete(table, chain string, rulespec ...string) error
	DeleteChain(table, chain string) error
	DeleteIfExists(table, chain string, rulespec ...string) error
	Insert(table, chain string, pos int, rulespec ...string) error
	ListChains(table string) ([]string, error)
	List(table, chain string) ([]string, error)
	NewChain(table, chain string) error
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"sync"

	"github.com/coreos/go-iptables/iptables"
	"github.com/hashicorp/go-hclog"
)

var (
	// loadLock is used to synchronize creation and setup of the singleton.
	loadLock sync.Mutex

	// singleton is the single instance of the filter.Filter interface.
	singleton *virtTables
)

const (
	// routeLocalnetPathTemplate is the template for generating the path to check for device specific routing support.
	routeLocalnetPathTemplate = "/proc/sys/net/ipv4/conf/%s/route_localnet"

	// routeLocalnetGlobalName is the name of the global kernel configuration for localnet routing.
	routeLocalnetGlobalName = "all"

	// removalName is the name set in the FilterRemoval
	removalName = "iptables"
)

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

// New returns the filter.Filter interface instance. If the singleton instance does not yet
// exist it will create the instance and run setup. Otherwise it will return the
// existing instance.
func New(logger hclog.Logger) (*virtTables, error) {
	loadLock.Lock()
	defer loadLock.Unlock()

	if singleton != nil {
		return singleton, nil
	}

	ipt, err := iptables.New()
	if err != nil {
		return nil, err
	}

	nt := &virtTables{
		ipt:                        ipt,
		interfaceByIPGetter:        getInterfaceByIP,
		names:                      NewNames(),
		routingInterfaceByIPGetter: getRoutingInterfaceByIP,
		logger:                     logger.Named("iptables"),
	}

	if err := nt.setup(); err != nil {
		return nil, err
	}

	singleton = nt
	return singleton, nil
}

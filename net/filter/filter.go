// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package filter

import (
	"sync"

	"github.com/hashicorp/go-hclog"
	virtnet "github.com/hashicorp/nomad-driver-virt/virt/net"
)

var (
	// loadLock is used to synchronize creation and setup of the singleton.
	loadLock sync.Mutex

	// singleton is the single instance of the filter.Filter interface.
	singleton Filter
)

// Filter is the interface to add and remove packet filtering
// configuration for virt tasks.
type Filter interface {
	SetLogger(hclog.Logger)
	Configure(mappings virtnet.PortMappings, cfg *virtnet.NetworkInterfaceBridgeConfig, ip string, identifier string) (*virtnet.FilterRemoval, error)
	Teardown(*virtnet.FilterRemoval) error
}

// New returns the filter.Filter interface instance. If the singleton instance does not yet
// exist it will create the instance and run setup. Otherwise it will return the
// existing instance.
func New() (Filter, error) {
	loadLock.Lock()
	defer loadLock.Unlock()

	if singleton != nil {
		return singleton, nil
	}

	filter, err := newFilter()
	if err != nil {
		return nil, err
	}

	singleton = filter
	return singleton, nil
}

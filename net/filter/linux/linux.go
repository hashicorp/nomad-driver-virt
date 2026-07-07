// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package linux

import (
	plogger "github.com/hashicorp/nomad-driver-virt/internal/logger"
	"github.com/hashicorp/nomad-driver-virt/net/filter/linux/iptables"
	"github.com/hashicorp/nomad-driver-virt/net/filter/linux/shared"
)

const (
	// routeLocalnetPathTemplate is the template for generating the path to check for device specific routing support.
	routeLocalnetPathTemplate = "/proc/sys/net/ipv4/conf/%s/route_localnet"

	// routeLocalnetGlobalName is the name of the global kernel configuration for localnet routing.
	routeLocalnetGlobalName = "all"

	// removalPrefix is the prefix for the generated name in the removal record
	removalPrefix = "backend:"
)

// Backend is the interface for adding and removing packet
// filtering rules.
type Backend interface {
	Add(*shared.Request) error
	Name() string
	Remove(shared.Teardown) error
}

// option is the type used for customizing a tables instance.
type option func(*tables)

func New(opts ...option) (*tables, error) {
	logger := plogger.Default().Named("packet-filter")

	t := &tables{
		interfaceByIPGetter:        getInterfaceByIP,
		routingInterfaceByIPGetter: getRoutingInterfaceByIP,
		logger:                     logger,
	}

	// Apply any options that may have been provided.
	for _, opt := range opts {
		opt(t)
	}

	// If no names were set by the options, set them now.
	if t.names == nil {
		t.names = NewNames()
	}

	// Build the backend if one was not provided.
	if t.backend == nil {
		backend, err := iptables.New(logger)
		if err != nil {
			return nil, err
		}
		t.backend = backend
	}

	// Now let the backend perform any required setup and be done.
	if err := t.setup(); err != nil {
		return nil, err
	}

	return t, nil
}

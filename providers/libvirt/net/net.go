// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package net

import (
	stdnet "net"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/net/filter"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
)

var (
	// defaultDHCPLeaseDiscoveryInterval is the default retry interval used
	// when performing DHCP lease discovery.
	defaultDHCPLeaseDiscoveryInterval = 2 * time.Second

	// defaultDHCPLeaseDiscoveryTimeout is the default timeout period used when
	// performing DHCP lease discovery. In the future we may want to make this
	// configurable, but for now, it's a good default.
	defaultDHCPLeaseDiscoveryTimeout = 30 * time.Second
)

// Controller implements to Net interface and is the main/only way in which the
// driver should interact with the network-subsystem.
type Controller struct {
	logger  hclog.Logger
	netConn shims.Connect
	filter  filter.Filter

	dhcpLeaseDiscoveryInterval time.Duration
	dhcpLeaseDiscoveryTimeout  time.Duration

	// ipByInterfaceGetter is the function that queries the host using the
	// passed interface name and identifies the IP address assigned to it.
	ipByInterfaceGetter
}

// NewController returns a Controller which implements the net.Net interface
// and has a named logger, to ensure log messages can be easily tied to the
// network system.
func NewController(logger hclog.Logger, conn shims.Connect) *Controller {
	return &Controller{
		dhcpLeaseDiscoveryInterval: defaultDHCPLeaseDiscoveryInterval,
		dhcpLeaseDiscoveryTimeout:  defaultDHCPLeaseDiscoveryTimeout,
		ipByInterfaceGetter:        getIPByInterface,
		logger:                     logger.Named("net"),
		netConn:                    conn,
	}
}

// ipByInterfaceGetter is the function that queries the host using the
// passed interface name and identifies the IP address assigned to it.
type ipByInterfaceGetter func(name string) (stdnet.IP, error)

// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package net

import (
	stdnet "net"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/libvirt"
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
	netConn libvirt.ConnectShim

	dhcpLeaseDiscoveryInterval time.Duration
	dhcpLeaseDiscoveryTimeout  time.Duration

	// interfaceByIPGetter is the function that queries the host using the
	// passed IP address and identifies the interface it is assigned to. It is
	// a field within the controller to aid testing.
	interfaceByIPGetter
}

// NewController returns a Controller which implements the net.Net interface
// and has a named logger, to ensure log messages can be easily tied to the
// network system.
func NewController(logger hclog.Logger, conn libvirt.ConnectShim) *Controller {
	return &Controller{
		dhcpLeaseDiscoveryInterval: defaultDHCPLeaseDiscoveryInterval,
		dhcpLeaseDiscoveryTimeout:  defaultDHCPLeaseDiscoveryTimeout,
		interfaceByIPGetter:        getInterfaceByIP,
		logger:                     logger.Named("net"),
		netConn:                    conn,
	}
}

// interfaceByIPGetter is the function signature used to identify the host's
// interface using a passed IP address. This is primarily used for testing,
// where we don't know the host, and we want to ensure stability and
// consistency when this is called.
type interfaceByIPGetter func(ip stdnet.IP) (string, error)

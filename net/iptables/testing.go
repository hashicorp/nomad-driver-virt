// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"net"

	"github.com/coreos/go-iptables/iptables"
	"github.com/hashicorp/go-hclog"
	"github.com/shoenig/test/must"
)

type testOption func(*nomadTables)

// interfaceByIPGetter is the function signature used to identify the host's
// interface using a passed IP address. This is primarily used for testing,
// where we don't know the host, and we want to ensure stability and
// consistency when this is called.
type interfaceByIPGetter func(ip net.IP) (string, error)

// iptablesInterfaceGetter is the function that returns an interface
// for IPTables.
type iptablesInterfaceGetter func() (IPTables, error)

// routingInterfaceByIPGetter is the function signature used to identify
// the host interface used for an IP address.
type routingInterfaceByIPGetter func(ip string) (string, error)

func WithIPTables(ipt IPTables) testOption {
	return func(n *nomadTables) {
		n.ipt = ipt
	}
}

func WithInterfaceByIPGetter(fn interfaceByIPGetter) testOption {
	return func(n *nomadTables) {
		n.interfaceByIPGetter = fn
	}
}

func WithIPTablesInterfaceGetter(fn iptablesInterfaceGetter) testOption {
	return func(n *nomadTables) {
		n.iptablesInterfaceGetter = fn
	}
}

func WithRoutingInterfaceByIPGetter(fn routingInterfaceByIPGetter) testOption {
	return func(n *nomadTables) {
		n.routingInterfaceByIPGetter = fn
	}
}

func WithRoutingLocalnetTemplate(path string) testOption {
	return func(n *nomadTables) {
		n.routeLocalnetTemplate = path
	}
}

func WithLogger(logger hclog.Logger) testOption {
	return func(n *nomadTables) {
		n.logger = logger
	}
}

func TestNew(t must.T, opts ...testOption) NomadTables {
	t.Helper()
	nt := &nomadTables{
		iptablesInterfaceGetter:    newIPTables,
		interfaceByIPGetter:        getInterfaceByIP,
		routingInterfaceByIPGetter: getRoutingInterfaceByIP,
		logger:                     hclog.NewNullLogger(),
	}

	for _, optFn := range opts {
		optFn(nt)
	}

	ipt, err := nt.iptablesInterfaceGetter()
	must.NoError(t, err, must.Sprint("failed to create iptables instance"))
	nt.ipt = ipt

	return nt
}

func newIPTables() (IPTables, error) {
	return iptables.New()
}

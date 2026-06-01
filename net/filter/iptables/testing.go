// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"net"

	"github.com/coreos/go-iptables/iptables"
	"github.com/go-viper/mapstructure/v2"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/shoenig/test/must"
)

type testOption func(*virtTables)

// interfaceByIPGetter is the function signature used to identify the host's
// interface using a passed IP address. This is primarily used for testing,
// where we don't know the host, and we want to ensure stability and
// consistency when this is called.
type interfaceByIPGetter func(ip net.IP) (string, error)

// routingInterfaceByIPGetter is the function signature used to identify
// the host interface used for an IP address.
type routingInterfaceByIPGetter func(ip string) (string, error)

// WithIPTables sets a custom IPTables implementation.
func WithIPTables(ipt IPTables) testOption {
	return func(n *virtTables) {
		n.ipt = ipt
	}
}

// WithInterfaceByIPGetter sets a custom interfaceByIPGetter.
func WithInterfaceByIPGetter(fn interfaceByIPGetter) testOption {
	return func(n *virtTables) {
		n.interfaceByIPGetter = fn
	}
}

// WithRoutingInterfaceByIPGetter sets a custom routingInterfaceByIPGetter.
func WithRoutingInterfaceByIPGetter(fn routingInterfaceByIPGetter) testOption {
	return func(n *virtTables) {
		n.routingInterfaceByIPGetter = fn
	}
}

// WithRoutingLocalnetPathTemplate sets a custom routeLocalnetPathTemplate.
func WithRoutingLocalnetPathTemplate(tmpl string) testOption {
	return func(n *virtTables) {
		n.routeLocalnetPathTemplate = tmpl
	}
}

// WithNames sets custom names and will fill in any unset values with defaults.
func WithNames(t must.T, nm *names) testOption {
	return func(n *virtTables) {
		dec, err := mapstructure.NewDecoder(
			&mapstructure.DecoderConfig{
				Deep:   true,
				Result: n.names,
			},
		)
		must.NoError(t, err, must.Sprint("failed to create mapstructure decoder"))
		err = dec.Decode(nm)
		must.NoError(t, err, must.Sprint("failed to decode names for iptables"))
	}
}

// WithLogger sets a custom logger.
func WithLogger(logger hclog.Logger) testOption {
	return func(n *virtTables) {
		n.logger = logger
	}
}

// Create a new VirtTables test instance. Returns an optional cleanup
// to remove iptables modifications.
func TestNew(t must.T, opts ...testOption) (*virtTables, func()) {
	t.Helper()
	nt := &virtTables{
		interfaceByIPGetter:        getInterfaceByIP,
		names:                      TestNewNames(),
		routingInterfaceByIPGetter: getRoutingInterfaceByIP,
		logger:                     hclog.NewNullLogger(),
	}

	for _, optFn := range opts {
		optFn(nt)
	}

	if nt.ipt == nil {
		ipt, err := iptables.New()
		must.NoError(t, err, must.Sprint("failed to create iptables instance"))
		nt.ipt = ipt
	}

	return nt, func() { nt.cleanup(t) }
}

// TestNewNames creates a new instance with all nomad chain name values
// set to generated testing names.
func TestNewNames() *names {
	return &names{
		chains: &ChainNames{
			Forward:     defaultChainNameForward,
			Output:      defaultChainNameOutput,
			Postrouting: defaultChainNamePostrouting,
			Prerouting:  defaultChainNamePrerouting,
			Nomad: &NomadChainNames{
				Forward:     genTestName(defaultChainNameNomadForward),
				Postrouting: genTestName(defaultChainNameNomadPostrouting),
				Prerouting:  genTestName(defaultChainNameNomadPrerouting),
				Output:      genTestName(defaultChainNameNomadOutput),
			},
		},
		tables: &TableNames{
			Filter: defaultTableNameFilter,
			NAT:    defaultTableNameNAT,
		},
	}
}

// genTestName appends a short uuid to the prefix to provide a unique
// name for testing.
func genTestName(prefix string) string {
	return prefix + "_" + uuid.Short()
}

// cleanup is used to remove entries from iptables when tests
// are interacting directly with iptables and are not mocked.
func (n *virtTables) cleanup(t must.T) {
	req := newRequest()

	// Add all the custom chains to be removed.
	req.chains.InsertSlice([]*chain{
		{
			table: n.names.tables.Filter,
			chain: n.names.chains.Nomad.Forward,
		},
		{
			table: n.names.tables.NAT,
			chain: n.names.chains.Nomad.Postrouting,
		},
		{
			table: n.names.tables.NAT,
			chain: n.names.chains.Nomad.Prerouting,
		},
		{
			table: n.names.tables.NAT,
			chain: n.names.chains.Nomad.Output,
		},
	})

	// Add all the jump rules to be removed.
	req.rules.InsertSlice([]*rule{
		{
			table: n.names.tables.Filter,
			chain: n.names.chains.Forward,
			spec:  []string{"-j", n.names.chains.Nomad.Forward},
		},
		{
			table: n.names.tables.NAT,
			chain: n.names.chains.Postrouting,
			spec:  []string{"-j", n.names.chains.Nomad.Postrouting},
		},
		{
			table: n.names.tables.NAT,
			chain: n.names.chains.Prerouting,
			spec:  []string{"-j", n.names.chains.Nomad.Prerouting},
		},
		{
			table: n.names.tables.NAT,
			chain: n.names.chains.Output,
			spec:  []string{"-j", n.names.chains.Nomad.Output},
		},
	})

	if err := n.remove(req); err != nil {
		t.Fatalf("error encountered during iptables cleanup: %s", err)
	}
}

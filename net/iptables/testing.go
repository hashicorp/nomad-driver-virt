// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"net"

	"github.com/coreos/go-iptables/iptables"
	"github.com/go-viper/mapstructure/v2"
	"github.com/hashicorp/go-hclog"
	"github.com/shoenig/test/must"
)

const (
	// defaultChainNameNomadPostroutingTest is the testing value of defaultChainNameNomadPostrouting.
	defaultChainNameNomadPostroutingTest = "NOMAD_VT_PST_T"

	// defaultChainNameNomadPreroutingTest is the testing value of defaultChainNameNomadPrerouting.
	defaultChainNameNomadPreroutingTest = "NOMAD_VT_PRT_T"

	// defaultChainNameNomadForwardTest is the testing value of defaultChainNameNomadForward.
	defaultChainNameNomadForwardTest = "NOMAD_VT_FW_T"

	// defaultChainNameNomadOutputTest is the testing value of defaultChainNameNomadOutput.
	defaultChainNameNomadOutputTest = "NOMAD_VT_OUT_T"
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
func TestNew(t must.T, opts ...testOption) (VirtTables, func()) {
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

func testNew(t must.T, opts ...testOption) (*virtTables, func()) {
	vt, cleanup := TestNew(t, opts...)
	return vt.(*virtTables), cleanup
}

// NewNames creates a new instance with all nomad chain name values
// set to testing defaults.
func TestNewNames() *names {
	return &names{
		chains: &ChainNames{
			Forward:     defaultChainNameForward,
			Output:      defaultChainNameOutput,
			Postrouting: defaultChainNamePostrouting,
			Prerouting:  defaultChainNamePrerouting,
			Nomad: &NomadChainNames{
				Forward:     defaultChainNameNomadForwardTest,
				Postrouting: defaultChainNameNomadPostroutingTest,
				Prerouting:  defaultChainNameNomadPreroutingTest,
				Output:      defaultChainNameNomadOutputTest,
			},
		},
		tables: &TableNames{
			Filter: defaultTableNameFilter,
			NAT:    defaultTableNameNAT,
		},
	}
}

// cleanup is used to remove entries from iptables when tests
// are interacting directly with iptables and are not mocked.
// NOTE: we could do the same as apply here: build request, generate lists and reverse
// the request modifcation to remove stuff, then run the deletes.
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
		t.Fatalf("error encountered during iptables cleanup: %w", err)
	}
}

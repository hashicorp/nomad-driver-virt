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

type testOption func(*nomadTables)

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
	return func(n *nomadTables) {
		n.ipt = ipt
	}
}

// WithInterfaceByIPGetter sets a custom interfaceByIPGetter.
func WithInterfaceByIPGetter(fn interfaceByIPGetter) testOption {
	return func(n *nomadTables) {
		n.interfaceByIPGetter = fn
	}
}

// WithRoutingInterfaceByIPGetter sets a custom routingInterfaceByIPGetter.
func WithRoutingInterfaceByIPGetter(fn routingInterfaceByIPGetter) testOption {
	return func(n *nomadTables) {
		n.routingInterfaceByIPGetter = fn
	}
}

// WithRoutingLocalnetPathTemplate sets a custom routeLocalnetPathTemplate.
func WithRoutingLocalnetPathTemplate(tmpl string) testOption {
	return func(n *nomadTables) {
		n.routeLocalnetPathTemplate = tmpl
	}
}

// WithNames sets custom names and will fill in any unset values with defaults.
func WithNames(t must.T, nm *names) testOption {
	return func(n *nomadTables) {
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
	return func(n *nomadTables) {
		n.logger = logger
	}
}

// Create a new NomadTables test instance.
func TestNew(t must.T, opts ...testOption) NomadTables {
	t.Helper()
	nt := &nomadTables{
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

	return nt
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

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

const (
	// defaultChainNameNomadPostrouting is the IPTables chain name used by the
	// driver for postrouting rules. This is currently used for entries within
	// the nat table specifically for handling the special case of loopback
	// addresses.
	defaultChainNameNomadPostrouting = "NOMAD_VT_PST"

	// defaultChainNameNomadPrerouting is the IPTables chain name used by the
	// driver for prerouting rules. This is currently used for entries within
	// the nat table.
	defaultChainNameNomadPrerouting = "NOMAD_VT_PRT"

	// defaultChainNameNomadForward is the IPTables chain name used by the driver
	// for forwarding rules. This is currently used for entries within the
	// filter table.
	defaultChainNameNomadForward = "NOMAD_VT_FW"

	// defaultChainNameNomadOutput is the IPTables chain name used by the driver
	// for output rules. This is currently used for entries within the nat
	// table specifically for handling the special case of loopback addresses.
	defaultChainNameNomadOutput = "NOMAD_VT_OUT"

	// defaultChainNameOutput is the name of the output chain within iptables.
	defaultChainNameOutput = "OUTPUT"

	// defaultChainNamePostrouting is the name of the postrouting chain within iptables.
	defaultChainNamePostrouting = "POSTROUTING"

	// defaultChainNamePrerouting is the name of the prerouting chain within iptables.
	defaultChainNamePrerouting = "PREROUTING"

	// defaultChainNameForward is the name of the forward chain within iptables.
	defaultChainNameForward = "FORWARD"

	// defaultTableNameNAT is the name of the nat table within iptables.
	defaultTableNameNAT = "nat"

	// defaultTableNameFilter is the name of the filter table within iptables.
	defaultTableNameFilter = "filter"
)

// names holds the names for tables and chains used in iptables.
type names struct {
	chains *ChainNames
	tables *TableNames
}

// TableNames holds the names of tables used in iptables.
type TableNames struct {
	Filter string
	NAT    string
}

// ChainNames holds the name of chains used in iptables.
type ChainNames struct {
	Forward     string
	Nomad       *NomadChainNames
	Output      string
	Postrouting string
	Prerouting  string
}

// NomadChainNames holds the names of nomad specific chains used in iptables.
type NomadChainNames struct {
	Forward     string
	Postrouting string
	Prerouting  string
	Output      string
}

// NewNames creates a new instance with all values set to defaults.
func NewNames() *names {
	return &names{
		chains: &ChainNames{
			Forward:     defaultChainNameForward,
			Output:      defaultChainNameNomadOutput,
			Postrouting: defaultChainNamePostrouting,
			Prerouting:  defaultChainNamePrerouting,
			Nomad: &NomadChainNames{
				Forward:     defaultChainNameNomadForward,
				Postrouting: defaultChainNameNomadPostrouting,
				Prerouting:  defaultChainNameNomadPrerouting,
				Output:      defaultChainNameNomadOutput,
			},
		},
		tables: &TableNames{
			Filter: defaultTableNameFilter,
			NAT:    defaultTableNameNAT,
		},
	}
}

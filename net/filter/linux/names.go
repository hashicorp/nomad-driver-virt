// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package linux

const (
	// defaultChainNameNomadPostrouting is the chain name used by the
	// driver for postrouting rules. This is currently used for entries within
	// the nat table specifically for handling the special case of loopback
	// addresses.
	defaultChainNameNomadPostrouting = "NOMAD_VT_PST"

	// defaultChainNameNomadPrerouting is the chain name used by the
	// driver for prerouting rules. This is currently used for entries within
	// the nat table.
	defaultChainNameNomadPrerouting = "NOMAD_VT_PRT"

	// defaultChainNameNomadForward is the chain name used by the driver
	// for forwarding rules. This is currently used for entries within the
	// filter table.
	defaultChainNameNomadForward = "NOMAD_VT_FW"

	// defaultChainNameNomadOutput is the chain name used by the driver
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

// names holds the names for tables and chains used for filtering.
type names struct {
	holder string
	chains *ChainNames
	tables *TableNames
}

// TableNames holds the names of tables used for filtering.
type TableNames struct {
	Filter string
	NAT    string
}

// ChainNames holds the name of chains used for filtering.
type ChainNames struct {
	Forward     string
	Nomad       *NomadChainNames
	Output      string
	Postrouting string
	Prerouting  string
}

// NomadChainNames holds the names of nomad specific chains used for filtering.
type NomadChainNames struct {
	Forward     string
	Postrouting string
	Prerouting  string
	Output      string
}

// NewNames creates a new instance with all values set to defaults.
func NewNames() *names {
	return &names{
		holder: defaultHolderName,
		chains: &ChainNames{
			Forward:     defaultChainNameForward,
			Output:      defaultChainNameOutput,
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

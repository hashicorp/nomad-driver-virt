package net

import (
	"github.com/coreos/go-iptables/iptables"
)

// Interface for iptables which defines the subset of functions
// that are currently used. This allows for easily swapping out
// implementations for testing.
type IPTables interface {
	Append(table, chain string, rulespec ...string) error
	ClearChain(table, chain string) error
	Delete(table, chain string, rulespec ...string) error
	DeleteChain(table, chain string) error
	DeleteIfExists(table, chain string, rulespec ...string) error
	Insert(table, chain string, pos int, rulespec ...string) error
	ListChains(table string) ([]string, error)
	NewChain(table, chain string) error
}

// newIPTables returns a real instance of iptables.
func newIPTables() (IPTables, error) {
	return iptables.New()
}

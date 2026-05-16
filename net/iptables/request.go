// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"cmp"
	"slices"
	"sync"

	"github.com/hashicorp/go-set/v3"
)

// newRequest returns a new request instance.
func newRequest() *request {
	return &request{
		chains: set.NewHashSet[*chain](0),
		rules:  set.NewHashSet[*rule](0),
	}
}

// request is used to provide chains and rules that need to
// be created or removed.
// NOTE: Chains and rules should be added to the request using
// the custom functions instead of adding them directly to the
// underlying set. This is because the custom functions will
// stamp the values to ensure they can be provided in the
// correct order.
type request struct {
	chains     *set.HashSet[*chain, string] // collection of chains
	rules      *set.HashSet[*rule, string]  // collection of rules
	stampValue uint                         // value to stamp on rules/chains for sorting
	m          sync.Mutex                   // mutex to sync stamping
}

// tableList returns the list of table name referenced in the
// request.
func (r *request) tableList() []string {
	s := set.New[string](0)
	tables := make([]string, 0)

	for _, c := range r.sortedChains() {
		if s.Insert(c.table) {
			tables = append(tables, c.table)
		}
	}

	for _, ru := range r.sortedRules() {
		if s.Insert(ru.table) {
			tables = append(tables, ru.table)
		}
	}

	return tables
}

// chainTables returns the collection of table names defined
// in the request's chain collection.
func (r *request) chainTables() []string {
	s := set.New[string](0)
	tables := make([]string, 0)

	for _, c := range r.sortedChains() {
		if s.Insert(c.table) {
			tables = append(tables, c.table)
		}
	}

	return tables
}

// chainList returns the list of chains referenced in the request.
func (r *request) chainList() []*chain {
	s := set.NewHashSet[*chain](0)
	chains := make([]*chain, 0)
	for _, ru := range r.sortedRules() {
		c := ru.mkchain()
		if s.Insert(c) {
			chains = append(chains, c)
		}
	}

	return chains
}

// ruleChains returns the collection chains defined in the
// request's rule collection.
func (r *request) ruleChains() []*chain {
	s := set.NewHashSet[*chain](0)
	chains := make([]*chain, 0)
	for _, rule := range r.sortedRules() {
		c := &chain{table: rule.table, chain: rule.chain}
		if s.Insert(c) {
			chains = append(chains, c)
		}
	}

	return chains
}

// sortedRules returns the rules as a sorted slice based on
// stamp value.
// NOTE: accessing rules directly is unsorted due to map backing.
func (r *request) sortedRules() []*rule {
	rules := r.rules.Slice()
	slices.SortFunc(rules, func(a, b *rule) int { return cmp.Compare(a.stamp, b.stamp) })

	return rules
}

// sortedChains returns the chain as a sorted slice based on
// stamp value.
// NOTE: accessing chains directly is unsorted due to map backing.
func (r *request) sortedChains() []*chain {
	chains := r.chains.Slice()
	slices.SortFunc(chains, func(a, b *chain) int { return cmp.Compare(a.stamp, b.stamp) })

	return chains
}

// teardown returns the raw collection of rules from the
// request that have been flagged as teardown.
func (r *request) teardown() Rules {
	result := make([][]string, 0)
	for _, r := range r.rules.Slice() {
		if !r.teardown {
			continue
		}
		result = append(result, r.slice())
	}

	return result
}

// stampable describes a type that can be stamped.
type stampable interface {
	setStamp(uint)
}

// addChain adds a single chain to the request.
func (r *request) addChain(chains ...*chain) {
	r.addChains(chains)
}

// addChains adds a chain slice to the request.
func (r *request) addChains(chains []*chain) {
	for _, c := range chains {
		r.stamp(c)
		r.chains.Insert(c)
	}
}

// addRule adds a single rule to the request.
func (r *request) addRule(rules ...*rule) {
	r.addRules(rules)
}

// addRules adds a rule slice to the request.
func (r *request) addRules(rules []*rule) {
	for _, rule := range rules {
		r.stamp(rule)
		r.rules.Insert(rule)
	}
}

// stamp sets the stamp value on the item(s).
func (r *request) stamp(items ...stampable) {
	r.m.Lock()
	defer r.m.Unlock()

	for _, item := range items {
		item.setStamp(r.stampValue)
		r.stampValue++
	}
}

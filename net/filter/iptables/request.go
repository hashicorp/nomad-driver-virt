// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
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
// underlying sets. This is because the custom functions will
// stamp the values to ensure they can be provided in the
// correct order.
type request struct {
	chains     *set.HashSet[*chain, string] // collection of chains
	rules      *set.HashSet[*rule, string]  // collection of rules
	stampValue uint                         // value to stamp on rules/chains for sorting
	m          sync.Mutex                   // mutex to sync stamping
}

// sortedRules returns the rules as a sorted slice based on
// stamp value.
// NOTE: accessing rules directly is unsorted due to map backing.
func (r *request) sortedRules() rules {
	return rules(r.rules.Slice()).sort()
}

// sortedChains returns the chain as a sorted slice based on
// stamp value.
// NOTE: accessing chains directly is unsorted due to map backing.
func (r *request) sortedChains() chains {
	return chains(r.chains.Slice()).sort()
}

// removalInstructions returns the raw collection of rules from the
// request that have been flagged as teardown.
func (r *request) removalInstructions() Rules {
	result := make([][]string, 0)
	for _, r := range r.sortedRules().removables() {
		result = append(result, r.slice())
	}

	return result
}

// stampable describes a type that can be stamped.
type stampable interface {
	setStamp(uint)
}

// addChain adds chains to the request.
func (r *request) addChain(chains ...*chain) {
	for _, c := range chains {
		r.stamp(c)
		r.chains.Insert(c)
	}
}

// addRule adds rules to the request.
func (r *request) addRule(rules ...*rule) {
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

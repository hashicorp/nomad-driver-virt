// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package shared

import (
	"cmp"
	"slices"
	"sync"

	"github.com/hashicorp/go-set/v3"
)

const (
	identifierPrefix = "nmd-id:"
)

// identifiable describes a type that can have its identifier set.
type identifiable interface {
	setIdentifier(string) identifiable
}

// stampable describes a type that can be stamped.
type stampable interface {
	setStamp(uint) stampable
}

// Request contains the collection of chains and rules to
// be created or deleted.
type Request struct {
	chains *set.HashSet[Chain, string] // collection of chains
	rules  *set.HashSet[Rule, string]  // collection of rules

	identifier string     // unique string for rule removals
	stampValue uint       // value to stamp on rules/chains for sorting
	m          sync.Mutex // mutex to sync stamping
}

// Create a new request.
func NewRequest(identifier string) *Request {
	return &Request{
		chains:     set.NewHashSet[Chain](0),
		rules:      set.NewHashSet[Rule](0),
		identifier: identifier,
	}
}

// Add an item to the request.
func (r *Request) Add(items ...stampable) bool {
	var added bool

	for _, item := range items {
		// Stamp the item so it can be properly ordered.
		item = r.stamp(item)

		// If the item is identifiable (a rule), add the identifier.
		if i, ok := item.(identifiable); ok {
			item = i.setIdentifier(r.Identifier()).(stampable)
		}

		switch v := item.(type) {
		case Chain:
			if r.chains.Insert(v) {
				added = true
			}
		case Rule:
			if r.rules.Insert(v) {
				added = true
			}
		default:
			panic("attempting to add unsupported item to request")
		}
	}

	return added
}

// Chains returns the ordered list of chains in the request.
func (r *Request) Chains() []Chain {
	s := r.chains.Slice()
	slices.SortFunc(s, func(a, b Chain) int { return cmp.Compare(a.stamp, b.stamp) })
	return s
}

// Rules returns the ordered list of rules in the request.
func (r *Request) Rules() []Rule {
	s := r.rules.Slice()
	slices.SortFunc(s, func(a, b Rule) int { return cmp.Compare(a.stamp, b.stamp) })
	return s
}

// Identifier returns the identifier string used for rules (includes prefix).
func (r *Request) Identifier() string {
	return identifierPrefix + r.identifier
}

// Teardown returns a Teardown struct with the required information
// to remove rules created by this request.
// NOTE: Process the rules in order and stamp the chain before adding it to
// the set. This allows returning a slice that's sorted in a predictible order.
func (r *Request) Teardown() Teardown {
	chains := set.NewHashSet[Chain](0)
	for _, rule := range r.Rules() {
		if !rule.Removable {
			continue
		}

		chain := r.stamp(rule.Chain).(Chain)
		chains.Insert(chain)
	}

	teardownChains := chains.Slice()
	slices.SortFunc(teardownChains, func(a, b Chain) int { return cmp.Compare(a.stamp, b.stamp) })

	return Teardown{
		Identifier:  r.Identifier(),
		RulesWithin: teardownChains,
	}
}

// stamp stamps the given item allowing it to be properly
// sorted.
func (r *Request) stamp(item stampable) stampable {
	r.m.Lock()
	defer r.m.Unlock()

	r.stampValue++
	return item.setStamp(r.stampValue)
}

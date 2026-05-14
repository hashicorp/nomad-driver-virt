// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
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
type request struct {
	chains *set.HashSet[*chain, string]
	rules  *set.HashSet[*rule, string]
}

// chainTables returns the collection of table names defined
// in the request's chain collection.
func (r *request) chainTables() set.Collection[string] {
	s := set.New[string](0)
	for _, c := range r.chains.Slice() {
		s.Insert(c.table)
	}

	return s
}

// ruleChains returns the collection chains defined in the
// request's rule collection.
func (r *request) ruleChains() set.Collection[*chain] {
	s := set.NewHashSet[*chain](0)
	for _, rule := range r.rules.Slice() {
		s.Insert(&chain{table: rule.table, chain: rule.chain})
	}

	return s
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

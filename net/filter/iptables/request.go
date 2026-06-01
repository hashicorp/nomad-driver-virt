// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"sync"
)

// newRequest returns a new request instance.
func newRequest() *request {
	r := &request{}
	r.chains = newChains(r.stamp)
	r.rules = newRules(r.stamp)

	return r
}

// request is used to provide chains and rules that need to
// be created or removed.
type request struct {
	chains     *chains    // collection of chains
	rules      *rules     // collection of rules
	stampValue uint       // value to stamp on rules/chains for sorting
	m          sync.Mutex // mutex to sync stamping
}

// removalInstructions returns the raw collection of rules from the
// request that have been flagged as teardown.
func (r *request) removalInstructions() Rules {
	result := make(Rules, 0)
	for _, r := range r.rules.removables().Slice() {
		result = append(result, r.slice())
	}

	return result
}

// stamp sets the stamp value on the item(s).
func (r *request) stamp(item stampable) {
	r.m.Lock()
	defer r.m.Unlock()

	item.setStamp(r.stampValue)
	r.stampValue++
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"fmt"
	"testing"

	"github.com/shoenig/test/must"
)

func Test_request_removalInstructions(t *testing.T) {
	req := newRequest()
	expectedTeardown := make(Rules, 0)

	// Start with an empty request.
	must.Empty(t, req.removalInstructions())

	// Add a chain entry.
	req.chains.Insert(&chain{table: "test-table", chain: "test-chain"})
	must.Size(t, 1, req.chains)

	// No teardown entries should be provided from chains.
	must.Empty(t, req.removalInstructions())

	// Add a rule entry.
	req.addRule(&rule{table: "test-table", chain: "test-chain"})
	must.Size(t, 1, req.rules)

	// Rule is not marked as teardown so no entries should be returned.
	must.Empty(t, req.removalInstructions())

	// Add a rule entry marked as removable.
	req.addRule(&rule{table: "mock-table", chain: "test-chain", removable: true})
	must.Size(t, 2, req.rules)
	expectedTeardown = append(expectedTeardown, []string{"mock-table", "test-chain"})
	must.SliceContainsAll(t, expectedTeardown, req.removalInstructions())

	// Add another rule entry marked as removable.
	req.addRule(&rule{table: "mock-table", chain: "mock-chain", removable: true})
	must.Size(t, 3, req.rules)
	expectedTeardown = append(expectedTeardown, []string{"mock-table", "mock-chain"})
	must.SliceContainsAll(t, expectedTeardown, req.removalInstructions())
}

func Test_request_sortedRules(t *testing.T) {
	list := make([]*rule, 50)
	for i := range len(list) {
		list[i] = &rule{
			table: "test-table",
			chain: "test-chain",
			spec:  []string{fmt.Sprintf("rule-%d", i)},
		}
	}
	req := newRequest()
	req.addRule(list...)

	must.Eq(t, list, req.sortedRules())
}

func Test_request_sortedChains(t *testing.T) {
	list := make([]*chain, 50)
	for i := range len(list) {
		list[i] = &chain{
			table: "test-table",
			chain: fmt.Sprintf("test-chain-%d", i),
		}
	}
	req := newRequest()
	req.addChain(list...)

	must.Eq(t, list, req.sortedChains())
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"testing"

	"github.com/shoenig/test/must"
)

func Test_request_chainTables(t *testing.T) {
	req := newRequest()

	// Start with an empty request.
	must.Empty(t, req.chainTables())

	// Add a chain.
	req.chains.Insert(&chain{table: "test-table", chain: "test-chain"})
	must.Size(t, 1, req.chains, must.Sprint("expected 1 chain entry"))
	must.SliceContainsAll(t, req.chainTables().Slice(), []string{"test-table"})

	// Add another chain on different table.
	req.chains.Insert(&chain{table: "mock-table", chain: "mock-chain"})
	must.Size(t, 2, req.chains, must.Sprint("expected 2 chain entries"))
	must.SliceContainsAll(t, req.chainTables().Slice(), []string{"test-table", "mock-table"})

	// Add chain on existing table.
	req.chains.Insert(&chain{table: "test-table", chain: "mock-chain"})
	must.Size(t, 3, req.chains, must.Sprint("expected 3 chain entries"))
	must.SliceContainsAll(t, req.chainTables().Slice(), []string{"test-table", "mock-table"})
}

func Test_request_ruleChains(t *testing.T) {
	req := newRequest()

	// Start with an empty request.
	must.Empty(t, req.ruleChains())

	// Add a rule.
	req.rules.Insert(&rule{table: "test-table", chain: "test-chain"})
	must.Size(t, 1, req.rules, must.Sprint("expected 1 rule entry"))
	expectedChains := []*chain{{table: "test-table", chain: "test-chain"}}
	must.SliceContainsAll(t, req.ruleChains().Slice(), expectedChains)

	// Add a rule on a different chain but same table.
	req.rules.Insert(&rule{table: "test-table", chain: "mock-chain"})
	must.Size(t, 2, req.rules, must.Sprint("expected 2 rule entries"))
	expectedChains = append(expectedChains, &chain{table: "test-table", chain: "mock-chain"})
	must.SliceContainsAll(t, req.ruleChains().Slice(), expectedChains)

	// Add a rule on a different table and but same chain name.
	req.rules.Insert(&rule{table: "mock-table", chain: "mock-chain"})
	must.Size(t, 3, req.rules, must.Sprint("expected 3 rule entries"))
	expectedChains = append(expectedChains, &chain{table: "mock-table", chain: "mock-chain"})
	must.SliceContainsAll(t, req.ruleChains().Slice(), expectedChains)

	// Add a rule on an existing table and chain.
	req.rules.Insert(&rule{table: "test-table", chain: "test-chain", position: 1})
	must.Size(t, 4, req.rules, must.Sprint("expected 1 rule entry"))
	must.SliceContainsAll(t, req.ruleChains().Slice(), expectedChains)
}

func Test_request_teardown(t *testing.T) {
	req := newRequest()
	expectedTeardown := make(Rules, 0)

	// Start with an empty request.
	must.Empty(t, req.teardown())

	// Add a chain entry.
	req.chains.Insert(&chain{table: "test-table", chain: "test-chain"})
	must.Size(t, 1, req.chains)

	// No teardown entries should be provided from chains.
	must.Empty(t, req.teardown())

	// Add a rule entry.
	req.rules.Insert(&rule{table: "test-table", chain: "test-chain"})
	must.Size(t, 1, req.rules)

	// Rule is not marked as teardown so no entries should be returned.
	must.Empty(t, req.teardown())

	// Add a rule entry marked as teardown.
	req.rules.Insert(&rule{table: "mock-table", chain: "test-chain", teardown: true})
	must.Size(t, 2, req.rules)
	expectedTeardown = append(expectedTeardown, []string{"mock-table", "test-chain"})
	must.SliceContainsAll(t, expectedTeardown, req.teardown())

	// Add another rule entry marked as teardown.
	req.rules.Insert(&rule{table: "mock-table", chain: "mock-chain", teardown: true})
	must.Size(t, 3, req.rules)
	expectedTeardown = append(expectedTeardown, []string{"mock-table", "mock-chain"})
	must.SliceContainsAll(t, expectedTeardown, req.teardown())
}

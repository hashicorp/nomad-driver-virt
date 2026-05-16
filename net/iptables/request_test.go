// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"testing"

	"github.com/shoenig/test/must"
)

func Test_request_tablesList(t *testing.T) {
	req := newRequest()

	// Start with an empty request.
	must.SliceEmpty(t, req.tableList())

	// Add a chain.
	req.addChain(&chain{table: "test-table", chain: "test-chain"})
	must.Size(t, 1, req.chains, must.Sprint("expected 1 chain entry"))
	must.SliceContainsAll(t, req.tableList(), []string{"test-table"})

	// Add another chain on different table.
	req.addChain(&chain{table: "mock-table", chain: "mock-chain"})
	must.Size(t, 2, req.chains, must.Sprint("expected 2 chain entries"))
	must.SliceContainsAll(t, req.tableList(), []string{"test-table", "mock-table"})

	// Add chain on existing table.
	req.addChain(&chain{table: "test-table", chain: "mock-chain"})
	must.Size(t, 3, req.chains, must.Sprint("expected 3 chain entries"))
	must.SliceContainsAll(t, req.tableList(), []string{"test-table", "mock-table"})

	// Add a rule.
	req.addRule(&rule{table: "faux-table", chain: "test-chain"})
	must.Size(t, 1, req.rules, must.Sprint("expected 1 chain entry"))
	must.SliceContainsAll(t, req.tableList(), []string{"test-table", "mock-table", "faux-table"})
}

func Test_request_chainList(t *testing.T) {
	req := newRequest()

	// Start with an empty request.
	must.SliceEmpty(t, req.chainList())

	// Add a rule.
	req.addRule(&rule{table: "test-table", chain: "test-chain"})
	must.Size(t, 1, req.rules, must.Sprint("expected 1 rule entry"))
	expectedChains := []*chain{{table: "test-table", chain: "test-chain"}}
	must.SliceContainsAll(t, req.chainList(), expectedChains)

	// Add a rule on a different chain but same table.
	req.addRule(&rule{table: "test-table", chain: "mock-chain"})
	must.Size(t, 2, req.rules, must.Sprint("expected 2 rule entries"))
	expectedChains = append(expectedChains, &chain{table: "test-table", chain: "mock-chain"})
	must.SliceContainsAll(t, req.chainList(), expectedChains)

	// Add a rule on a different table and but same chain name.
	req.addRule(&rule{table: "mock-table", chain: "mock-chain"})
	must.Size(t, 3, req.rules, must.Sprint("expected 3 rule entries"))
	expectedChains = append(expectedChains, &chain{table: "mock-table", chain: "mock-chain"})
	must.SliceContainsAll(t, req.chainList(), expectedChains)

	// Add a rule on an existing table and chain.
	req.addRule(&rule{table: "test-table", chain: "test-chain", position: 1})
	must.Size(t, 3, req.rules, must.Sprint("expected 3 rule entry"))
	must.SliceContainsAll(t, req.chainList(), expectedChains)
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
	req.addRule(&rule{table: "test-table", chain: "test-chain"})
	must.Size(t, 1, req.rules)

	// Rule is not marked as teardown so no entries should be returned.
	must.Empty(t, req.teardown())

	// Add a rule entry marked as teardown.
	req.addRule(&rule{table: "mock-table", chain: "test-chain", teardown: true})
	must.Size(t, 2, req.rules)
	expectedTeardown = append(expectedTeardown, []string{"mock-table", "test-chain"})
	must.SliceContainsAll(t, expectedTeardown, req.teardown())

	// Add another rule entry marked as teardown.
	req.addRule(&rule{table: "mock-table", chain: "mock-chain", teardown: true})
	must.Size(t, 3, req.rules)
	expectedTeardown = append(expectedTeardown, []string{"mock-table", "mock-chain"})
	must.SliceContainsAll(t, expectedTeardown, req.teardown())
}

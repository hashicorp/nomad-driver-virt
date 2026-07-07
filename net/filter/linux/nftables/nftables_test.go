// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package nftables

import (
	"testing"

	"github.com/google/nftables"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	"github.com/hashicorp/nomad-driver-virt/net/filter/linux/shared"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/shoenig/test/must"
)

func Test_nft_Add(t *testing.T) {
	t.Run("ok - chains only", func(t *testing.T) {
		nft := testNft(t)

		// Create a new request and add chains.
		req := shared.NewRequest(testName("nomad-identifier"))
		chains := []shared.Chain{
			{
				Table: "filter",
				Chain: testName("test-chain"),
			},
			{
				Table: "filter",
				Chain: testName("test-chain"),
			},
		}
		for _, c := range chains {
			req.Add(c)
		}

		// Add the request.
		must.NoError(t, nft.Add(req))

		// Grab the isolated table our chains are created within.
		table, err := nft.getTable()
		must.NoError(t, err)

		// Now check that the chains exist.
		for _, c := range chains {
			computedChainName := chainName(c)
			nc, err := nft.nft.ListChain(table, computedChainName)
			must.NoError(t, err)
			must.Eq(t, computedChainName, nc.Name)
		}
	})

	t.Run("ok - chains and rules", func(t *testing.T) {
		nft := testNft(t)

		// Create a new request and add chains and rules.
		req := shared.NewRequest(testName("nomad-identifier"))
		chain := shared.Chain{
			Table: "filter",
			Chain: testName("test-chain"),
		}
		rule := shared.Rule{
			Chain: chain,
			Spec: shared.Spec{
				Destination:     "10.22.33.44",
				Protocol:        "tcp",
				State:           "NEW",
				DestinationPort: 9999,
				Jump:            "ACCEPT",
			},
			Removable: true,
		}
		req.Add(chain, rule)

		// Add the request.
		must.NoError(t, nft.Add(req))

		// Collect the rules from the chain.
		rules, err := nft.nft.GetRules(testTable(t, nft), testChain(t, nft, chain))
		must.NoError(t, err)

		// There should exist a single rule.
		must.Len(t, 1, rules)

		// Convert our rule so we can compare.
		computedRule, err := nft.generateRule(rule)
		must.NoError(t, err)

		// Compare the rules.
		must.EqFunc(t, computedRule, rules[0], equivalentRules)
	})

	t.Run("ok - existing chains and rules", func(t *testing.T) {
		nft := testNft(t)

		// Create a new request and add chains and rules.
		req := shared.NewRequest(testName("nomad-identifier"))
		chain := shared.Chain{
			Table: "filter",
			Chain: testName("test-chain"),
		}
		rule := shared.Rule{
			Chain: chain,
			Spec: shared.Spec{
				Destination:     "10.22.33.44",
				Protocol:        "tcp",
				State:           "NEW",
				DestinationPort: 9999,
				Jump:            "ACCEPT",
			},
			Removable: true,
		}
		req.Add(chain, rule)

		// Add the request.
		must.NoError(t, nft.Add(req))

		// Collect the rules from the chain.
		rules, err := nft.nft.GetRules(testTable(t, nft), testChain(t, nft, chain))
		must.NoError(t, err)

		// There should exist a single rule.
		must.Len(t, 1, rules)

		// Add the request again.
		must.NoError(t, nft.Add(req))

		// Collect the rules from the chain.
		rules, err = nft.nft.GetRules(testTable(t, nft), testChain(t, nft, chain))
		must.NoError(t, err)

		// There should still only be a single rule.
		must.Len(t, 1, rules)
	})

	t.Run("ok - updated rules", func(t *testing.T) {
		nft := testNft(t)

		// Create a new request and add chains and rules.
		req := shared.NewRequest(testName("nomad-identifier"))
		chain := shared.Chain{
			Table: "filter",
			Chain: testName("test-chain"),
		}

		srcRules := []shared.Rule{
			{
				Chain: chain,
				Spec: shared.Spec{
					Destination:     "10.22.33.11",
					Protocol:        "tcp",
					State:           "NEW",
					DestinationPort: 1111,
					Jump:            "ACCEPT",
				},
			},
			{
				Chain: chain,
				Spec: shared.Spec{
					Destination:     "10.22.33.22",
					Protocol:        "tcp",
					State:           "NEW",
					DestinationPort: 2222,
					Jump:            "ACCEPT",
				},
			},
			{
				Chain: chain,
				Spec: shared.Spec{
					Destination:     "10.22.33.33",
					Protocol:        "tcp",
					State:           "NEW",
					DestinationPort: 3333,
					Jump:            "ACCEPT",
				},
			},
			{
				Chain: chain,
				Spec: shared.Spec{
					Destination:     "10.22.33.44",
					Protocol:        "tcp",
					State:           "NEW",
					DestinationPort: 4444,
					Jump:            "ACCEPT",
				},
			},
		}
		req.Add(chain)
		for _, r := range srcRules {
			req.Add(r)
		}

		// Add the request.
		must.NoError(t, nft.Add(req))

		// Compute the rules.
		computedRules := make([]*nftables.Rule, len(srcRules))
		for i, r := range srcRules {
			var err error
			computedRules[i], err = nft.generateRule(r)
			must.NoError(t, err)
		}

		// Collect the rules from the chain.
		rules, err := nft.nft.GetRules(testTable(t, nft), testChain(t, nft, chain))
		must.NoError(t, err)

		// Check that the expected number of rules exist.
		must.Len(t, len(srcRules), rules)

		// Check that the rules are what we expect.
		for i := range len(srcRules) {
			must.EqFunc(t, computedRules[i], rules[i], equalRules)
		}

		// Create a new request.
		req = shared.NewRequest(testName("nomad-identifier"))

		// Define updates to existing rules. Provide the collection in a
		// different order than the original, make one removable (which will
		// add a comment to the final rule), add a position to two rules (and
		// set the position to 1 to verify expected ordering), and leave one
		// rule alone.
		updatedRules := []shared.Rule{
			{
				Chain: chain,
				Spec: shared.Spec{
					Destination:     "10.22.33.33",
					Protocol:        "tcp",
					State:           "NEW",
					DestinationPort: 3333,
					Jump:            "ACCEPT",
				},
				Removable: true,
			},
			{
				Chain: chain,
				Spec: shared.Spec{
					Destination:     "10.22.33.11",
					Protocol:        "tcp",
					State:           "NEW",
					DestinationPort: 1111,
					Jump:            "ACCEPT",
				},
			},
			{
				Chain:    chain,
				Position: 1,
				Spec: shared.Spec{
					Destination:     "10.22.33.44",
					Protocol:        "tcp",
					State:           "NEW",
					DestinationPort: 4444,
					Jump:            "ACCEPT",
				},
			},
			{
				Chain:    chain,
				Position: 1,
				Spec: shared.Spec{
					Destination:     "10.22.33.22",
					Protocol:        "tcp",
					State:           "NEW",
					DestinationPort: 2222,
					Jump:            "ACCEPT",
				},
			},
		}
		for _, r := range updatedRules {
			req.Add(r)
		}

		// Compute the updated rules. Use the rules from the
		// request to account for any updates it may have made.
		computedUpdatedRules := make([]*nftables.Rule, len(req.Rules()))
		for i, r := range req.Rules() {
			computedUpdatedRules[i], err = nft.generateRule(r)
			must.NoError(t, err)
		}

		// Apply the updated rules.
		must.NoError(t, nft.Add(req))

		// Get the updated list of current rules.
		rules, err = nft.nft.GetRules(testTable(t, nft), testChain(t, nft, chain))
		must.NoError(t, err)

		// The number of rules should still be the same.
		must.Len(t, len(updatedRules), rules)

		// Check that the rules have been updated, and are now in the expected order. The
		// expected rule ordering of the original updated rule collection is: 3, 2, 1, 0
		must.EqFunc(t, computedUpdatedRules[3], rules[0], equalRules)
		must.EqFunc(t, computedUpdatedRules[2], rules[1], equalRules)
		must.EqFunc(t, computedUpdatedRules[1], rules[2], equalRules)
		must.EqFunc(t, computedUpdatedRules[0], rules[3], equalRules)
	})

	t.Run("error - missing chain", func(t *testing.T) {
		nft := testNft(t)

		// Create a new request and only add the rule.
		req := shared.NewRequest(testName("nomad-identifier"))
		chain := shared.Chain{
			Table: "filter",
			Chain: testName("test-chain"),
		}
		rule := shared.Rule{
			Chain: chain,
			Spec: shared.Spec{
				Destination:     "10.22.33.44",
				Protocol:        "tcp",
				State:           "NEW",
				DestinationPort: 9999,
				Jump:            "ACCEPT",
			},
			Removable: true,
		}
		req.Add(rule)

		// Expect an error to be raised.
		err := nft.Add(req)
		must.ErrorIs(t, err, errs.ErrPacketFilterCreateRule)
		must.ErrorContains(t, err, "failed to load chain")
	})

	t.Run("error - missing table", func(t *testing.T) {
		nft := testNft(t)
		// Immediately destroy the table.
		must.NoError(t, nft.Destroy())

		// Create a new request and only add the chain and rule.
		req := shared.NewRequest(testName("nomad-identifier"))
		chain := shared.Chain{
			Table: "filter",
			Chain: testName("test-chain"),
		}
		rule := shared.Rule{
			Chain: chain,
			Spec: shared.Spec{
				Destination:     "10.22.33.44",
				Protocol:        "tcp",
				State:           "NEW",
				DestinationPort: 9999,
				Jump:            "ACCEPT",
			},
			Removable: true,
		}
		req.Add(chain, rule)

		// Expect an error to be raised.
		err := nft.Add(req)
		must.ErrorIs(t, err, errs.ErrPacketFilter)
		must.ErrorContains(t, err, "failed to load table")
	})
}

func Test_nft_Remove(t *testing.T) {
	t.Run("ok - empty", func(t *testing.T) {
		nft := testNft(t)
		must.NoError(t, nft.Remove(shared.Teardown{}))
	})

	t.Run("ok - no rules or chains exist", func(t *testing.T) {
		nft := testNft(t)
		req := shared.NewRequest(testName("test-identifier"))
		req.Add(
			shared.Chain{
				Table: "filter",
				Chain: testName("nomad-test"),
			},
			shared.Rule{
				Chain: shared.Chain{
					Table: "filter",
					Chain: testName("nomad-other"),
				},
				Spec: shared.Spec{
					Destination:     "10.22.33.44",
					Protocol:        "tcp",
					State:           "NEW",
					DestinationPort: 9999,
					Jump:            "ACCEPT",
				},
				Removable: true,
			},
		)
		must.NoError(t, nft.Remove(req.Teardown()))
	})

	t.Run("ok - no rules exist", func(t *testing.T) {
		nft := testNft(t)
		// Add a chain.
		chainReq := shared.NewRequest(testName("test-identifier"))
		chainReq.Add(shared.Chain{
			Table: "filter",
			Chain: testName("nomad-test"),
		})
		must.NoError(t, nft.Add(chainReq))

		// Create a new request that includes a rule on the existing chain
		// but do not create the rule.
		req := shared.NewRequest(testName("test-identifier"))
		req.Add(
			shared.Rule{
				Chain: shared.Chain{
					Table: "filter",
					Chain: testName("nomad-other"),
				},
				Spec: shared.Spec{
					Destination:     "10.22.33.44",
					Protocol:        "tcp",
					State:           "NEW",
					DestinationPort: 9999,
					Jump:            "ACCEPT",
				},
				Removable: true,
			},
		)

		// Now remove the rule.
		must.NoError(t, nft.Remove(req.Teardown()))
	})

	t.Run("ok - chains", func(t *testing.T) {
		nft := testNft(t)
		req := shared.NewRequest(testName("test-identifier"))
		chains := []shared.Chain{
			{
				Table: "filter",
				Chain: testName("nomad-test"),
			},
			{
				Table: "filter",
				Chain: testName("nomad-test"),
			},
		}
		for _, c := range chains {
			req.Add(c)
		}

		// Add the chains.
		must.NoError(t, nft.Add(req))

		// Check they exist.
		for _, c := range chains {
			_, err := nft.getChain(c)
			must.NoError(t, err)
		}

		// Add chains to teardown request and remove.
		teardown := req.Teardown()
		teardown.Chains = chains
		must.NoError(t, nft.Remove(teardown))

		// Check they do not exist.
		for _, c := range chains {
			_, err := nft.getChain(c)
			must.ErrorIs(t, err, errs.ErrNotFound)
		}
	})

	t.Run("ok - rules", func(t *testing.T) {
		identifier := testName("test-identifier")
		req := shared.NewRequest(identifier)
		chains := []shared.Chain{
			{
				Table: "filter",
				Chain: testName("nomad-test"),
			},
			{
				Table: "filter",
				Chain: testName("nomad-test"),
			},
		}
		nft := testNft(t)

		// Add in some chains.
		for _, c := range chains {
			req.Add(c)
		}

		// Add two rules in each chain, one removeable and one not.
		for _, c := range chains {
			req.Add(
				shared.Rule{
					Chain: c,
					Spec: shared.Spec{
						Destination:     "10.22.33.44",
						Protocol:        "tcp",
						State:           "NEW",
						DestinationPort: 9999,
						Jump:            "ACCEPT",
					},
					Removable: true,
				},
				shared.Rule{
					Chain: c,
					Spec: shared.Spec{
						Destination:     "10.22.33.55",
						Protocol:        "tcp",
						State:           "NEW",
						DestinationPort: 9999,
						Jump:            "ACCEPT",
					},
				},
			)
		}

		// Create the chains and rules.
		must.NoError(t, nft.Add(req))

		// Check that both chains exist and have two rules.
		for _, c := range chains {
			chain, err := nft.getChain(c)
			must.NoError(t, err)

			rules, err := nft.nft.GetRules(chain.Table, chain)
			must.NoError(t, err)
			must.Len(t, 2, rules)
		}

		// Now remove.
		must.NoError(t, nft.Remove(req.Teardown()))

		// Each chain should now only include one rule.
		for _, c := range chains {
			chain, err := nft.getChain(c)
			must.NoError(t, err)

			rules, err := nft.nft.GetRules(chain.Table, chain)
			must.NoError(t, err)
			must.Len(t, 1, rules)
		}
	})
}

func testName(n string) string {
	return n + "_" + uuid.Short()
}

func testNft(t *testing.T) *nft {
	nft, err := New(hclog.NewNullLogger(), testName("nomad-test"))
	must.NoError(t, err)
	t.Cleanup(func() { nft.Destroy() })

	return nft
}

func testTable(t *testing.T, n *nft) *nftables.Table {
	table, err := n.getTable()
	must.NoError(t, err, must.Sprint("failed to get nftables table"))
	return table
}

func testChain(t *testing.T, n *nft, c shared.Chain) *nftables.Chain {
	chain, err := n.getChain(c)
	must.NoError(t, err, must.Sprintf("failed to get nftables chain - %v", c))
	return chain
}

// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"fmt"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/net/filter/linux/shared"
	"github.com/hashicorp/nomad-driver-virt/testutil"

	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/shoenig/test/must"
)

func Test_ipt_Add(t *testing.T) {
	testutil.RequireIPTables(t)

	t.Run("ok - chains only", func(t *testing.T) {
		req := shared.NewRequest(testName("test-identifier"))
		chains := make([]shared.Chain, 10)
		ipt, err := New(hclog.NewNullLogger())
		must.NoError(t, err)
		t.Cleanup(func() {
			t := req.Teardown()
			t.Chains = chains
			ipt.Remove(t)
		})

		// Add new chains to the request.
		for i := range 10 {
			chains[i] = shared.Chain{Table: "filter", Chain: testName("nomad-test")}
			req.Add(chains[i])
		}

		// Add the request.
		must.NoError(t, ipt.Add(req))

		// Check that all the chains now exist.
		for _, chain := range chains {
			ok, err := ipt.ipt.ChainExists(chain.Table, chain.Chain)
			must.NoError(t, err)
			must.True(t, ok, must.Sprintf("expected chain %q to exist in the %q table", chain.Chain, chain.Table))
		}
	})

	t.Run("ok - chains and rules", func(t *testing.T) {
		req := shared.NewRequest(testName("test-identifier"))
		chain := shared.Chain{Table: "filter", Chain: testName("nomad-test")}
		ipt, err := New(hclog.NewNullLogger())
		must.NoError(t, err)
		t.Cleanup(func() {
			t := req.Teardown()
			t.Chains = []shared.Chain{chain}
			ipt.Remove(t)
		})

		req.Add(
			chain,
			shared.Rule{
				Chain: chain,
				Spec: shared.Spec{
					Destination:     "10.22.33.44",
					Protocol:        "tcp",
					State:           "NEW",
					DestinationPort: 9999,
					Jump:            "ACCEPT",
				},
			},
		)

		// Add the request.
		must.NoError(t, ipt.Add(req))

		// Check that the new chain exists.
		ok, err := ipt.ipt.ChainExists(chain.Table, chain.Chain)
		must.NoError(t, err)
		must.True(t, ok, must.Sprintf("expected chain %q to exist in the %q table", chain.Chain, chain.Table))

		// Check that the rule exists.
		rules, err := ipt.ipt.List(chain.Table, chain.Chain)
		must.NoError(t, err)

		// Two rules should be returned, the first is the creation of the chain.
		must.Len(t, 2, rules)

		// Check that the rule was added correctly.
		expected := fmt.Sprintf("-A %s -d 10.22.33.44/32 -p tcp -m tcp --dport 9999 -m state --state NEW -j ACCEPT", chain.Chain)
		must.Eq(t, expected, rules[1])
	})

	t.Run("ok - removable rules", func(t *testing.T) {
		identifier := testName("test-identifier")
		req := shared.NewRequest(identifier)
		chain := shared.Chain{Table: "filter", Chain: testName("nomad-test")}
		ipt, err := New(hclog.NewNullLogger())
		must.NoError(t, err)
		t.Cleanup(func() {
			t := req.Teardown()
			t.Chains = []shared.Chain{chain}
			ipt.Remove(t)
		})

		req.Add(
			chain,
			shared.Rule{
				Chain: chain,
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

		// Add the request.
		must.NoError(t, ipt.Add(req))

		// Check that the new chain exists.
		ok, err := ipt.ipt.ChainExists(chain.Table, chain.Chain)
		must.NoError(t, err)
		must.True(t, ok, must.Sprintf("expected chain %q to exist in the %q table", chain.Chain, chain.Table))

		// Check that the rule exists.
		rules, err := ipt.ipt.List(chain.Table, chain.Chain)
		must.NoError(t, err)

		// Two rules should be returned, the first is the creation of the chain.
		must.Len(t, 2, rules)

		// Check that the rule was added correctly. Since this rule is removable
		// it should include a comment for identification.
		expected := fmt.Sprintf(`-A %s -d 10.22.33.44/32 -p tcp -m tcp --dport 9999 -m state --state NEW -m comment --comment "nmd-id:%s" -j ACCEPT`, chain.Chain, identifier)
		must.Eq(t, expected, rules[1])
	})

	t.Run("ok - existing chains and rules", func(t *testing.T) {
		req := shared.NewRequest(testName("test-identifier"))
		chain := shared.Chain{Table: "filter", Chain: testName("nomad-test")}
		ipt, err := New(hclog.NewNullLogger())
		must.NoError(t, err)
		t.Cleanup(func() {
			t := req.Teardown()
			t.Chains = []shared.Chain{chain}
			ipt.Remove(t)
		})

		req.Add(
			chain,
			shared.Rule{
				Chain: chain,
				Spec: shared.Spec{
					Destination:     "10.22.33.44",
					Protocol:        "tcp",
					State:           "NEW",
					DestinationPort: 9999,
					Jump:            "ACCEPT",
				},
			},
		)

		// Add the request.
		must.NoError(t, ipt.Add(req))

		// Check that the new chain exists.
		ok, err := ipt.ipt.ChainExists(chain.Table, chain.Chain)
		must.NoError(t, err)
		must.True(t, ok, must.Sprintf("expected chain %q to exist in the %q table", chain.Chain, chain.Table))

		// Check that the rule exists.
		rules, err := ipt.ipt.List(chain.Table, chain.Chain)
		must.NoError(t, err)

		// Two rules should be returned, the first is the creation of the chain.
		must.Len(t, 2, rules)

		// Check that the rule was added correctly.
		expected := fmt.Sprintf("-A %s -d 10.22.33.44/32 -p tcp -m tcp --dport 9999 -m state --state NEW -j ACCEPT", chain.Chain)
		must.Eq(t, expected, rules[1])

		// Now add the request again.
		must.NoError(t, ipt.Add(req))

		// Only one rule should exist (with a count of two for the chain creation)
		rules, err = ipt.ipt.List(chain.Table, chain.Chain)
		must.Len(t, 2, rules)
	})
}

func Test_ipt_Remove(t *testing.T) {
	t.Run("ok - empty", func(t *testing.T) {
		ipt, err := New(hclog.NewNullLogger())
		must.NoError(t, err)
		must.NoError(t, ipt.Remove(shared.Teardown{}))
	})

	t.Run("ok - no rules or chains exist", func(t *testing.T) {
		ipt, err := New(hclog.NewNullLogger())
		must.NoError(t, err)
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
		must.NoError(t, ipt.Remove(req.Teardown()))
	})

	t.Run("ok - chains", func(t *testing.T) {
		ipt, err := New(hclog.NewNullLogger())
		must.NoError(t, err)
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
		must.NoError(t, ipt.Add(req))

		// Check they exist.
		for _, c := range chains {
			ok, err := ipt.ipt.ChainExists(c.Table, c.Chain)
			must.NoError(t, err)
			must.True(t, ok, must.Sprintf("expecting chain %q in table %q to exist", c.Chain, c.Table))
		}

		// Add chains to teardown request and remove.
		teardown := req.Teardown()
		teardown.Chains = chains
		must.NoError(t, ipt.Remove(teardown))

		// Check they do not exist.
		for _, c := range chains {
			ok, err := ipt.ipt.ChainExists(c.Table, c.Chain)
			must.NoError(t, err)
			must.False(t, ok, must.Sprintf("expecting chain %q in table %q to not exist", c.Chain, c.Table))
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
		ipt, err := New(hclog.NewNullLogger())
		must.NoError(t, err)
		t.Cleanup(func() { ipt.Remove(shared.Teardown{Chains: chains}) })

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
		must.NoError(t, ipt.Add(req))

		// Check that both chains exist and have 2 rules.
		for _, c := range chains {
			ok, err := ipt.ipt.ChainExists(c.Table, c.Chain)
			must.NoError(t, err)
			must.True(t, ok, must.Sprintf("expecting chain %q to exist in table %q", c.Chain, c.Table))

			// Number of rules in chain includes one extra for chain creation.
			rules, err := ipt.ipt.List(c.Table, c.Chain)
			must.NoError(t, err)
			must.Len(t, 3, rules)
		}

		// Now remove.
		must.NoError(t, ipt.Remove(req.Teardown()))

		// Each chain should now only include one rule (2 count)
		for _, c := range chains {
			rules, err := ipt.ipt.List(c.Table, c.Chain)
			must.NoError(t, err)
			must.Len(t, 2, rules)
		}
	})
}

func Test_ipt_generateRule(t *testing.T) {
	testCases := []struct {
		name       string
		identifier string
		spec       shared.Spec
		expected   rule
	}{
		{
			name:     "protocol",
			spec:     shared.Spec{Protocol: "tcp"},
			expected: rule{spec: []string{"--protocol", "tcp"}},
		},
		{
			name:     "source",
			spec:     shared.Spec{Source: "10.0.0.4"},
			expected: rule{spec: []string{"--source", "10.0.0.4"}},
		},
		{
			name:     "destination",
			spec:     shared.Spec{Destination: "10.0.0.5"},
			expected: rule{spec: []string{"--destination", "10.0.0.5"}},
		},
		{
			name:     "in-interface",
			spec:     shared.Spec{InInterface: "eth0"},
			expected: rule{spec: []string{"--in-interface", "eth0"}},
		},
		{
			name:     "out-interface",
			spec:     shared.Spec{OutInterface: "eth1"},
			expected: rule{spec: []string{"--out-interface", "eth1"}},
		},
		{
			name:     "destination-port (default)",
			spec:     shared.Spec{DestinationPort: 9999},
			expected: rule{spec: []string{"--match", "tcp", "--dport", "9999"}},
		},
		{
			name:     "destination-port (with protocol)",
			spec:     shared.Spec{DestinationPort: 9999, Protocol: "udp"},
			expected: rule{spec: []string{"--protocol", "udp", "--match", "udp", "--dport", "9999"}},
		},
		{
			name:     "source-type",
			spec:     shared.Spec{SourceType: "LOCAL"},
			expected: rule{spec: []string{"--match", "addrtype", "--src-type", "LOCAL"}},
		},
		{
			name:     "destination-type",
			spec:     shared.Spec{DestinationType: "UNICAST"},
			expected: rule{spec: []string{"--match", "addrtype", "--dst-type", "UNICAST"}},
		},
		{
			name:     "source-type and destination-type",
			spec:     shared.Spec{SourceType: "LOCAL", DestinationType: "UNICAST"},
			expected: rule{spec: []string{"--match", "addrtype", "--src-type", "LOCAL", "--dst-type", "UNICAST"}},
		},
		{
			name:     "state",
			spec:     shared.Spec{State: "NEW"},
			expected: rule{spec: []string{"--match", "state", "--state", "NEW"}},
		},
		{
			name:     "jump",
			spec:     shared.Spec{Jump: "ACCEPT"},
			expected: rule{spec: []string{"--jump", "ACCEPT"}},
		},
		{
			name:     "to-destination",
			spec:     shared.Spec{ToDestination: "10.0.9.2:2222"},
			expected: rule{spec: []string{"--to-destination", "10.0.9.2:2222"}},
		},
		{
			name:       "identifier",
			identifier: "nomad-test-ident",
			expected:   rule{spec: []string{"--match", "comment", "--comment", "nomad-test-ident"}},
		},
	}

	// automatically add chain information where needed since it's
	// not interesting.
	chain := shared.Chain{Table: "test-table", Chain: "test-chain"}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			rule := shared.Rule{
				Chain:      chain,
				Identifier: tc.identifier,
				Spec:       tc.spec,
			}
			result := generateRule(rule)
			expected := tc.expected
			expected.table = chain.Table
			expected.chain = chain.Chain

			must.Eq(t, expected, result)
		})
	}
}

func testName(n string) string {
	return n + "_" + uuid.Short()
}

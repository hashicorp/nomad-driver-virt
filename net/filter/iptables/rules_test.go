// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"testing"

	"github.com/shoenig/test/must"
)

func TestRulesEmpty(t *testing.T) {
	r := make(Rules, 0)

	// Empty rules are empty.
	must.Empty(t, r)

	// Add an entry.
	r = append(r, []string{"item"})

	// Non-empty rules are not empty.
	must.NotEmpty(t, r)
}

func TestRules_rules(t *testing.T) {
	testCases := []struct {
		desc   string
		Rules  Rules
		result []*rule
	}{
		{
			desc:  "ok",
			Rules: Rules{{"table-name", "chain-name", "spec", "values"}},
			result: []*rule{{
				table: "table-name", chain: "chain-name",
				spec: []string{"spec", "values"}}},
		},
		{
			desc: "duplicaes",
			Rules: Rules{
				{"table-name", "chain-name", "spec", "values"},
				{"mock-name", "chain-name", "spec", "values"},
				{"table-name", "chain-name", "spec", "values"},
			},
			result: []*rule{
				{
					table: "table-name", chain: "chain-name",
					spec: []string{"spec", "values"},
				},
				{
					table: "mock-name", chain: "chain-name",
					spec: []string{"spec", "values"},
				},
			},
		},
		{
			desc:   "empty",
			Rules:  Rules{{}},
			result: []*rule{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			res := tc.Rules.rules()
			must.SliceContainsAll(t, tc.result, res.Slice())
		})
	}
}

func Test_chain_Equal(t *testing.T) {
	testCases := []struct {
		desc  string
		lhs   *chain
		rhs   *chain
		equal bool
	}{
		{
			desc:  "nil",
			equal: true,
		},
		{
			desc:  "empty",
			lhs:   &chain{},
			rhs:   &chain{},
			equal: true,
		},
		{
			desc: "lhs nil",
			rhs:  &chain{},
		},
		{
			desc: "rhs nil",
			lhs:  &chain{},
		},
		{
			desc:  "ok",
			lhs:   &chain{table: "test-table", chain: "test-chain"},
			rhs:   &chain{table: "test-table", chain: "test-chain"},
			equal: true,
		},
		{
			desc: "diff table",
			lhs:  &chain{table: "mock-table", chain: "test-chain"},
			rhs:  &chain{table: "test-table", chain: "test-chain"},
		},
		{
			desc: "diff chain",
			lhs:  &chain{table: "test-table", chain: "mock-chain"},
			rhs:  &chain{table: "test-table", chain: "test-chain"},
		},
		{
			desc: "diff table and chain",
			lhs:  &chain{table: "test-table", chain: "mock-chain"},
			rhs:  &chain{table: "mock-table", chain: "test-chain"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			if tc.equal {
				must.Equal(t, tc.lhs, tc.rhs)
			} else {
				must.NotEqual(t, tc.lhs, tc.rhs)
			}
		})
	}
}

func Test_rule_Equal(t *testing.T) {
	testCases := []struct {
		desc  string
		lhs   *rule
		rhs   *rule
		equal bool
	}{
		{
			desc:  "nil",
			equal: true,
		},
		{
			desc:  "empty",
			lhs:   &rule{},
			rhs:   &rule{},
			equal: true,
		},
		{
			desc: "lhs nil",
			rhs:  &rule{},
		},
		{
			desc: "rhs nil",
			lhs:  &rule{},
		},
		{
			desc:  "ok table",
			lhs:   &rule{table: "test-table"},
			rhs:   &rule{table: "test-table"},
			equal: true,
		},
		{
			desc:  "ok chain",
			lhs:   &rule{chain: "test-chain"},
			rhs:   &rule{chain: "test-chain"},
			equal: true,
		},
		{
			desc:  "ok table chain",
			lhs:   &rule{table: "test-table", chain: "test-chain"},
			rhs:   &rule{table: "test-table", chain: "test-chain"},
			equal: true,
		},
		{
			desc:  "ok table chain spec",
			lhs:   &rule{table: "test-table", chain: "test-chain", spec: []string{"test", "spec"}},
			rhs:   &rule{table: "test-table", chain: "test-chain", spec: []string{"test", "spec"}},
			equal: true,
		},
		{
			desc: "diff table",
			lhs:  &rule{table: "mock-table", chain: "test-chain", spec: []string{"test", "spec"}},
			rhs:  &rule{table: "test-table", chain: "test-chain", spec: []string{"test", "spec"}},
		},
		{
			desc: "diff chain",
			lhs:  &rule{table: "test-table", chain: "mock-chain", spec: []string{"test", "spec"}},
			rhs:  &rule{table: "test-table", chain: "test-chain", spec: []string{"test", "spec"}},
		},
		{
			desc: "diff spec",
			lhs:  &rule{table: "test-table", chain: "test-chain", spec: []string{"test", "spec"}},
			rhs:  &rule{table: "test-table", chain: "test-chain", spec: []string{"mock", "spec"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			if tc.equal {
				must.Equal(t, tc.rhs, tc.lhs)
			} else {
				must.NotEqual(t, tc.rhs, tc.lhs)
			}
		})
	}
}

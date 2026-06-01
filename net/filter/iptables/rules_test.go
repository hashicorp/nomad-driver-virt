// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"fmt"
	"math/rand"
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
			desc: "duplicates",
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
			desc: "no spec",
			Rules: Rules{
				{"table-name", "chain-name"},
				{"mock-name", "chain-name"},
				{"table-name", "chain-name", "spec", "values"},
			},
			result: []*rule{
				{
					table: "table-name", chain: "chain-name",
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

func Test_chains_sort(t *testing.T) {
	orig := make([]*chain, 50)
	for i := range len(orig) {
		orig[i] = &chain{
			table: "test-table",
			chain: fmt.Sprintf("test-chain-%d", i),
			stamp: uint(i),
		}
	}

	// Create a shuffled chains to sort.
	p := rand.Perm(len(orig))
	mixed := make([]*chain, len(orig))
	for i, j := range p {
		mixed[i] = orig[j]
	}

	list := newChains(nil)
	list.InsertSlice(mixed)

	must.Eq(t, orig, list.Slice())
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
			desc:  "ok stamped",
			lhs:   &chain{table: "test-table", chain: "mock-chain", stamp: 2},
			rhs:   &chain{table: "test-table", chain: "mock-chain", stamp: 3},
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

func Test_rules_sort(t *testing.T) {
	orig := make([]*rule, 50)
	for i := range len(orig) {
		orig[i] = &rule{
			table: "test-table",
			chain: "test-chain",
			spec:  []string{fmt.Sprintf("rule-%d", i)},
			stamp: uint(i),
		}
	}

	// Create a shuffled rules to sort.
	p := rand.Perm(len(orig))
	mixed := make([]*rule, len(orig))
	for i, j := range p {
		mixed[i] = orig[j]
	}

	list := newRules(nil)
	list.InsertSlice(mixed)
	must.Eq(t, orig, list.Slice())
}

func Test_rules_removables(t *testing.T) {
	removables := make([]*rule, 16)
	src := make([]*rule, 50)
	for i := range len(src) {
		src[i] = &rule{
			table: "test-table",
			chain: "test-chain",
			spec:  []string{fmt.Sprintf("rule-%d", i)},
			stamp: uint(i),
		}

		if i > 0 && i%3 == 0 {
			src[i].removable = true
			removables[(i/3)-1] = src[i]
		}
	}

	list := newRules(nil)
	list.InsertSlice(src)

	must.Eq(t, removables, list.removables().Slice())
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
			desc:  "ok stamped",
			lhs:   &rule{table: "test-table", chain: "test-chain", spec: []string{"test", "spec"}, stamp: 0},
			rhs:   &rule{table: "test-table", chain: "test-chain", spec: []string{"test", "spec"}, stamp: 1},
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

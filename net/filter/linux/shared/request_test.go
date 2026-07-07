// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package shared

import (
	"testing"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/shoenig/test/must"
)

func TestRequest_Add(t *testing.T) {
	testCases := []struct {
		name           string
		items          []stampable
		expectedChains []Chain
		expectedRules  []Rule
	}{
		{
			name: "chains",
			items: []stampable{
				Chain{
					Table: "test-table",
					Chain: "chain1",
				},
				Chain{
					Table: "test-table",
					Chain: "chain2",
				},
			},
			expectedChains: []Chain{
				{
					Table: "test-table",
					Chain: "chain1",
					stamp: 1,
				},
				{
					Table: "test-table",
					Chain: "chain2",
					stamp: 2,
				},
			},
			expectedRules: []Rule{},
		},
		{
			name: "rules",
			items: []stampable{
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain1",
					},
					Spec: Spec{
						Destination: "test-dest1",
					},
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain2",
					},
					Spec: Spec{
						Destination: "test-dest2",
					},
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain3",
					},
					Spec: Spec{
						Destination: "test-dest3",
					},
				},
			},
			expectedChains: []Chain{},
			expectedRules: []Rule{
				{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain1",
					},
					Spec: Spec{
						Destination: "test-dest1",
					},
					stamp: 1,
				},
				{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain2",
					},
					Spec: Spec{
						Destination: "test-dest2",
					},
					stamp: 2,
				},
				{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain3",
					},
					Spec: Spec{
						Destination: "test-dest3",
					},
					stamp: 3,
				},
			},
		},
		{
			name: "mixed",
			items: []stampable{
				Chain{
					Table: "test-table",
					Chain: "chain1",
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain1",
					},
					Spec: Spec{
						Destination: "test-dest1",
					},
				},
				Chain{
					Table: "test-table",
					Chain: "chain2",
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain2",
					},
					Spec: Spec{
						Destination: "test-dest2",
					},
				},
				Chain{
					Table: "test-table",
					Chain: "chain3",
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain3",
					},
					Spec: Spec{
						Destination: "test-dest3",
					},
				},
			},
			expectedChains: []Chain{
				{
					Table: "test-table",
					Chain: "chain1",
					stamp: 1,
				},
				{
					Table: "test-table",
					Chain: "chain2",
					stamp: 3,
				},
				{
					Table: "test-table",
					Chain: "chain3",
					stamp: 5,
				},
			},
			expectedRules: []Rule{
				{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain1",
					},
					Spec: Spec{
						Destination: "test-dest1",
					},
					stamp: 2,
				},
				{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain2",
					},
					Spec: Spec{
						Destination: "test-dest2",
					},
					stamp: 4,
				},
				{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain3",
					},
					Spec: Spec{
						Destination: "test-dest3",
					},
					stamp: 6,
				},
			},
		},
		{
			name: "duplicates",
			items: []stampable{
				Chain{
					Table: "test-table",
					Chain: "chain1",
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain1",
					},
					Spec: Spec{
						Destination: "test-dest1",
					},
				},
				Chain{
					Table: "test-table",
					Chain: "chain1",
				},
				Chain{
					Table: "test-table",
					Chain: "chain2",
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain2",
					},
					Spec: Spec{
						Destination: "test-dest2",
					},
				},
				Chain{
					Table: "test-table",
					Chain: "chain3",
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain3",
					},
					Spec: Spec{
						Destination: "test-dest3",
					},
				},
				Chain{
					Table: "test-table",
					Chain: "chain1",
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain2",
					},
					Spec: Spec{
						Destination: "test-dest2",
					},
				},
			},
			expectedChains: []Chain{
				{
					Table: "test-table",
					Chain: "chain1",
					stamp: 1,
				},
				{
					Table: "test-table",
					Chain: "chain2",
					stamp: 4,
				},
				{
					Table: "test-table",
					Chain: "chain3",
					stamp: 6,
				},
			},
			expectedRules: []Rule{
				{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain1",
					},
					Spec: Spec{
						Destination: "test-dest1",
					},
					stamp: 2,
				},
				{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain2",
					},
					Spec: Spec{
						Destination: "test-dest2",
					},
					stamp: 5,
				},
				{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain3",
					},
					Spec: Spec{
						Destination: "test-dest3",
					},
					stamp: 7,
				},
			},
		},
		{
			name: "removables",
			items: []stampable{
				Chain{
					Table: "test-table",
					Chain: "chain1",
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain1",
					},
					Removable: true,
					Spec: Spec{
						Destination: "test-dest1",
					},
				},
				Chain{
					Table: "test-table",
					Chain: "chain2",
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain2",
					},
					Spec: Spec{
						Destination: "test-dest2",
					},
				},
				Chain{
					Table: "test-table",
					Chain: "chain3",
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain3",
					},
					Removable: true,
					Spec: Spec{
						Destination: "test-dest3",
					},
				},
			},
			expectedChains: []Chain{
				{
					Table: "test-table",
					Chain: "chain1",
					stamp: 1,
				},
				{
					Table: "test-table",
					Chain: "chain2",
					stamp: 3,
				},
				{
					Table: "test-table",
					Chain: "chain3",
					stamp: 5,
				},
			},
			expectedRules: []Rule{
				{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain1",
					},
					Spec: Spec{
						Destination: "test-dest1",
					},
					Removable:  true,
					Identifier: "nmd-id:testing",
					stamp:      2,
				},
				{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain2",
					},
					Spec: Spec{
						Destination: "test-dest2",
					},
					stamp: 4,
				},
				{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain3",
					},
					Spec: Spec{
						Destination: "test-dest3",
					},
					Removable:  true,
					Identifier: "nmd-id:testing",
					stamp:      6,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := NewRequest("testing")
			req.Add(tc.items...)
			must.Eq(t, tc.expectedChains, req.Chains())
			must.Eq(t, tc.expectedRules, req.Rules())
		})
	}
}

func TestRequest_Teardown(t *testing.T) {
	testCases := []struct {
		name     string
		items    []stampable
		expected Teardown
	}{
		{
			name: "chains only",
			items: []stampable{
				Chain{
					Table: "test-table",
					Chain: "chain1",
				},
				Chain{
					Table: "test-table",
					Chain: "chain2",
				},
			},
			expected: Teardown{
				Identifier:  "nmd-id:testing",
				RulesWithin: []Chain{},
			},
		},
		{
			name: "no removable rules",
			items: []stampable{
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain1",
					},
					Spec: Spec{
						Destination: "test-dest1",
					},
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain2",
					},
					Spec: Spec{
						Destination: "test-dest2",
					},
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain3",
					},
					Spec: Spec{
						Destination: "test-dest3",
					},
				},
			},
			expected: Teardown{
				Identifier:  "nmd-id:testing",
				RulesWithin: []Chain{},
			},
		},
		{
			name: "removable rules same chain",
			items: []stampable{
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain1",
					},
					Removable: true,
					Spec: Spec{
						Destination: "test-dest1",
					},
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain2",
					},
					Spec: Spec{
						Destination: "test-dest2",
					},
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain1",
					},
					Removable: true,
					Spec: Spec{
						Destination: "test-dest3",
					},
				},
			},
			expected: Teardown{
				Identifier: "nmd-id:testing",
				RulesWithin: []Chain{
					{
						Table: "test-table",
						Chain: "chain1",
					},
				},
			},
		},
		{
			name: "removable rules",
			items: []stampable{
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain1",
					},
					Removable: true,
					Spec: Spec{
						Destination: "test-dest1",
					},
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain2",
					},
					Spec: Spec{
						Destination: "test-dest2",
					},
				},
				Rule{
					Chain: Chain{
						Table: "test-table",
						Chain: "chain3",
					},
					Removable: true,
					Spec: Spec{
						Destination: "test-dest3",
					},
				},
			},
			expected: Teardown{
				Identifier: "nmd-id:testing",
				RulesWithin: []Chain{
					{
						Table: "test-table",
						Chain: "chain1",
					},
					{
						Table: "test-table",
						Chain: "chain3",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := NewRequest("testing")
			req.Add(tc.items...)
			// The Chain slice returned by RulesWithin has no guaranteed sort order
			// so check parts individually to properly test the slice.
			must.Eq(t, tc.expected.Identifier, req.Teardown().Identifier)
			must.SliceContainsAll(t, tc.expected.RulesWithin, req.Teardown().RulesWithin,
				must.Cmp(cmpopts.IgnoreUnexported(Chain{})))
		})
	}
}

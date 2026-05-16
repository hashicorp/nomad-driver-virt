// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-set/v3"
	"github.com/hashicorp/nomad-driver-virt/testutil"
	mock_iptables "github.com/hashicorp/nomad-driver-virt/testutil/mock/iptables"
	"github.com/shoenig/test/must"
)

// NOTE: Many of the tests in this file contain two versions: mock and direct.
// The "mock" tests will utilize a mock iptables implementation to exercise the
// implementation. The "direct" tests require being run as root and the iptables
// command being available. The "direct" tests are built to be isolated and cleaned
// up after being run, but they will make changes to iptables and no guarantees are
// made about the resulting state. We just do our best. Good luck! \o/

func Test_virtTables_Configure(t *testing.T) {

}

func Test_virtTables_setup(t *testing.T) {
	t.Run("mock", func(t *testing.T) {
		t.Run("not setup", func(t *testing.T) {
			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ListChains{Table: "nat", Result: []string{"PREROUTING"}},
				mock_iptables.ListChains{Table: "filter", Result: []string{"FORWARD"}},
				mock_iptables.List{Table: "nat", Chain: "PREROUTING"},
				mock_iptables.List{Table: "filter", Chain: "FORWARD"},
				mock_iptables.NewChain{Table: "nat", Chain: "NOMAD_VT_PRT_T"},
				mock_iptables.NewChain{Table: "filter", Chain: "NOMAD_VT_FW_T"},
				mock_iptables.Insert{Table: "nat", Chain: "PREROUTING", Pos: 1, RuleSpec: []string{"-j", "NOMAD_VT_PRT_T"}},
				mock_iptables.Insert{Table: "filter", Chain: "FORWARD", Pos: 1, RuleSpec: []string{"-j", "NOMAD_VT_FW_T"}},
			)
			defer ipt.AssertExpectations()

			vt, _ := testNew(t, WithIPTables(ipt))
			must.NoError(t, vt.setup())
		})

		t.Run("already setup", func(t *testing.T) {
			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ListChains{Table: "nat", Result: []string{"PREROUTING"}},
				mock_iptables.ListChains{Table: "filter", Result: []string{"FORWARD"}},
				mock_iptables.List{Table: "nat", Chain: "PREROUTING"},
				mock_iptables.List{Table: "filter", Chain: "FORWARD"},
				mock_iptables.NewChain{Table: "nat", Chain: "NOMAD_VT_PRT_T"},
				mock_iptables.NewChain{Table: "filter", Chain: "NOMAD_VT_FW_T"},
				mock_iptables.Insert{Table: "nat", Chain: "PREROUTING", Pos: 1, RuleSpec: []string{"-j", "NOMAD_VT_PRT_T"}},
				mock_iptables.Insert{Table: "filter", Chain: "FORWARD", Pos: 1, RuleSpec: []string{"-j", "NOMAD_VT_FW_T"}},
				mock_iptables.ListChains{Table: "nat", Result: []string{"PREROUTING", "NOMAD_VT_PRT_T"}},
				mock_iptables.ListChains{Table: "filter", Result: []string{"FORWARD", "NOMAD_VT_FW_T"}},
				mock_iptables.List{Table: "nat", Chain: "PREROUTING", Result: []string{"-A PREROUTING -j NOMAD_VT_PRT_T"}},
				mock_iptables.List{Table: "filter", Chain: "FORWARD", Result: []string{"-A FORWARD -j NOMAD_VT_FW_T"}},
			)
			defer ipt.AssertExpectations()
			vt, _ := testNew(t, WithIPTables(ipt))

			// Run an initial setup to ensure configuration exists.
			must.NoError(t, vt.setup())

			// Run the setup again.
			must.NoError(t, vt.setup())
		})

	})

	t.Run("direct", func(t *testing.T) {
		testutil.RequireIPTables(t)

		t.Run("not setup", func(t *testing.T) {
			vt, cleanup := testNew(t)
			// Perform an initial cleanup to provide a clean start.
			t.Cleanup(cleanup)

			// Run the setup.
			must.NoError(t, vt.setup())

			// Check that chains exist.
			natChains, err := vt.ipt.ListChains("nat")
			must.NoError(t, err)
			must.SliceContains(t, natChains, "NOMAD_VT_PRT_T")

			filterChains, err := vt.ipt.ListChains("filter")
			must.NoError(t, err)
			must.SliceContains(t, filterChains, "NOMAD_VT_FW_T")

			// Check that rules exist.
			natRules, err := vt.ipt.List("nat", "PREROUTING")
			must.NoError(t, err)
			must.SliceContains(t, natRules, "-A PREROUTING -j NOMAD_VT_PRT_T")

			filterRules, err := vt.ipt.List("filter", "FORWARD")
			must.NoError(t, err)
			must.SliceContains(t, filterRules, "-A FORWARD -j NOMAD_VT_FW_T")
		})

		t.Run("already setup", func(t *testing.T) {
			vt, cleanup := testNew(t)
			t.Cleanup(cleanup)

			// Run an initial setup to ensure configuration exists.
			must.NoError(t, vt.setup())

			// Run the setup again.
			must.NoError(t, vt.setup())

			// Duplicate chains will cause an error but duplicate
			// rules will not, so check for duplicate rules.
			natSet := set.New[string](0)
			natRules, err := vt.ipt.List("nat", "PREROUTING")
			must.NoError(t, err)
			for _, r := range natRules {
				must.True(t, natSet.Insert(r), must.Sprintf("duplicate rule found: %q", r))
			}

			filterSet := set.New[string](0)
			filterRules, err := vt.ipt.List("filter", "FORWARD")
			must.NoError(t, err)
			for _, r := range filterRules {
				must.True(t, filterSet.Insert(r), must.Sprintf("duplicate rule found: %q", r))
			}
		})
	})
}

func Test_virtTables_add(t *testing.T) {
	t.Run("mock", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ListChains{Table: "nat"},
				mock_iptables.NewChain{Table: "nat", Chain: "NOMAD_TEST_1"},
				mock_iptables.NewChain{Table: "nat", Chain: "NOMAD_TEST_2"},
				mock_iptables.Append{Table: "nat", Chain: "NOMAD_TEST_1", RuleSpec: []string{"-p", "tcp", "-j", "ACCEPT"}},
			)
			defer ipt.AssertExpectations()

			vt, _ := testNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChains([]*chain{
				{
					table: "nat",
					chain: "NOMAD_TEST_1",
				},
				{
					table: "nat",
					chain: "NOMAD_TEST_2",
				},
			})
			req.addRule(&rule{
				table: "nat",
				chain: "NOMAD_TEST_1",
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.add(req))
		})

		t.Run("ok insert", func(t *testing.T) {
			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ListChains{Table: "nat"},
				mock_iptables.NewChain{Table: "nat", Chain: "NOMAD_TEST_1"},
				mock_iptables.NewChain{Table: "nat", Chain: "NOMAD_TEST_2"},
				mock_iptables.Append{Table: "nat", Chain: "NOMAD_TEST_1", RuleSpec: []string{"-p", "tcp", "-j", "ACCEPT"}},
				mock_iptables.Insert{Table: "nat", Chain: "NOMAD_TEST_2", Pos: 1, RuleSpec: []string{"-p", "tcp", "-j", "ACCEPT"}},
			)
			defer ipt.AssertExpectations()

			vt, _ := testNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChains([]*chain{
				{
					table: "nat",
					chain: "NOMAD_TEST_1",
				},
				{
					table: "nat",
					chain: "NOMAD_TEST_2",
				},
			})
			req.addRules([]*rule{
				{
					table: "nat",
					chain: "NOMAD_TEST_1",
					spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
				},
				{
					table:    "nat",
					chain:    "NOMAD_TEST_2",
					position: 1,
					spec:     []string{"-p", "tcp", "-j", "ACCEPT"},
				},
			})

			must.NoError(t, vt.add(req))
		})

		t.Run("chain exists", func(t *testing.T) {
			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ListChains{Table: "nat", Result: []string{"NOMAD_TEST_2"}},
				mock_iptables.NewChain{Table: "nat", Chain: "NOMAD_TEST_1"},
				mock_iptables.Append{Table: "nat", Chain: "NOMAD_TEST_1", RuleSpec: []string{"-p", "tcp", "-j", "ACCEPT"}},
			)
			defer ipt.AssertExpectations()

			vt, _ := testNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChains([]*chain{
				{
					table: "nat",
					chain: "NOMAD_TEST_1",
				},
				{
					table: "nat",
					chain: "NOMAD_TEST_2",
				},
			})
			req.addRule(&rule{
				table: "nat",
				chain: "NOMAD_TEST_1",
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.add(req))

		})
	})
}

func Test_virtTables_remove(t *testing.T) {
	t.Run("mock", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ListChains{Table: "nat", Result: []string{"NOMAD_TEST_1", "NOMAD_TEST_2"}},
				mock_iptables.List{Table: "nat", Chain: "NOMAD_TEST_1", Result: []string{"-A NOMAD_TEST_1 -p tcp -j ACCEPT"}},
				mock_iptables.Delete{Table: "nat", Chain: "NOMAD_TEST_1", RuleSpec: []string{"-p", "tcp", "-j", "ACCEPT"}},
				mock_iptables.ClearChain{Table: "nat", Chain: "NOMAD_TEST_2"},
				mock_iptables.DeleteChain{Table: "nat", Chain: "NOMAD_TEST_2"},
			)
			defer ipt.AssertExpectations()

			vt, _ := testNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChains([]*chain{
				{
					table: "nat",
					chain: "NOMAD_TEST_2",
				},
			})
			req.addRule(&rule{
				table: "nat",
				chain: "NOMAD_TEST_1",
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.remove(req))
		})

		t.Run("no rules", func(t *testing.T) {
			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ListChains{Table: "nat", Result: []string{"NOMAD_TEST_1", "NOMAD_TEST_2"}},
				mock_iptables.List{Table: "nat", Chain: "NOMAD_TEST_1"},
				mock_iptables.ClearChain{Table: "nat", Chain: "NOMAD_TEST_2"},
				mock_iptables.DeleteChain{Table: "nat", Chain: "NOMAD_TEST_2"},
			)
			defer ipt.AssertExpectations()

			vt, _ := testNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChains([]*chain{
				{
					table: "nat",
					chain: "NOMAD_TEST_2",
				},
			})
			req.addRule(&rule{
				table: "nat",
				chain: "NOMAD_TEST_1",
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.remove(req))
		})

		t.Run("no chains", func(t *testing.T) {
			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ListChains{Table: "nat"},
			)
			defer ipt.AssertExpectations()

			vt, _ := testNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChains([]*chain{
				{
					table: "nat",
					chain: "NOMAD_TEST_2",
				},
			})
			req.addRule(&rule{
				table: "nat",
				chain: "NOMAD_TEST_1",
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.remove(req))
		})
	})

	t.Run("direct", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			vt, _ := testNew(t)
			must.NoError(t, vt.ipt.NewChain("nat", "NOMAD_TEST_1"))
			must.NoError(t, vt.ipt.NewChain("nat", "NOMAD_TEST_2"))
			must.NoError(t, vt.ipt.Append("nat", "NOMAD_TEST_1", "-p", "tcp", "-j", "ACCEPT"))
			t.Cleanup(func() {
				vt.ipt.ClearChain("nat", "NOMAD_TEST_1")
				vt.ipt.DeleteChain("nat", "NOMAD_TEST_2")
				vt.ipt.DeleteChain("nat", "NOMAD_TEST_1")
			})
			req := newRequest()
			req.addChains([]*chain{
				{
					table: "nat",
					chain: "NOMAD_TEST_2",
				},
			})
			req.addRule(&rule{
				table: "nat",
				chain: "NOMAD_TEST_1",
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.remove(req))

			chains, err := vt.ipt.ListChains("nat")
			must.NoError(t, err)
			rules, err := vt.ipt.List("nat", "NOMAD_TEST_1")
			must.NoError(t, err)
			must.SliceContains(t, chains, "NOMAD_TEST_1")
			must.SliceNotContains(t, chains, "NOMAD_TEST_2")
			must.Eq(t, rules, []string{"-N NOMAD_TEST_1"})
		})

		t.Run("no rules", func(t *testing.T) {
			vt, _ := testNew(t)
			must.NoError(t, vt.ipt.NewChain("nat", "NOMAD_TEST_1"))
			must.NoError(t, vt.ipt.NewChain("nat", "NOMAD_TEST_2"))

			t.Cleanup(func() {
				vt.ipt.ClearChain("nat", "NOMAD_TEST_1")
				vt.ipt.DeleteChain("nat", "NOMAD_TEST_2")
				vt.ipt.DeleteChain("nat", "NOMAD_TEST_1")
			})

			req := newRequest()
			req.addChains([]*chain{
				{
					table: "nat",
					chain: "NOMAD_TEST_2",
				},
			})
			req.addRule(&rule{
				table: "nat",
				chain: "NOMAD_TEST_1",
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.remove(req))

			chains, err := vt.ipt.ListChains("nat")
			must.NoError(t, err)
			rules, err := vt.ipt.List("nat", "NOMAD_TEST_1")
			must.NoError(t, err)
			must.SliceContains(t, chains, "NOMAD_TEST_1")
			must.SliceNotContains(t, chains, "NOMAD_TEST_2")
			must.Eq(t, rules, []string{"-N NOMAD_TEST_1"})
		})

		t.Run("no chains", func(t *testing.T) {
			vt, _ := testNew(t)
			req := newRequest()
			req.addChains([]*chain{
				{
					table: "nat",
					chain: "NOMAD_TEST_2",
				},
			})
			req.addRule(&rule{
				table: "nat",
				chain: "NOMAD_TEST_1",
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.remove(req))

			chains, err := vt.ipt.ListChains("nat")
			must.NoError(t, err)
			must.SliceNotContains(t, chains, "NOMAD_TEST_1")
			must.SliceNotContains(t, chains, "NOMAD_TEST_2")
		})
	})
}

func Test_virtTables_buildLists(t *testing.T) {
	t.Run("mock", func(t *testing.T) {
		t.Run("empty sets", func(t *testing.T) {
			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ListChains{Table: "nat"},
			)
			defer ipt.AssertExpectations()

			vt, _ := testNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChains([]*chain{
				{
					table: "nat",
					chain: "NOMAD_TEST_1",
				},
				{
					table: "nat",
					chain: "NOMAD_TEST_2",
				},
			})
			req.addRule(&rule{
				table: "nat",
				chain: "NOMAD_TEST_2",
			})

			chains, rules, err := vt.buildLists(req)
			must.NoError(t, err)
			must.Empty(t, chains)
			must.Empty(t, rules)
		})

		t.Run("chain exists", func(t *testing.T) {
			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ListChains{Table: "nat", Result: []string{"NOMAD_TEST_4"}},
				// NOTE: empty chain will contain an empty rule
				mock_iptables.List{Table: "nat", Chain: "NOMAD_TEST_4", Result: []string{"-N NOMAD_TEST_4"}},
			)
			defer ipt.AssertExpectations()

			vt, _ := testNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChains([]*chain{
				{
					table: "nat",
					chain: "NOMAD_TEST_3",
				},
				{
					table: "nat",
					chain: "NOMAD_TEST_4",
				},
			})
			req.addRule(&rule{
				table: "nat",
				chain: "NOMAD_TEST_4",
			})

			chains, rules, err := vt.buildLists(req)
			must.NoError(t, err)

			must.SliceEqual(t, chains.Slice(), []*chain{{table: "nat", chain: "NOMAD_TEST_4"}})
			must.SliceEqual(t, rules.Slice(), []*rule{{table: "nat", chain: "NOMAD_TEST_4"}})
		})

		t.Run("rules exists", func(t *testing.T) {
			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ListChains{Table: "nat", Result: []string{"NOMAD_TEST_6"}},
				mock_iptables.List{Table: "nat", Chain: "NOMAD_TEST_6", Result: []string{"-A NOMAD_TEST_6 -p tcp -j ACCEPT"}},
			)
			defer ipt.AssertExpectations()

			vt, _ := testNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChains([]*chain{
				{
					table: "nat",
					chain: "NOMAD_TEST_5",
				},
				{
					table: "nat",
					chain: "NOMAD_TEST_6",
				},
			})
			req.addRules([]*rule{
				{
					table: "nat",
					chain: "NOMAD_TEST_6",
				},
				{
					table: "nat",
					chain: "NOMAD_TEST_5",
				},
			})

			chains, rules, err := vt.buildLists(req)
			must.NoError(t, err)

			must.SliceEqual(t, chains.Slice(), []*chain{{table: "nat", chain: "NOMAD_TEST_6"}})
			expectedRule := &rule{table: "nat", chain: "NOMAD_TEST_6", spec: []string{"-p", "tcp", "-j", "ACCEPT"}}
			must.True(t, rules.Contains(expectedRule),
				must.Sprintf("missing expected rule: %s", expectedRule))
		})
	})

	t.Run("direct", func(t *testing.T) {
		t.Run("empty sets", func(t *testing.T) {
			vt, _ := testNew(t)
			req := newRequest()
			req.addChains([]*chain{
				{
					table: "nat",
					chain: "NOMAD_TEST_1",
				},
				{
					table: "nat",
					chain: "NOMAD_TEST_2",
				},
			})
			req.addRule(&rule{
				table: "nat",
				chain: "NOMAD_TEST_2",
			})

			chains, rules, err := vt.buildLists(req)
			must.NoError(t, err)
			// NOTE: There's probably other chains, so just confirm that
			// the test chains aren't included.
			must.SliceNotContains(t, chains.Slice(), &chain{table: "nat", chain: "NOMAD_TEST_1"})
			must.SliceNotContains(t, chains.Slice(), &chain{table: "nat", chain: "NOMAD_TEST_2"})
			must.Empty(t, rules)
		})

		t.Run("chain exists", func(t *testing.T) {
			vt, _ := testNew(t)
			must.NoError(t, vt.ipt.NewChain("nat", "NOMAD_TEST_4"))
			t.Cleanup(func() { vt.ipt.DeleteChain("nat", "NOMAD_TEST_4") })

			req := newRequest()
			req.addChains([]*chain{
				{
					table: "nat",
					chain: "NOMAD_TEST_3",
				},
				{
					table: "nat",
					chain: "NOMAD_TEST_4",
				},
			})
			req.addRule(&rule{
				table: "nat",
				chain: "NOMAD_TEST_4",
			})

			chains, rules, err := vt.buildLists(req)
			must.NoError(t, err)

			must.SliceContains(t, chains.Slice(), &chain{table: "nat", chain: "NOMAD_TEST_4"})
			must.SliceEqual(t, rules.Slice(), []*rule{{table: "nat", chain: "NOMAD_TEST_4"}})
		})

		t.Run("rules exists", func(t *testing.T) {
			vt, _ := testNew(t)
			must.NoError(t, vt.ipt.NewChain("nat", "NOMAD_TEST_6"))
			must.NoError(t, vt.ipt.Append("nat", "NOMAD_TEST_6", "-p", "tcp", "-j", "ACCEPT"))
			t.Cleanup(func() {
				vt.ipt.ClearChain("nat", "NOMAD_TEST_6")
				vt.ipt.DeleteChain("nat", "NOMAD_TEST_6")
			})
			req := newRequest()
			req.addChains([]*chain{
				{
					table: "nat",
					chain: "NOMAD_TEST_5",
				},
				{
					table: "nat",
					chain: "NOMAD_TEST_6",
				},
			})
			req.addRules([]*rule{
				{
					table: "nat",
					chain: "NOMAD_TEST_6",
				},
				{
					table: "nat",
					chain: "NOMAD_TEST_5",
				},
			})

			chains, rules, err := vt.buildLists(req)
			must.NoError(t, err)

			must.SliceContains(t, chains.Slice(), &chain{table: "nat", chain: "NOMAD_TEST_6"})
			expectedRule := &rule{table: "nat", chain: "NOMAD_TEST_6", spec: []string{"-p", "tcp", "-j", "ACCEPT"}}
			must.True(t, rules.Contains(expectedRule),
				must.Sprintf("missing expected rule: %s", expectedRule))
		})
	})
}

func Test_virtTables_loopbackPortForwardsSupported(t *testing.T) {
	testCases := []struct {
		desc          string
		deviceContent string // empty string will result in no file
		globalContent string // empty string will result in no file
		result        bool
	}{
		{
			desc:          "global only enabled",
			globalContent: "1",
			result:        true,
		},
		{
			desc:          "global only disabled",
			globalContent: "0",
			result:        false,
		},
		{
			desc:          "device only route localnet enabled",
			deviceContent: "1",
			result:        true,
		},
		{
			desc:          "device only route localnet disabled",
			deviceContent: "0",
			result:        false,
		},
		{
			desc:          "global enabled device enabled",
			globalContent: "1",
			deviceContent: "1",
			result:        true,
		},
		{
			desc:          "global enabled device disabled",
			globalContent: "1",
			deviceContent: "0",
			result:        true,
		},
		{
			desc:          "global disabled device enabled",
			globalContent: "0",
			deviceContent: "1",
			result:        true,
		},
		{
			desc:          "global disabled device disabled",
			globalContent: "0",
			deviceContent: "0",
			result:        false,
		},
		{
			desc:   "all route localnet missing",
			result: false,
		},
	}

	deviceName := "test-dev"

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			tdir := t.TempDir()
			tmpl := filepath.Join(tdir, "/%s_route_localnet")
			devPath := fmt.Sprintf(tmpl, deviceName)
			globalPath := fmt.Sprintf(tmpl, routeLocalnetGlobalName)
			if tc.globalContent != "" {
				f, err := os.Create(globalPath)
				must.NoError(t, err)
				_, err = f.WriteString(tc.globalContent)
				must.NoError(t, err)
				f.Close()
			}

			if tc.deviceContent != "" {
				f, err := os.Create(devPath)
				must.NoError(t, err)
				_, err = f.WriteString(tc.deviceContent)
				must.NoError(t, err)
				f.Close()
			}

			vt, _ := testNew(t, WithRoutingLocalnetPathTemplate(tmpl))
			must.Eq(t, tc.result, vt.loopbackPortForwardsSupported(deviceName))
		})
	}
}

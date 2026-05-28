// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-set/v3"
	"github.com/hashicorp/nomad-driver-virt/net/filter"
	"github.com/hashicorp/nomad-driver-virt/testutil"
	mock_iptables "github.com/hashicorp/nomad-driver-virt/testutil/mock/iptables"
	virtnet "github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/helper/uuid"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/shoenig/test/must"
)

// NOTE: Many of the tests in this file contain two versions: mock and direct.
// The "mock" tests will utilize a mock iptables implementation to exercise the
// implementation. The "direct" tests require being run as root and the iptables
// command being available. The "direct" tests are built to be isolated and cleaned
// up after being run, but they will make changes to iptables and no guarantees are
// made about the resulting state. We just do our best. Good luck! \o/

func Test_virtTables_Configure(t *testing.T) {
	t.Run("mock", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			n := TestNewNames()
			hostIP := "192.168.44.22"
			taskIP := "10.0.22.33"
			ifaceName := "test0"

			ipt := mock_iptables.New(t).Expect(
				mock_iptables.AppendUnique{Table: "nat", Chain: n.chains.Nomad.Prerouting, RuleSpec: []string{
					"-d", hostIP, "-i", ifaceName, "-p", "tcp", "-m", "tcp", "--dport", "22222",
					"-j", "DNAT", "--to-destination", taskIP + ":8000"}},
				mock_iptables.AppendUnique{Table: "filter", Chain: n.chains.Nomad.Forward, RuleSpec: []string{
					"-d", taskIP, "-p", "tcp", "-m", "state", "--state", "NEW", "-m", "tcp",
					"--dport", "8000", "-j", "ACCEPT"}},
				mock_iptables.AppendUnique{Table: "nat", Chain: n.chains.Nomad.Prerouting, RuleSpec: []string{
					"-d", hostIP, "-i", ifaceName, "-p", "tcp", "-m", "tcp", "--dport", "22223",
					"-j", "DNAT", "--to-destination", taskIP + ":2222"}},
				mock_iptables.AppendUnique{Table: "filter", Chain: n.chains.Nomad.Forward, RuleSpec: []string{
					"-d", taskIP, "-p", "tcp", "-m", "state", "--state", "NEW", "-m", "tcp",
					"--dport", "2222", "-j", "ACCEPT"}},
			)
			defer ipt.AssertExpectations()

			vt, _ := TestNew(t,
				WithIPTables(ipt),
				WithNames(t, n),
				WithInterfaceByIPGetter(func(net.IP) (string, error) { return ifaceName, nil }),
			)
			resources := &drivers.Resources{
				Ports: &structs.AllocatedPorts{
					{
						Label:  "http",
						To:     8000,
						HostIP: hostIP,
						Value:  22222,
					},
					{
						Label:  "ssh",
						To:     2222,
						HostIP: hostIP,
						Value:  22223,
					},
				},
			}
			cfg := &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"http", "ssh"},
			}

			expected := [][]string{
				{
					"nat", n.chains.Nomad.Prerouting, "-d", hostIP, "-i", ifaceName, "-p",
					"tcp", "-m", "tcp", "--dport", "22222", "-j", "DNAT", "--to-destination",
					taskIP + ":8000",
				},
				{
					"filter", n.chains.Nomad.Forward, "-d", taskIP, "-p", "tcp", "-m",
					"state", "--state", "NEW", "-m", "tcp", "--dport", "8000", "-j", "ACCEPT",
				},
				{
					"nat", n.chains.Nomad.Prerouting, "-d", hostIP, "-i", ifaceName, "-p",
					"tcp", "-m", "tcp", "--dport", "22223", "-j", "DNAT", "--to-destination",
					taskIP + ":2222",
				},
				{
					"filter", n.chains.Nomad.Forward, "-d", taskIP, "-p", "tcp", "-m",
					"state", "--state", "NEW", "-m", "tcp", "--dport", "2222", "-j", "ACCEPT",
				},
			}

			teardownRules, err := vt.Configure(resources, cfg, taskIP)
			must.NoError(t, err)
			must.Eq(t, expected, teardownRules.Data.(Rules))
		})

		t.Run("loopback", func(t *testing.T) {
			n := TestNewNames()
			hostIP := "127.0.0.1"
			taskIP := "10.0.22.33"

			ifaceName := "testlo0"
			dstIfaceName := "testbr0"

			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ChainExists{Table: "nat", Chain: n.chains.Nomad.Output},
				mock_iptables.ChainExists{Table: "nat", Chain: n.chains.Nomad.Postrouting},
				mock_iptables.NewChain{Table: "nat", Chain: n.chains.Nomad.Output},
				mock_iptables.NewChain{Table: "nat", Chain: n.chains.Nomad.Postrouting},
				mock_iptables.InsertUnique{Table: "nat", Chain: "OUTPUT", Pos: 1, RuleSpec: []string{"-j", n.chains.Nomad.Output}},
				mock_iptables.InsertUnique{Table: "nat", Chain: "POSTROUTING", Pos: 1, RuleSpec: []string{"-j", n.chains.Nomad.Postrouting}},
				mock_iptables.AppendUnique{Table: "nat", Chain: n.chains.Nomad.Postrouting, RuleSpec: []string{
					"-o", dstIfaceName, "-m", "addrtype", "--src-type", "LOCAL", "--dst-type", "UNICAST", "-j", "MASQUERADE"}},
				mock_iptables.AppendUnique{Table: "nat", Chain: n.chains.Nomad.Output, RuleSpec: []string{
					"-s", hostIP, "-o", ifaceName, "-p", "tcp", "-m", "tcp", "--dport", "22222", "-j", "DNAT",
					"--to-destination", taskIP + ":8000"}},
			)
			defer ipt.AssertExpectations()

			vt, _ := TestNew(t,
				WithIPTables(ipt),
				WithNames(t, n),
				WithInterfaceByIPGetter(func(net.IP) (string, error) { return ifaceName, nil }),
				WithRoutingInterfaceByIPGetter(func(string) (string, error) { return dstIfaceName, nil }),
				WithRoutingLocalnetPathTemplate(enableLocalnetRouting(t, dstIfaceName)),
			)
			resources := &drivers.Resources{
				Ports: &structs.AllocatedPorts{
					{
						Label:  "http",
						To:     8000,
						HostIP: hostIP,
						Value:  22222,
					},
				},
			}
			cfg := &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"http"},
			}

			expected := [][]string{
				{
					"nat", n.chains.Nomad.Output, "-s", hostIP, "-o", ifaceName, "-p", "tcp", "-m", "tcp",
					"--dport", "22222", "-j", "DNAT", "--to-destination", taskIP + ":8000",
				},
			}

			teardownRules, err := vt.Configure(resources, cfg, taskIP)
			must.NoError(t, err)
			must.Eq(t, expected, teardownRules.Data.(Rules))
		})

		t.Run("loopback not enabled", func(t *testing.T) {
			n := TestNewNames()
			hostIP := "127.0.0.1"
			taskIP := "10.0.22.33"
			ifaceName := "testlo0"
			dstIfaceName := "testbr0"

			ipt := mock_iptables.New(t)
			defer ipt.AssertExpectations()

			vt, _ := TestNew(t,
				WithIPTables(ipt),
				WithNames(t, n),
				WithInterfaceByIPGetter(func(net.IP) (string, error) { return ifaceName, nil }),
				WithRoutingInterfaceByIPGetter(func(string) (string, error) { return dstIfaceName, nil }),
			)
			resources := &drivers.Resources{
				Ports: &structs.AllocatedPorts{
					{
						Label:  "http",
						To:     8000,
						HostIP: hostIP,
						Value:  22222,
					},
				},
			}
			cfg := &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"http"},
			}

			_, err := vt.Configure(resources, cfg, taskIP)
			must.ErrorContains(t, err, "loopback port forwarding not enabled")
		})
	})

	t.Run("direct", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			n := TestNewNames()
			hostIP := "192.168.44.22"
			taskIP := "10.0.22.33"
			maskedHostIP := "192.168.44.22/32"
			maskedTaskIP := "10.0.22.33/32"

			ifaceName := "test0"

			vt, cleanup := TestNew(t,
				WithNames(t, n),
				WithInterfaceByIPGetter(func(net.IP) (string, error) { return ifaceName, nil }),
			)
			t.Cleanup(cleanup)

			// Run setup so required chains exist.
			must.NoError(t, vt.setup())

			resources := &drivers.Resources{
				Ports: &structs.AllocatedPorts{
					{
						Label:  "http",
						To:     8000,
						HostIP: hostIP,
						Value:  22222,
					},
					{
						Label:  "ssh",
						To:     2222,
						HostIP: hostIP,
						Value:  22223,
					},
				},
			}
			cfg := &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"http", "ssh"},
			}

			expected := [][]string{
				{
					"nat", n.chains.Nomad.Prerouting, "-d", hostIP, "-i", ifaceName, "-p",
					"tcp", "-m", "tcp", "--dport", "22222", "-j", "DNAT", "--to-destination",
					taskIP + ":8000",
				},
				{
					"filter", n.chains.Nomad.Forward, "-d", taskIP, "-p", "tcp", "-m",
					"state", "--state", "NEW", "-m", "tcp", "--dport", "8000", "-j", "ACCEPT",
				},
				{
					"nat", n.chains.Nomad.Prerouting, "-d", hostIP, "-i", ifaceName, "-p",
					"tcp", "-m", "tcp", "--dport", "22223", "-j", "DNAT", "--to-destination",
					taskIP + ":2222",
				},
				{
					"filter", n.chains.Nomad.Forward, "-d", taskIP, "-p", "tcp", "-m",
					"state", "--state", "NEW", "-m", "tcp", "--dport", "2222", "-j", "ACCEPT",
				},
			}

			teardownRules, err := vt.Configure(resources, cfg, taskIP)
			must.NoError(t, err)
			must.Eq(t, expected, teardownRules.Data.(Rules))

			// Check rules in iptables
			natRules, err := vt.ipt.List("nat", n.chains.Nomad.Prerouting)
			must.NoError(t, err)
			filterRules, err := vt.ipt.List("filter", n.chains.Nomad.Forward)
			must.NoError(t, err)

			expectedNats := []string{
				fmt.Sprintf("-A %s -d %s -i %s -p tcp -m tcp --dport 22222 -j DNAT --to-destination %s:8000", n.chains.Nomad.Prerouting, maskedHostIP, ifaceName, taskIP),
				fmt.Sprintf("-A %s -d %s -i %s -p tcp -m tcp --dport 22223 -j DNAT --to-destination %s:2222", n.chains.Nomad.Prerouting, maskedHostIP, ifaceName, taskIP),
			}

			expectedFilters := []string{
				fmt.Sprintf("-A %s -d %s -p tcp -m state --state NEW -m tcp --dport 8000 -j ACCEPT", n.chains.Nomad.Forward, maskedTaskIP),
				fmt.Sprintf("-A %s -d %s -p tcp -m state --state NEW -m tcp --dport 2222 -j ACCEPT", n.chains.Nomad.Forward, maskedTaskIP),
			}

			must.SliceContainsSubset(t, natRules, expectedNats,
				must.Sprintf("nat rules: %#v", natRules))
			must.SliceContainsSubset(t, filterRules, expectedFilters,
				must.Sprintf("filter rules: %#v", filterRules))
		})

		t.Run("loopback", func(t *testing.T) {
			n := TestNewNames()
			hostIP := "127.0.0.1"
			taskIP := "10.0.22.33"
			maskedHostIP := "127.0.0.1/32"

			ifaceName := "testlo0"
			dstIfaceName := "testbr0"

			vt, _ := TestNew(t,
				WithNames(t, n),
				WithInterfaceByIPGetter(func(net.IP) (string, error) { return ifaceName, nil }),
				WithRoutingInterfaceByIPGetter(func(string) (string, error) { return dstIfaceName, nil }),
				WithRoutingLocalnetPathTemplate(enableLocalnetRouting(t, dstIfaceName)),
			)
			resources := &drivers.Resources{
				Ports: &structs.AllocatedPorts{
					{
						Label:  "http",
						To:     8000,
						HostIP: hostIP,
						Value:  22222,
					},
				},
			}
			cfg := &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"http"},
			}

			expected := [][]string{
				{
					"nat", n.chains.Nomad.Output, "-s", hostIP, "-o", ifaceName, "-p", "tcp", "-m", "tcp",
					"--dport", "22222", "-j", "DNAT", "--to-destination", taskIP + ":8000",
				},
			}

			teardownRules, err := vt.Configure(resources, cfg, taskIP)
			must.NoError(t, err)
			must.Eq(t, expected, teardownRules.Data.(Rules))

			natChains, err := vt.ipt.ListChains("nat")
			must.NoError(t, err)
			postRules, err := vt.ipt.List("nat", n.chains.Nomad.Postrouting)
			must.NoError(t, err)
			outRules, err := vt.ipt.List("nat", n.chains.Nomad.Output)
			must.NoError(t, err)

			expectedPostRules := []string{
				fmt.Sprintf("-A %s -o %s -m addrtype --src-type LOCAL --dst-type UNICAST -j MASQUERADE", n.chains.Nomad.Postrouting, dstIfaceName),
			}
			expectedOutRules := []string{
				fmt.Sprintf("-A %s -s %s -o %s -p tcp -m tcp --dport 22222 -j DNAT --to-destination %s:8000", n.chains.Nomad.Output, maskedHostIP, ifaceName, taskIP),
			}

			must.SliceContainsSubset(t, natChains, []string{n.chains.Postrouting, n.chains.Output},
				must.Sprintf("nat chains: %#v", natChains))
			must.SliceContainsSubset(t, postRules, expectedPostRules,
				must.Sprintf("POSTROUTING rules: %#v", postRules))
			must.SliceContainsSubset(t, outRules, expectedOutRules,
				must.Sprintf("OUTPUT rules: %#v", outRules))
		})

		t.Run("with teardown", func(t *testing.T) {
			n := TestNewNames()
			hostIP := "192.168.44.22"
			taskIP := "10.0.22.33"
			maskedHostIP := "192.168.44.22/32"
			maskedTaskIP := "10.0.22.33/32"

			ifaceName := "test0"

			vt, cleanup := TestNew(t,
				WithNames(t, n),
				WithInterfaceByIPGetter(func(net.IP) (string, error) { return ifaceName, nil }),
			)
			t.Cleanup(cleanup)

			// Run setup so required chains exist.
			must.NoError(t, vt.setup())

			resources := &drivers.Resources{
				Ports: &structs.AllocatedPorts{
					{
						Label:  "http",
						To:     8000,
						HostIP: hostIP,
						Value:  22222,
					},
					{
						Label:  "ssh",
						To:     2222,
						HostIP: hostIP,
						Value:  22223,
					},
				},
			}
			cfg := &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"http", "ssh"},
			}

			// Apply the updates.
			teardownRules, err := vt.Configure(resources, cfg, taskIP)
			must.NoError(t, err)

			// Now remove the updates.
			must.NoError(t, vt.Teardown(teardownRules))

			// Check rules in iptables
			natRules, err := vt.ipt.List("nat", n.chains.Nomad.Prerouting)
			must.NoError(t, err)
			filterRules, err := vt.ipt.List("filter", n.chains.Nomad.Forward)
			must.NoError(t, err)

			removedNats := []string{
				fmt.Sprintf("-A %s -d %s -i %s -p tcp -m tcp --dport 22222 -j DNAT --to-destination %s:8000", n.chains.Nomad.Prerouting, maskedHostIP, ifaceName, taskIP),
				fmt.Sprintf("-A %s -d %s -i %s -p tcp -m tcp --dport 22223 -j DNAT --to-destination %s:2222", n.chains.Nomad.Prerouting, maskedHostIP, ifaceName, taskIP),
			}

			removedFilters := []string{
				fmt.Sprintf("-A %s -d %s -p tcp -m state --state NEW -m tcp --dport 8000 -j ACCEPT", n.chains.Nomad.Forward, maskedTaskIP),
				fmt.Sprintf("-A %s -d %s -p tcp -m state --state NEW -m tcp --dport 2222 -j ACCEPT", n.chains.Nomad.Forward, maskedTaskIP),
			}

			for _, rn := range removedNats {
				must.SliceNotContains(t, natRules, rn)
			}

			for _, rf := range removedFilters {
				must.SliceNotContains(t, filterRules, rf)
			}
		})
	})
}

func Test_virtTables_Teardown(t *testing.T) {
	// This is currently a stub for Teardown tests, of which there are none. This
	// is because the Teardown function currently just calls `remove` so there is
	// nothing else to test. If/when that changes, this will no longer be a stub.
}

func Test_virtTables_setup(t *testing.T) {
	t.Run("mock", func(t *testing.T) {
		t.Run("not setup", func(t *testing.T) {
			n := TestNewNames()
			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ChainExists{Table: "nat", Chain: n.chains.Nomad.Prerouting},
				mock_iptables.ChainExists{Table: "filter", Chain: n.chains.Nomad.Forward},
				mock_iptables.NewChain{Table: "nat", Chain: n.chains.Nomad.Prerouting},
				mock_iptables.NewChain{Table: "filter", Chain: n.chains.Nomad.Forward},
				mock_iptables.InsertUnique{Table: "nat", Chain: "PREROUTING", Pos: 1, RuleSpec: []string{"-j", n.chains.Nomad.Prerouting}},
				mock_iptables.InsertUnique{Table: "filter", Chain: "FORWARD", Pos: 1, RuleSpec: []string{"-j", n.chains.Nomad.Forward}},
			)
			defer ipt.AssertExpectations()

			vt, _ := TestNew(t, WithNames(t, n), WithIPTables(ipt))
			must.NoError(t, vt.setup())
		})

		t.Run("already setup", func(t *testing.T) {
			n := TestNewNames()
			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ChainExists{Table: "nat", Chain: n.chains.Nomad.Prerouting},
				mock_iptables.ChainExists{Table: "filter", Chain: n.chains.Nomad.Forward},
				mock_iptables.NewChain{Table: "nat", Chain: n.chains.Nomad.Prerouting},
				mock_iptables.NewChain{Table: "filter", Chain: n.chains.Nomad.Forward},
				mock_iptables.InsertUnique{Table: "nat", Chain: "PREROUTING", Pos: 1, RuleSpec: []string{"-j", n.chains.Nomad.Prerouting}},
				mock_iptables.InsertUnique{Table: "filter", Chain: "FORWARD", Pos: 1, RuleSpec: []string{"-j", n.chains.Nomad.Forward}},
				mock_iptables.ChainExists{Table: "nat", Chain: n.chains.Nomad.Prerouting, Result: true},
				mock_iptables.ChainExists{Table: "filter", Chain: n.chains.Nomad.Forward, Result: true},
				mock_iptables.InsertUnique{Table: "nat", Chain: "PREROUTING", Pos: 1, RuleSpec: []string{"-j", n.chains.Nomad.Prerouting}},
				mock_iptables.InsertUnique{Table: "filter", Chain: "FORWARD", Pos: 1, RuleSpec: []string{"-j", n.chains.Nomad.Forward}},
			)
			defer ipt.AssertExpectations()
			vt, _ := TestNew(t, WithNames(t, n), WithIPTables(ipt))

			// Run an initial setup to ensure configuration exists.
			must.NoError(t, vt.setup())

			// Run the setup again.
			must.NoError(t, vt.setup())
		})

	})

	t.Run("direct", func(t *testing.T) {
		testutil.RequireIPTables(t)

		t.Run("not setup", func(t *testing.T) {
			vt, cleanup := TestNew(t)
			t.Cleanup(cleanup)

			// Run the setup.
			must.NoError(t, vt.setup())

			// Check that chains exist.
			natChains, err := vt.ipt.ListChains("nat")
			must.NoError(t, err)
			must.SliceContains(t, natChains, vt.names.chains.Nomad.Prerouting)

			filterChains, err := vt.ipt.ListChains("filter")
			must.NoError(t, err)
			must.SliceContains(t, filterChains, vt.names.chains.Nomad.Forward)

			// Check that rules exist.
			natRules, err := vt.ipt.List("nat", "PREROUTING")
			must.NoError(t, err)
			must.SliceContains(t, natRules, "-A PREROUTING -j "+vt.names.chains.Nomad.Prerouting)

			filterRules, err := vt.ipt.List("filter", "FORWARD")
			must.NoError(t, err)
			must.SliceContains(t, filterRules, "-A FORWARD -j "+vt.names.chains.Nomad.Forward)
		})

		t.Run("already setup", func(t *testing.T) {
			vt, cleanup := TestNew(t)
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
			nomadTest1 := genChainName()
			nomadTest2 := genChainName()

			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ChainExists{Table: "nat", Chain: nomadTest1},
				mock_iptables.ChainExists{Table: "nat", Chain: nomadTest2},
				mock_iptables.NewChain{Table: "nat", Chain: nomadTest1},
				mock_iptables.NewChain{Table: "nat", Chain: nomadTest2},
				mock_iptables.AppendUnique{Table: "nat", Chain: nomadTest1, RuleSpec: []string{"-p", "tcp", "-j", "ACCEPT"}},
			)
			defer ipt.AssertExpectations()

			vt, _ := TestNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChain([]*chain{
				{
					table: "nat",
					chain: nomadTest1,
				},
				{
					table: "nat",
					chain: nomadTest2,
				},
			}...)
			req.addRule(&rule{
				table: "nat",
				chain: nomadTest1,
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.add(req))
		})

		t.Run("ok insert", func(t *testing.T) {
			nomadTest1 := genChainName()
			nomadTest2 := genChainName()

			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ChainExists{Table: "nat", Chain: nomadTest1},
				mock_iptables.ChainExists{Table: "nat", Chain: nomadTest2},
				mock_iptables.NewChain{Table: "nat", Chain: nomadTest1},
				mock_iptables.NewChain{Table: "nat", Chain: nomadTest2},
				mock_iptables.AppendUnique{Table: "nat", Chain: nomadTest1, RuleSpec: []string{"-p", "tcp", "-j", "ACCEPT"}},
				mock_iptables.InsertUnique{Table: "nat", Chain: nomadTest2, Pos: 1, RuleSpec: []string{"-p", "tcp", "-j", "ACCEPT"}},
			)
			defer ipt.AssertExpectations()

			vt, _ := TestNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChain([]*chain{
				{
					table: "nat",
					chain: nomadTest1,
				},
				{
					table: "nat",
					chain: nomadTest2,
				},
			}...)
			req.addRule([]*rule{
				{
					table: "nat",
					chain: nomadTest1,
					spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
				},
				{
					table:    "nat",
					chain:    nomadTest2,
					position: 1,
					spec:     []string{"-p", "tcp", "-j", "ACCEPT"},
				},
			}...)

			must.NoError(t, vt.add(req))
		})

		t.Run("chain exists", func(t *testing.T) {
			nomadTest1 := genChainName()
			nomadTest2 := genChainName()

			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ChainExists{Table: "nat", Chain: nomadTest1},
				mock_iptables.ChainExists{Table: "nat", Chain: nomadTest2, Result: true},
				mock_iptables.NewChain{Table: "nat", Chain: nomadTest1},
				mock_iptables.AppendUnique{Table: "nat", Chain: nomadTest1, RuleSpec: []string{"-p", "tcp", "-j", "ACCEPT"}},
			)
			defer ipt.AssertExpectations()

			vt, _ := TestNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChain([]*chain{
				{
					table: "nat",
					chain: nomadTest1,
				},
				{
					table: "nat",
					chain: nomadTest2,
				},
			}...)
			req.addRule(&rule{
				table: "nat",
				chain: nomadTest1,
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.add(req))
		})

		t.Run("rule exists", func(t *testing.T) {
			nomadTest1 := genChainName()
			nomadTest2 := genChainName()

			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ChainExists{Table: "nat", Chain: nomadTest1, Result: true},
				mock_iptables.ChainExists{Table: "nat", Chain: nomadTest2, Result: true},
				mock_iptables.AppendUnique{Table: "nat", Chain: nomadTest1, RuleSpec: []string{"-p", "tcp", "-j", "ACCEPT"}},
			)
			defer ipt.AssertExpectations()

			vt, _ := TestNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChain([]*chain{
				{
					table: "nat",
					chain: nomadTest1,
				},
				{
					table: "nat",
					chain: nomadTest2,
				},
			}...)
			req.addRule(&rule{
				table: "nat",
				chain: nomadTest1,
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.add(req))
		})
	})

	t.Run("direct", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			nomadTest1 := genChainName()
			nomadTest2 := genChainName()

			vt, _ := TestNew(t)
			t.Cleanup(chainRemover(t, vt.ipt, nomadTest1, nomadTest2))

			req := newRequest()
			req.addChain([]*chain{
				{
					table: "nat",
					chain: nomadTest1,
				},
				{
					table: "nat",
					chain: nomadTest2,
				},
			}...)
			req.addRule(&rule{
				table: "nat",
				chain: nomadTest1,
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.add(req))
		})

		t.Run("ok insert", func(t *testing.T) {
			nomadTest1 := genChainName()
			nomadTest2 := genChainName()

			vt, _ := TestNew(t)
			t.Cleanup(chainRemover(t, vt.ipt, nomadTest1, nomadTest2))

			req := newRequest()
			req.addChain([]*chain{
				{
					table: "nat",
					chain: nomadTest1,
				},
				{
					table: "nat",
					chain: nomadTest2,
				},
			}...)
			req.addRule([]*rule{
				{
					table: "nat",
					chain: nomadTest1,
					spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
				},
				{
					table:    "nat",
					chain:    nomadTest2,
					position: 1,
					spec:     []string{"-p", "tcp", "-j", "ACCEPT"},
				},
			}...)

			must.NoError(t, vt.add(req))

			chains, err := vt.ipt.ListChains("nat")
			must.NoError(t, err)
			rules1, err := vt.ipt.List("nat", nomadTest1)
			must.NoError(t, err)
			rules2, err := vt.ipt.List("nat", nomadTest2)
			must.NoError(t, err)

			must.SliceContainsSubset(t, chains, []string{nomadTest1, nomadTest2})
			must.SliceContains(t, rules1, "-A "+nomadTest1+" -p tcp -j ACCEPT")
			must.SliceContains(t, rules2, "-A "+nomadTest2+" -p tcp -j ACCEPT")

		})

		t.Run("chain exists", func(t *testing.T) {
			nomadTest1 := genChainName()
			nomadTest2 := genChainName()

			vt, _ := TestNew(t)
			t.Cleanup(chainRemover(t, vt.ipt, nomadTest1, nomadTest2))

			// Create nomadTest2 chain so it already exists.
			must.NoError(t, vt.ipt.NewChain("nat", nomadTest2))

			req := newRequest()
			req.addChain([]*chain{
				{
					table: "nat",
					chain: nomadTest1,
				},
				{
					table: "nat",
					chain: nomadTest2,
				},
			}...)
			req.addRule(&rule{
				table: "nat",
				chain: nomadTest1,
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.add(req))

			chains, err := vt.ipt.ListChains("nat")
			must.NoError(t, err)
			rules, err := vt.ipt.List("nat", nomadTest1)
			must.NoError(t, err)

			must.SliceContainsSubset(t, chains, []string{nomadTest1, nomadTest2})
			must.SliceContains(t, rules, "-A "+nomadTest1+" -p tcp -j ACCEPT")
		})

		t.Run("rule exists", func(t *testing.T) {
			nomadTest1 := genChainName()
			nomadTest2 := genChainName()

			vt, _ := TestNew(t)
			t.Cleanup(chainRemover(t, vt.ipt, nomadTest1, nomadTest2))

			// Create both chains and rule so no changes are needed.
			must.NoError(t, vt.ipt.NewChain("nat", nomadTest1))
			must.NoError(t, vt.ipt.NewChain("nat", nomadTest2))
			must.NoError(t, vt.ipt.Append("nat", nomadTest1, "-p", "tcp", "-j", "ACCEPT"))

			req := newRequest()
			req.addChain([]*chain{
				{
					table: "nat",
					chain: nomadTest1,
				},
				{
					table: "nat",
					chain: nomadTest2,
				},
			}...)
			req.addRule(&rule{
				table: "nat",
				chain: nomadTest1,
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.add(req))

			// Validate that everything is still there.
			chains, err := vt.ipt.ListChains("nat")
			must.NoError(t, err)
			rules, err := vt.ipt.List("nat", nomadTest1)
			must.NoError(t, err)

			must.SliceContainsSubset(t, chains, []string{nomadTest1, nomadTest2})
			must.SliceContains(t, rules, "-A "+nomadTest1+" -p tcp -j ACCEPT")
		})
	})
}

func Test_virtTables_remove(t *testing.T) {
	t.Run("mock", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			nomadTest1 := genChainName()
			nomadTest2 := genChainName()

			ipt := mock_iptables.New(t).Expect(
				mock_iptables.DeleteIfExists{Table: "nat", Chain: nomadTest1, RuleSpec: []string{"-p", "tcp", "-j", "ACCEPT"}},
				mock_iptables.ClearAndDeleteChain{Table: "nat", Chain: nomadTest2},
			)
			defer ipt.AssertExpectations()

			vt, _ := TestNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChain(&chain{table: "nat", chain: nomadTest2})
			req.addRule(&rule{
				table: "nat",
				chain: nomadTest1,
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.remove(req))
		})

		t.Run("no chains", func(t *testing.T) {
			nomadTest1 := genChainName()

			ipt := mock_iptables.New(t).Expect(
				mock_iptables.DeleteIfExists{Table: "nat", Chain: nomadTest1, RuleSpec: []string{"-p", "tcp", "-j", "ACCEPT"}},
			)
			defer ipt.AssertExpectations()

			vt, _ := TestNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addRule(&rule{
				table: "nat",
				chain: nomadTest1,
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.remove(req))
		})

		t.Run("no rules", func(t *testing.T) {
			nomadTest1 := genChainName()

			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ClearAndDeleteChain{Table: "nat", Chain: nomadTest1},
			)
			defer ipt.AssertExpectations()

			vt, _ := TestNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChain(&chain{table: "nat", chain: nomadTest1})

			must.NoError(t, vt.remove(req))
		})

		t.Run("empty request", func(t *testing.T) {
			ipt := mock_iptables.New(t)
			defer ipt.AssertExpectations()

			vt, _ := TestNew(t, WithIPTables(ipt))
			req := newRequest()

			must.NoError(t, vt.remove(req))
		})

		t.Run("rule on deleted chain", func(t *testing.T) {
			nomadTest1 := genChainName()

			ipt := mock_iptables.New(t).Expect(
				mock_iptables.ClearAndDeleteChain{Table: "nat", Chain: nomadTest1},
			)
			defer ipt.AssertExpectations()

			vt, _ := TestNew(t, WithIPTables(ipt))
			req := newRequest()
			req.addChain(&chain{table: "nat", chain: nomadTest1})
			req.addRule(&rule{
				table: "nat",
				chain: nomadTest1,
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.remove(req))
		})
	})

	t.Run("direct", func(t *testing.T) {
		t.Run("ok", func(t *testing.T) {
			nomadTest1 := genChainName()
			nomadTest2 := genChainName()

			vt, _ := TestNew(t)
			t.Cleanup(chainRemover(t, vt.ipt, nomadTest1, nomadTest2))

			must.NoError(t, vt.ipt.NewChain("nat", nomadTest1))
			must.NoError(t, vt.ipt.NewChain("nat", nomadTest2))
			must.NoError(t, vt.ipt.Append("nat", nomadTest1, "-p", "tcp", "-j", "ACCEPT"))

			req := newRequest()
			req.addChain(&chain{table: "nat", chain: nomadTest2})
			req.addRule(&rule{
				table: "nat",
				chain: nomadTest1,
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.remove(req))

			chains, err := vt.ipt.ListChains("nat")
			must.NoError(t, err)
			rules, err := vt.ipt.List("nat", nomadTest1)
			must.NoError(t, err)
			must.SliceContains(t, chains, nomadTest1)
			must.SliceNotContains(t, chains, nomadTest2)
			must.Eq(t, rules, []string{"-N " + nomadTest1})
		})

		t.Run("no existing rules", func(t *testing.T) {
			nomadTest1 := genChainName()
			nomadTest2 := genChainName()

			vt, _ := TestNew(t)
			t.Cleanup(chainRemover(t, vt.ipt, nomadTest1, nomadTest2))

			must.NoError(t, vt.ipt.NewChain("nat", nomadTest1))
			must.NoError(t, vt.ipt.NewChain("nat", nomadTest2))

			req := newRequest()
			req.addChain(&chain{table: "nat", chain: nomadTest2})
			req.addRule(&rule{
				table: "nat",
				chain: nomadTest1,
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.remove(req))

			chains, err := vt.ipt.ListChains("nat")
			must.NoError(t, err)
			rules, err := vt.ipt.List("nat", nomadTest1)
			must.NoError(t, err)
			must.SliceContains(t, chains, nomadTest1)
			must.SliceNotContains(t, chains, nomadTest2)
			must.Eq(t, rules, []string{"-N " + nomadTest1})
		})

		t.Run("no existing chains", func(t *testing.T) {
			nomadTest1 := genChainName()
			nomadTest2 := genChainName()

			vt, _ := TestNew(t)

			req := newRequest()
			req.addChain(&chain{table: "nat", chain: nomadTest2})
			req.addRule(&rule{
				table: "nat",
				chain: nomadTest1,
				spec:  []string{"-p", "tcp", "-j", "ACCEPT"},
			})

			must.NoError(t, vt.remove(req))

			chains, err := vt.ipt.ListChains("nat")
			must.NoError(t, err)
			must.SliceNotContains(t, chains, nomadTest1)
			must.SliceNotContains(t, chains, nomadTest2)
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

			vt, _ := TestNew(t, WithRoutingLocalnetPathTemplate(tmpl))
			must.Eq(t, tc.result, vt.loopbackPortForwardsSupported(deviceName))
		})
	}
}

// genChainName generates a unique chain name for tests.
func genChainName() string {
	return fmt.Sprintf("NOMAD_VT_TEST_%s", uuid.Short())
}

// chainRemover returns a function that deletes chains from the nat table.
func chainRemover(t *testing.T, ipt IPTables, names ...string) func() {
	return func() {
		t.Helper()
		for _, name := range names {
			if err := ipt.ClearAndDeleteChain("nat", name); err != nil {
				t.Logf("error deleting chain %q in nat table (%s), continuing...", name, err)
				continue
			}
		}
	}
}

// enableLocalnetRouting writes a file with a `1` for content and returns
// the path. Can be used with `WithRoutingLocalnetTemplate`.
func enableLocalnetRouting(t *testing.T, device string) string {
	if device == "" {
		device = routeLocalnetGlobalName
	}

	tmpl := filepath.Join(t.TempDir(), "localnet-routing-%s")
	path := fmt.Sprintf(tmpl, device)
	f, err := os.Create(path)
	must.NoError(t, err)
	_, err = f.WriteString("1")
	must.NoError(t, err)
	must.NoError(t, f.Close())

	return tmpl
}

var (
	_ filter.Filter = (*virtTables)(nil)
)

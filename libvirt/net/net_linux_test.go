// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package net

import (
	"fmt"
	"testing"

	"github.com/coreos/go-iptables/iptables"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/libvirt"
	"github.com/hashicorp/nomad/plugins/shared/structs"
	"github.com/shoenig/test/must"
)

func TestController_Fingerprint(t *testing.T) {

	// Use a populated mock shim to test that we query and correctly populate
	// the passed attributes.
	mockController := NewController(hclog.NewNullLogger(), &libvirt.ConnectMock{})

	mockControllerAttrs := map[string]*structs.Attribute{}
	mockController.Fingerprint(mockControllerAttrs)

	expectedOutput := map[string]*structs.Attribute{
		"driver.virt.network.default.state":       structs.NewStringAttribute("active"),
		"driver.virt.network.default.bridge_name": structs.NewStringAttribute("virbr0"),
		"driver.virt.network.routed.state":        structs.NewStringAttribute("inactive"),
		"driver.virt.network.routed.bridge_name":  structs.NewStringAttribute("br0"),
	}
	must.Eq(t, expectedOutput, mockControllerAttrs)

	// Set the shim to our empty mock, to ensure we do not panic or have any
	// other undesired outcome when the process does not find any networks
	// available on the host.
	mockEmptyController := NewController(hclog.NewNullLogger(), &libvirt.ConnectMockEmpty{})

	mockEmptyControllerAttrs := map[string]*structs.Attribute{}
	mockEmptyController.Fingerprint(mockEmptyControllerAttrs)
	must.Eq(t, map[string]*structs.Attribute{}, mockEmptyControllerAttrs)
}

func TestController_ensureIPTables(t *testing.T) {

	ipt, err := iptables.New()
	must.NoError(t, err)

	// Try listing our custom chains to ensure they do not exist before the
	// test starts.
	natChains, err := ipt.ListChains(iptablesNATTableName)
	must.NoError(t, err)
	must.SliceNotContains(t, natChains, preroutingIPTablesChainName)

	filterChains, err := ipt.ListChains(iptablesFilterTableName)
	must.NoError(t, err)
	must.SliceNotContains(t, filterChains, forwardIPTablesChainName)

	// Add a cleanup function which will remove all the added iptables chain
	// and rule entries. This avoids polluting the machine that runs the test,
	// so our development machines do not require manual intervention after
	// each test run.
	//
	// Any errors running the cleanup commands are logged, so developers can
	// spot these and perform manual fixes. Manual fixes:
	//  - sudo iptables -t filter -D FORWARD -j NOMAD_VT_FW
	//  - sudo iptables -F NOMAD_VT_FW -t filter
	//  - sudo iptables -X NOMAD_VT_FW -t filter
	//  - sudo iptables -t nat -D PREROUTING -j NOMAD_VT_PRT
	//  - sudo iptables -F NOMAD_VT_PRT -t nat
	//  - sudo iptables -X NOMAD_VT_PRT -t nat
	t.Cleanup(func() {
		fn := func(e error) {
			if e != nil {
				t.Log(fmt.Sprint("failed to cleanup iptables: %w", e))
			}
		}

		fn(ipt.Delete(iptablesNATTableName, "PREROUTING", []string{"-j", preroutingIPTablesChainName}...))
		fn(ipt.ClearChain(iptablesNATTableName, preroutingIPTablesChainName))
		fn(ipt.DeleteChain(iptablesNATTableName, preroutingIPTablesChainName))

		fn(ipt.Delete(iptablesFilterTableName, "FORWARD", []string{"-j", forwardIPTablesChainName}...))
		fn(ipt.ClearChain(iptablesFilterTableName, forwardIPTablesChainName))
		fn(ipt.DeleteChain(iptablesFilterTableName, forwardIPTablesChainName))
	})

	mockController := &Controller{logger: hclog.NewNullLogger()}

	// Trigger the ensure function which should add our base iptables chains
	// and rules for the driver.
	must.NoError(t, mockController.ensureIPTables())

	// Ensure the custom chain is found within the NAT table and check that the
	// table has a jump rule to the custom chain.
	natChains, err = ipt.ListChains(iptablesNATTableName)
	must.NoError(t, err)
	must.SliceContains(t, natChains, preroutingIPTablesChainName)

	natRules, err := ipt.List(iptablesNATTableName, "PREROUTING")
	must.NoError(t, err)
	must.SliceContains(t, natRules, "-A PREROUTING -j "+preroutingIPTablesChainName)

	// Ensure the custom chain is found within the filter table and check that
	// the table has a jump rule to the custom chain.
	filterChains, err = ipt.ListChains(iptablesFilterTableName)
	must.NoError(t, err)
	must.SliceContains(t, filterChains, forwardIPTablesChainName)

	filterRules, err := ipt.List(iptablesFilterTableName, "FORWARD")
	must.NoError(t, err)
	must.SliceContains(t, filterRules, "-A FORWARD -j "+forwardIPTablesChainName)

	// Trigger the ensure function again. This tests that the function can
	// handle being run multiple times without error, whilst maintaining the
	// iptables setup we require.
	must.NoError(t, mockController.ensureIPTables())

	// Ensure the custom chain is found within the NAT table and check that the
	// table has a jump rule to the custom chain.
	natChains, err = ipt.ListChains(iptablesNATTableName)
	must.NoError(t, err)
	must.SliceContains(t, natChains, preroutingIPTablesChainName)

	natRules, err = ipt.List(iptablesNATTableName, "PREROUTING")
	must.NoError(t, err)
	must.SliceContains(t, natRules, "-A PREROUTING -j "+preroutingIPTablesChainName)

	// Ensure the custom chain is found within the filter table and check that
	// the table has a jump rule to the custom chain.
	filterChains, err = ipt.ListChains(iptablesFilterTableName)
	must.NoError(t, err)
	must.SliceContains(t, filterChains, forwardIPTablesChainName)

	filterRules, err = ipt.List(iptablesFilterTableName, "FORWARD")
	must.NoError(t, err)
	must.SliceContains(t, filterRules, "-A FORWARD -j "+forwardIPTablesChainName)
}

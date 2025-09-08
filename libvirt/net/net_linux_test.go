// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package net

import (
	"fmt"
	stdnet "net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-iptables/iptables"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/libvirt"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	nomadstructs "github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/shared/structs"
	"github.com/shoenig/test/must"
	"libvirt.org/go/libvirtxml"
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
	// and rule entries.
	t.Cleanup(func() { iptablesCleanup(t, ipt) })

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

func TestController_VMStartedBuild(t *testing.T) {

	mockController := &Controller{
		dhcpLeaseDiscoveryInterval: 100 * time.Millisecond,
		dhcpLeaseDiscoveryTimeout:  500 * time.Millisecond,
		logger:                     hclog.NewNullLogger(),
		netConn:                    &libvirt.ConnectMock{},
		interfaceByIPGetter:        func(_ stdnet.IP) (string, error) { return "enp126s0", nil },
	}

	must.NoError(t, mockController.Init())

	ipt, err := iptables.New()
	must.NoError(t, err)

	// Add a cleanup function which will remove all the added iptables chain
	// and rule entries.
	t.Cleanup(func() { iptablesCleanup(t, ipt) })

	// Ensure passing a nil request object doesn't cause the function to panic.
	nilRequestResp, err := mockController.VMStartedBuild(nil)
	must.ErrorContains(t, err, "no request provided")
	must.Nil(t, nilRequestResp)

	// Ensure passing an empty request object doesn't cause the function to
	// panic.
	nilRequestResp, err = mockController.VMStartedBuild(&net.VMStartedBuildRequest{})
	must.NoError(t, err)
	must.NotNil(t, nilRequestResp)
	must.Nil(t, nilRequestResp.TeardownSpec)

	// Pass a request that doesn't contain any configured networks to ensure we
	// correctly handle that.
	emptyNetworkRequestResp, err := mockController.VMStartedBuild(&net.VMStartedBuildRequest{
		NetConfig: &net.NetworkInterfacesConfig{},
		Resources: &drivers.Resources{},
	})
	must.NoError(t, err)
	must.NotNil(t, emptyNetworkRequestResp)
	must.Nil(t, emptyNetworkRequestResp.TeardownSpec)

	// Test a correct and full request.
	fullReq := net.VMStartedBuildRequest{
		DomainName: "nomad-0ea818bc",
		Hostname:   "nomad-0ea818bc",
		Hwaddrs:    []string{"52:54:00:1c:7c:14"},
		NetConfig: &net.NetworkInterfacesConfig{
			{
				Bridge: &net.NetworkInterfaceBridgeConfig{
					Name:  "virbr0",
					Ports: []string{"ssh", "nomad"},
				},
			},
		},
		Resources: &drivers.Resources{
			Ports: &nomadstructs.AllocatedPorts{
				{
					Label:  "ssh",
					Value:  27494,
					To:     22,
					HostIP: "10.0.1.161",
				},
				{
					Label:  "nomad",
					Value:  27512,
					To:     4646,
					HostIP: "10.0.1.161",
				},
			},
		},
	}

	fullReqResp, err := mockController.VMStartedBuild(&fullReq)
	must.NoError(t, err)
	must.NotNil(t, fullReqResp)
	must.NotNil(t, fullReqResp.DriverNetwork)
	must.NotNil(t, fullReqResp.TeardownSpec)

	must.Eq(t, &drivers.DriverNetwork{IP: "192.168.122.58"}, fullReqResp.DriverNetwork)

	expectedTeardownRules := [][]string{
		{"filter", "NOMAD_VT_FW", "-d", "192.168.122.58", "-p", "tcp",
			"-m", "state", "--state", "NEW", "-m", "tcp", "--dport",
			"22", "-j", "ACCEPT",
		},
		{"nat", "NOMAD_VT_PRT", "-d", "10.0.1.161", "-i", "enp126s0",
			"-p", "tcp", "-m", "tcp", "--dport", "27494", "-j", "DNAT",
			"--to-destination", "192.168.122.58:22",
		},
		{"filter", "NOMAD_VT_FW", "-d", "192.168.122.58", "-p", "tcp",
			"-m", "state", "--state", "NEW", "-m", "tcp", "--dport",
			"4646", "-j", "ACCEPT",
		},
		{"nat", "NOMAD_VT_PRT", "-d", "10.0.1.161", "-i", "enp126s0",
			"-p", "tcp", "-m", "tcp", "--dport", "27512", "-j", "DNAT",
			"--to-destination", "192.168.122.58:4646",
		},
	}

	must.EqFunc(t, expectedTeardownRules, fullReqResp.TeardownSpec.IPTablesRules, func(a, b [][]string) bool {
		if len(a) != len(b) {
			return false
		}

		var found int

		for _, ruleA := range a {
			for _, ruleB := range b {
				if !reflect.DeepEqual(ruleA, ruleB) {
					continue
				}
				found++
			}
		}
		return found == len(a)
	})
}

func TestController_networkNameFromBridgeName(t *testing.T) {

	// Create out controller which has a mocked connection with identified
	// networks.
	mockController := &Controller{
		logger:  hclog.NewNullLogger(),
		netConn: &libvirt.ConnectMock{},
	}

	// Query a non-existent network.
	nonExistentResp, err := mockController.networkNameFromBridgeName("non-existent-bridge")
	must.ErrorContains(t, err, "failed to find network with bridge")
	must.Eq(t, nonExistentResp, "")

	// Query a network which does exist.
	virbr0Resp, err := mockController.networkNameFromBridgeName("virbr0")
	must.NoError(t, err)
	must.Eq(t, virbr0Resp, "default")

	// Create a controller with a connection that does not have any identified
	// networks. This allows us to ensure the behaviour is the same on hosts
	// which have no networks, as one that do.
	mockEmptyController := &Controller{
		logger:  hclog.NewNullLogger(),
		netConn: &libvirt.ConnectMockEmpty{},
	}

	mockEmptyResp, err := mockEmptyController.networkNameFromBridgeName("virbr0")
	must.ErrorContains(t, err, "failed to find network with bridge")
	must.Eq(t, mockEmptyResp, "")
}

func TestController_discoverDHCPLeaseIP(t *testing.T) {

	// Create out controller which has a mocked connection with identified
	// networks and low discovery time durations, so the tests do not take ages
	// to run.
	mockController := &Controller{
		logger:                     hclog.NewNullLogger(),
		netConn:                    &libvirt.ConnectMock{},
		dhcpLeaseDiscoveryInterval: 100 * time.Millisecond,
		dhcpLeaseDiscoveryTimeout:  500 * time.Millisecond,
	}

	defaultNet, err := mockController.netConn.LookupNetworkByName("default")
	must.NoError(t, err)
	must.NotNil(t, defaultNet)

	// Query for a domain that does not have a lease entry and ensure the
	// timeout is triggered.
	nonExistentResp, mac, err := mockController.discoverDHCPLeaseIP(defaultNet, "non-existent-domain",
		"default", []string{"00:00:00:00:00:00"})
	must.ErrorContains(t, err, "timeout reached discovering DHCP lease")
	must.Eq(t, nonExistentResp, "")
	must.Eq(t, mac, "")

	// Query for a domain which does have a lease.
	existentResp, mac, err := mockController.discoverDHCPLeaseIP(defaultNet, "nomad-0ea818bc",
		"default", []string{"52:54:00:1c:7c:14"})
	must.NoError(t, err)
	must.Eq(t, existentResp, "192.168.122.58")
	must.Eq(t, mac, "52:54:00:1c:7c:14")

	// Query for a domain which does have a lease using multiple MAC addresses.
	existentResp, mac, err = mockController.discoverDHCPLeaseIP(defaultNet, "nomad-0ea818bc",
		"default", []string{"11:11:11:11:11:11", "52:54:00:1c:7c:14", "22:22:22:22:22:22"})
	must.NoError(t, err)
	must.Eq(t, existentResp, "192.168.122.58")
	must.Eq(t, mac, "52:54:00:1c:7c:14")

	// Query for a domain with several matching leases.
	multiResp, mac, err := mockController.discoverDHCPLeaseIP(defaultNet, "nomad-3edc43aa",
		"default", []string{"11:22:33:44:55:66"})
	must.NoError(t, err)
	must.Eq(t, multiResp, "192.168.122.65")
	must.Eq(t, mac, "11:22:33:44:55:66")

	// Query for domain with matching expired lease.
	expiredResp, mac, err := mockController.discoverDHCPLeaseIP(defaultNet, "nomad-eabba892",
		"default", []string{"66:55:44:33:22:11"})
	must.ErrorContains(t, err, "timeout reached discovering DHCP lease")
	must.Eq(t, expiredResp, "")
	must.Eq(t, mac, "")

	// Query for domain with matching MAC address only.
	macOnlyResp, mac, err := mockController.discoverDHCPLeaseIP(defaultNet, "different-hostname",
		"default", []string{"52:54:00:1c:7c:14"})
	must.ErrorContains(t, err, "timeout reached discovering DHCP lease")
	must.Eq(t, macOnlyResp, "")
	must.Eq(t, mac, "")

	// Query for domain with matching MAC address and empty hostname on lease.
	macOnlyNoHostnameResp, mac, err := mockController.discoverDHCPLeaseIP(defaultNet, "custom-hostname",
		"default", []string{"11:22:11:22:11:22"})
	must.NoError(t, err)
	must.Eq(t, macOnlyNoHostnameResp, "192.168.122.99")
	must.Eq(t, mac, "11:22:11:22:11:22")
}

func TestController_configureIPTables(t *testing.T) {

	mockController := &Controller{
		logger:              hclog.NewNullLogger(),
		netConn:             &libvirt.ConnectMock{},
		interfaceByIPGetter: func(_ stdnet.IP) (string, error) { return "enp126s0", nil },
	}

	// Create driver and network interface request arguments. The allocated
	// ports includes a port not specified in the task config, to ensure this
	// does not get configured.
	driverReq := drivers.Resources{
		Ports: &nomadstructs.AllocatedPorts{
			{
				Label:  "ssh",
				Value:  27494,
				To:     22,
				HostIP: "10.0.1.161",
			},
			{
				Label:  "nomad",
				Value:  27512,
				To:     4646,
				HostIP: "10.0.1.161",
			},
			{
				Label:  "http",
				Value:  27513,
				To:     80,
				HostIP: "10.0.1.161",
			},
		},
	}

	netInterfaceReq := net.NetworkInterfaceBridgeConfig{
		Name:  "virbr0",
		Ports: []string{"ssh", "nomad"},
	}

	// Init the controller, so we have the required iptables chains available.
	must.NoError(t, mockController.Init())

	ipt, err := iptables.New()
	must.NoError(t, err)

	// Add a cleanup function which will remove all the added iptables chain
	// and rule entries.
	t.Cleanup(func() { iptablesCleanup(t, ipt) })

	// Execute the function, collecting the teardown rules and building our
	// expected output.
	actualTeardownRules, err := mockController.configureIPTables(
		&driverReq, &netInterfaceReq, "192.168.122.58")
	must.NoError(t, err)

	expectedTeardownRules := [][]string{
		{"filter", "NOMAD_VT_FW", "-d", "192.168.122.58", "-p", "tcp",
			"-m", "state", "--state", "NEW", "-m", "tcp", "--dport",
			"22", "-j", "ACCEPT",
		},
		{"nat", "NOMAD_VT_PRT", "-d", "10.0.1.161", "-i", "enp126s0",
			"-p", "tcp", "-m", "tcp", "--dport", "27494", "-j", "DNAT",
			"--to-destination", "192.168.122.58:22",
		},
		{"filter", "NOMAD_VT_FW", "-d", "192.168.122.58", "-p", "tcp",
			"-m", "state", "--state", "NEW", "-m", "tcp", "--dport",
			"4646", "-j", "ACCEPT",
		},
		{"nat", "NOMAD_VT_PRT", "-d", "10.0.1.161", "-i", "enp126s0",
			"-p", "tcp", "-m", "tcp", "--dport", "27512", "-j", "DNAT",
			"--to-destination", "192.168.122.58:4646",
		},
	}

	// Perform the equality check ensuring the returned rules match exactly
	// what we expected.
	must.EqFunc(t, expectedTeardownRules, actualTeardownRules, func(a, b [][]string) bool {

		if len(a) != len(b) {
			return false
		}

		var found int

		for _, ruleA := range a {
			for _, ruleB := range b {
				if !reflect.DeepEqual(ruleA, ruleB) {
					continue
				}
				found++
			}
		}
		return found == len(a)
	})

	// List the rules directly from the host to ensure we have also made
	// changes there rather than just populate a return object.
	//
	// We can't use expectedTeardownRules as the iptables return includes
	// bit length of the subnet mask.
	expectedNATRules := [][]string{
		{"nat", "NOMAD_VT_PRT", "-d", "10.0.1.161/32", "-i", "enp126s0",
			"-p", "tcp", "-m", "tcp", "--dport", "27494", "-j", "DNAT",
			"--to-destination", "192.168.122.58:22",
		},
		{"nat", "NOMAD_VT_PRT", "-d", "10.0.1.161/32", "-i", "enp126s0",
			"-p", "tcp", "-m", "tcp", "--dport", "27512", "-j", "DNAT",
			"--to-destination", "192.168.122.58:4646",
		},
	}

	natRules, err := ipt.List("nat", "NOMAD_VT_PRT")
	must.NoError(t, err)
	must.SliceContains(t, natRules, "-A "+strings.Join(expectedNATRules[0][1:], " "))
	must.SliceContains(t, natRules, "-A "+strings.Join(expectedNATRules[1][1:], " "))

	expectedFilterRules := [][]string{
		{"filter", "NOMAD_VT_FW", "-d", "192.168.122.58/32", "-p", "tcp",
			"-m", "state", "--state", "NEW", "-m", "tcp", "--dport",
			"22", "-j", "ACCEPT",
		},
		{"filter", "NOMAD_VT_FW", "-d", "192.168.122.58/32", "-p", "tcp",
			"-m", "state", "--state", "NEW", "-m", "tcp", "--dport",
			"4646", "-j", "ACCEPT",
		},
	}

	filterRules, err := ipt.List("filter", "NOMAD_VT_FW")
	must.NoError(t, err)
	must.SliceContains(t, filterRules, "-A "+strings.Join(expectedFilterRules[0][1:], " "))
	must.SliceContains(t, filterRules, "-A "+strings.Join(expectedFilterRules[1][1:], " "))
}

func TestController_VMTerminatedTeardown(t *testing.T) {

	mockController := &Controller{
		logger:  hclog.NewNullLogger(),
		netConn: &libvirt.ConnectMock{},
	}

	// Call the function with a nil argument, to ensure it handles this
	// correctly and doesn't panic.
	resp, err := mockController.VMTerminatedTeardown(nil)
	must.NoError(t, err)
	must.NotNil(t, resp)

	// Create a couple of iptables entries we will use moving forward. They go
	// into the default chains, rather than the custom driver ones, so we don't
	// need to manage the init.
	iptablesRules := [][]string{
		{"filter", "FORWARD", "-d", "192.168.122.58", "-p", "tcp",
			"-m", "state", "--state", "NEW", "-m", "tcp", "--dport",
			"22", "-j", "ACCEPT",
		},
		{"nat", "PREROUTING", "-d", "10.0.1.161", "-i", "enp126s0",
			"-p", "tcp", "-m", "tcp", "--dport", "27494", "-j", "DNAT",
			"--to-destination", "192.168.122.58:22",
		},
	}

	// Create a teardown spec which has rules that do not exist currently on
	// the host. It ensures we can loop through, without generating an error.
	nonExistentRuleArgs := net.VMTerminatedTeardownRequest{
		TeardownSpec: &net.TeardownSpec{
			IPTablesRules: iptablesRules,
			Network:       "default",
		},
	}
	resp, err = mockController.VMTerminatedTeardown(&nonExistentRuleArgs)
	must.NoError(t, err)
	must.NotNil(t, resp)

	// Grab a handle on iptables, so we can create a couple of test rules for
	// deletion. The library and iptables uses a lock per write, so this won't
	// conflict with calls to VMTerminatedTeardown.
	ipt, err := iptables.New()
	must.NoError(t, err)

	// Add a cleanup function which will remove all the added iptables chain
	// rule entries, in the event of a failure in the post stop failing to do
	// this.
	//
	// Any errors running the cleanup commands are logged, so developers can
	// spot these and perform manual fixes. To run a manual removal, execute
	// "sudo iptables -D" followed by the arguments details in each of the
	// arrays.
	t.Cleanup(func() {
		fn := func(e error) {
			if e != nil {
				t.Log(fmt.Sprint("failed to cleanup iptables: %w", e))
			}
		}
		for _, rule := range iptablesRules {
			fn(ipt.DeleteIfExists(rule[0], rule[1], rule[2:]...))
		}
	})

	// Create some iptables rules, which are based on a real-world example.
	for _, rule := range iptablesRules {
		must.NoError(t, ipt.Append(rule[0], rule[1], rule[2:]...))
	}

	// Perform the post stop request and check that the entries have been
	// removed by the process, as expected.
	existentRuleArgs := net.VMTerminatedTeardownRequest{
		TeardownSpec: &net.TeardownSpec{
			IPTablesRules: iptablesRules,
			Network:       "default",
		},
	}
	resp, err = mockController.VMTerminatedTeardown(&existentRuleArgs)
	must.NoError(t, err)
	must.NotNil(t, resp)

	for _, rule := range iptablesRules {
		rules, err := ipt.List(rule[0], rule[1])
		must.NoError(t, err)
		must.SliceNotContains(t, rules, strings.Join(rule[2:], " "))
	}
}

func Test_removeIPReservation(t *testing.T) {
	mockController := &Controller{
		logger:  hclog.NewNullLogger(),
		netConn: &libvirt.ConnectMock{},
	}

	testCases := []struct {
		desc        string
		network     string
		reservation *libvirtxml.NetworkDHCPHost
		err         string
	}{
		{
			desc:    "reservation does not exist",
			network: "default",
			reservation: &libvirtxml.NetworkDHCPHost{
				IP:   "127.0.0.1",
				MAC:  "00:00:00:11:11:11",
				Name: "testing",
			},
		},
		{
			desc:    "reservation exists",
			network: "default",
			reservation: &libvirtxml.NetworkDHCPHost{
				IP:   "192.168.122.45",
				MAC:  "00:11:22:33:44:55",
				Name: "test-hostname",
			},
		},
		{
			desc:    "network does not exist",
			network: "does-not-exist",
			reservation: &libvirtxml.NetworkDHCPHost{
				IP:   "192.168.122.45",
				MAC:  "00:11:22:33:44:55",
				Name: "test-hostname",
			},
			err: "failed to find network",
		},
	}

	for _, tc := range testCases {
		entry, err := tc.reservation.Marshal()
		must.NoError(t, err)

		err = mockController.removeIPReservation(tc.network, entry)
		if tc.err != "" {
			println("looked up network", tc.network)
			must.ErrorContains(t, err, tc.err)
		} else {
			must.NoError(t, err)
		}
	}
}

func Test_ipReservationExists(t *testing.T) {
	mockController := &Controller{
		logger:  hclog.NewNullLogger(),
		netConn: &libvirt.ConnectMock{},
	}

	testCases := []struct {
		desc           string
		reservation    *libvirtxml.NetworkDHCPHost
		reservationRaw string
		exists         bool
		err            string
	}{
		{
			desc: "reservation does not exist",
			reservation: &libvirtxml.NetworkDHCPHost{
				IP:   "127.0.0.1",
				MAC:  "00:00:00:11:11:11",
				Name: "testing",
			},
		},
		{
			desc: "reservation exists",
			reservation: &libvirtxml.NetworkDHCPHost{
				IP:   "192.168.122.45",
				MAC:  "00:11:22:33:44:55",
				Name: "test-hostname",
			},
			exists: true,
		},
		{
			desc:           "invalid reservation",
			reservationRaw: "-",
			exists:         false,
			err:            "could not parse",
		},
	}

	network, err := mockController.netConn.LookupNetworkByName("default")
	must.NoError(t, err)

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			entry := tc.reservationRaw
			if tc.reservation != nil {
				entry, err = tc.reservation.Marshal()
				must.NoError(t, err)
			}

			exists, err := mockController.ipReservationExists(network, entry)
			must.Eq(t, tc.exists, exists)
			if tc.err != "" {
				must.ErrorContains(t, err, tc.err)
			} else {
				must.NoError(t, err)
			}
		})
	}
}

func Test_releaseDHCPLease(t *testing.T) {
	mockController := &Controller{
		logger:              hclog.NewNullLogger(),
		netConn:             &libvirt.ConnectMock{},
		ipByInterfaceGetter: func(_ string) (stdnet.IP, error) { return stdnet.ParseIP("192.168.122.1"), nil },
	}

	testCases := []struct {
		desc           string
		reservation    *libvirtxml.NetworkDHCPHost
		reservationRaw string
		network        string
		err            string
	}{
		{
			desc: "ok",
			reservation: &libvirtxml.NetworkDHCPHost{
				IP:   "192.168.122.45",
				MAC:  "00:11:22:33:44:55",
				Name: "test-hostname",
			},
			network: "default",
		},
		{
			desc: "unknown network",
			reservation: &libvirtxml.NetworkDHCPHost{
				IP:   "192.168.122.45",
				MAC:  "00:11:22:33:44:55",
				Name: "test-hostname",
			},
			network: "unknown",
			err:     "failed to lookup network",
		},
		{
			desc: "invalid reservation MAC",
			reservation: &libvirtxml.NetworkDHCPHost{
				IP:   "192.168.122.45",
				Name: "test-hostname",
			},
			network: "default",
			err:     "failed to parse lease MAC",
		},
		{
			desc:           "invalid reservation",
			reservationRaw: "-",
			network:        "default",
			err:            "failed to parse DHCP reservation",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			var err error
			reservation := tc.reservationRaw
			if tc.reservation != nil {
				reservation, err = tc.reservation.Marshal()
				must.NoError(t, err)
			}

			err = mockController.releaseDHCPLease(tc.network, reservation)
			if tc.err != "" {
				must.ErrorContains(t, err, tc.err)
			} else {
				must.NoError(t, err)

			}
		})
	}
}

// iptablesCleanup can be used as a cleanup function which will remove all the
// added iptables chain and rule entries. This avoids polluting the machine
// that runs the test, so our development machines do not require manual
// intervention after each test run.
//
// Any errors running the cleanup commands are logged, so developers can
// spot these and perform manual fixes. Manual fixes:
//   - sudo iptables -t filter -D FORWARD -j NOMAD_VT_FW
//   - sudo iptables -F NOMAD_VT_FW -t filter
//   - sudo iptables -X NOMAD_VT_FW -t filter
//   - sudo iptables -t nat -D PREROUTING -j NOMAD_VT_PRT
//   - sudo iptables -F NOMAD_VT_PRT -t nat
//   - sudo iptables -X NOMAD_VT_PRT -t nat
func iptablesCleanup(t *testing.T, ipt *iptables.IPTables) {
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
}

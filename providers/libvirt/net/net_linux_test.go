// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package net

import (
	stdnet "net"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	filter_mock "github.com/hashicorp/nomad-driver-virt/testutil/mock/net/filter"
	libvirt_mock "github.com/hashicorp/nomad-driver-virt/testutil/mock/providers/libvirt"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	nomadstructs "github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/shared/structs"
	"github.com/shoenig/test/must"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

func TestController_Fingerprint(t *testing.T) {
	// Use a populated mock shim to test that we query and correctly populate
	// the passed attributes.
	controller := NewController(hclog.NewNullLogger(), &libvirt_mock.StaticConnect{})

	controllerAttrs := map[string]*structs.Attribute{}
	controller.Fingerprint(controllerAttrs)

	expectedOutput := map[string]*structs.Attribute{
		"driver.virt.network.default.state":       structs.NewStringAttribute("active"),
		"driver.virt.network.default.bridge_name": structs.NewStringAttribute("virbr0"),
		"driver.virt.network.routed.state":        structs.NewStringAttribute("inactive"),
		"driver.virt.network.routed.bridge_name":  structs.NewStringAttribute("br0"),
	}
	must.Eq(t, expectedOutput, controllerAttrs)

	// Set the shim to our empty mock, to ensure we do not panic or have any
	// other undesired outcome when the process does not find any networks
	// available on the host.
	emptyController := NewController(hclog.NewNullLogger(), &libvirt_mock.ConnectEmpty{})

	emptyControllerAttrs := map[string]*structs.Attribute{}
	emptyController.Fingerprint(emptyControllerAttrs)
	must.Eq(t, map[string]*structs.Attribute{}, emptyControllerAttrs)
}

func TestController_VMStartedBuild(t *testing.T) {
	t.Run("ok", func(t *testing.T) {

	})
	// define a mock network
	mockedNet := &libvirt_mock.StaticNetwork{
		Name:       "default",
		Active:     true,
		BridgeName: "virbr0",
		DhcpLeases: []libvirt.NetworkDHCPLease{
			{
				Iface:      "virbr0",
				ExpiryTime: time.Now().Add(1 * time.Hour),
				Type:       libvirt.IP_ADDR_TYPE_IPV4,
				Mac:        "52:54:00:1c:7c:14",
				IPaddr:     "192.168.122.58",
				Hostname:   "nomad-0ea818bc",
				Clientid:   "ff:08:24:45:0e:00:02:00:00:ab:11:35:ab:f3:c7:ac:54:9e:c8",
			},
		},
	}

	mockFilter := filter_mock.NewMock(t).Expect(
		filter_mock.Configure{
			Resources: &drivers.Resources{Ports: &nomadstructs.AllocatedPorts{
				{
					Label:  "ssh",
					To:     22,
					HostIP: "10.0.1.161",
					Value:  27494,
				},
				{
					Label:  "nomad",
					To:     4646,
					HostIP: "10.0.1.161",
					Value:  27512,
				},
			}},
			NetworkConfig: &net.NetworkInterfaceBridgeConfig{
				Name:  "virbr0",
				Ports: []string{"ssh", "nomad"},
			},
			IP: "192.168.122.58",
		},
	)
	defer mockFilter.AssertExpectations()

	// define a mock connect instance to provide network information
	mockConnect := libvirt_mock.NewConnect(t).Expect(
		libvirt_mock.ListNetworks{Result: []string{"default", "routed"}},
		libvirt_mock.LookupNetworkByName{Name: "default", Result: mockedNet},
		libvirt_mock.LookupNetworkByName{Name: "default", Result: mockedNet},
	)
	defer mockConnect.AssertExpectations()

	controller := &Controller{
		dhcpLeaseDiscoveryInterval: 100 * time.Millisecond,
		dhcpLeaseDiscoveryTimeout:  500 * time.Millisecond,
		logger:                     hclog.NewNullLogger(),
		netConn:                    mockConnect,
		filter:                     mockFilter,
	}

	must.NoError(t, controller.Init())

	// Ensure passing a nil request object doesn't cause the function to panic.
	nilRequestResp, err := controller.VMStartedBuild(nil)
	must.ErrorContains(t, err, "no request provided")
	must.Nil(t, nilRequestResp)

	// Ensure passing an empty request object doesn't cause the function to
	// panic.
	nilRequestResp, err = controller.VMStartedBuild(&net.VMStartedBuildRequest{})
	must.NoError(t, err)
	must.NotNil(t, nilRequestResp)
	must.Nil(t, nilRequestResp.TeardownSpec)

	// Pass a request that doesn't contain any configured networks to ensure we
	// correctly handle that.
	emptyNetworkRequestResp, err := controller.VMStartedBuild(&net.VMStartedBuildRequest{
		NetConfig: net.NetworkInterfacesConfig{},
		Resources: &drivers.Resources{},
	})
	must.NoError(t, err)
	must.NotNil(t, emptyNetworkRequestResp)
	must.Nil(t, emptyNetworkRequestResp.TeardownSpec)

	// Test a correct and full request.
	fullReq := net.VMStartedBuildRequest{
		VMName:   "nomad-0ea818bc",
		Hostname: "nomad-0ea818bc",
		Hwaddrs:  []string{"52:54:00:1c:7c:14"},
		NetConfig: net.NetworkInterfacesConfig{
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

	fullReqResp, err := controller.VMStartedBuild(&fullReq)
	must.NoError(t, err)
	must.NotNil(t, fullReqResp)
	must.NotNil(t, fullReqResp.DriverNetwork)
	must.NotNil(t, fullReqResp.TeardownSpec)

	must.Eq(t, &drivers.DriverNetwork{IP: "192.168.122.58"}, fullReqResp.DriverNetwork)
}

func TestController_VMTerminatedTeardown(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		controller := &Controller{
			logger:  hclog.NewNullLogger(),
			netConn: &libvirt_mock.StaticConnect{},
			filter:  filter_mock.NewStatic(),
		}

		resp, err := controller.VMTerminatedTeardown(nil)
		must.NoError(t, err)
		must.Eq(t, &net.VMTerminatedTeardownResponse{}, resp)

		resp, err = controller.VMTerminatedTeardown(&net.VMTerminatedTeardownRequest{})
		must.NoError(t, err)
		must.Eq(t, &net.VMTerminatedTeardownResponse{}, resp)
	})

	t.Run("ok", func(t *testing.T) {
		req := &net.VMTerminatedTeardownRequest{
			TeardownSpec: &net.TeardownSpec{
				FilterRemoval: &net.FilterRemoval{
					Name: "testing",
					Data: "test-data",
				},
			},
		}

		mockFilter := filter_mock.NewMock(t).Expect(
			filter_mock.Teardown{
				Removal: &net.FilterRemoval{
					Name: "testing",
					Data: "test-data",
				},
			},
		)
		defer mockFilter.AssertExpectations()

		controller := &Controller{
			logger:  hclog.NewNullLogger(),
			netConn: &libvirt_mock.StaticConnect{},
			filter:  mockFilter,
		}

		resp, err := controller.VMTerminatedTeardown(req)
		must.NoError(t, err)
		must.Eq(t, &net.VMTerminatedTeardownResponse{}, resp)
	})
}

func TestController_networkNameFromBridgeName(t *testing.T) {
	// Create out controller which has a mocked connection with identified
	// networks.
	controller := &Controller{
		logger:  hclog.NewNullLogger(),
		netConn: &libvirt_mock.StaticConnect{},
	}

	// Query a non-existent network.
	nonExistentResp, err := controller.networkNameFromBridgeName("non-existent-bridge")
	must.ErrorContains(t, err, "failed to find network with bridge")
	must.Eq(t, nonExistentResp, "")

	// Query a network which does exist.
	virbr0Resp, err := controller.networkNameFromBridgeName("virbr0")
	must.NoError(t, err)
	must.Eq(t, virbr0Resp, "default")

	// Create a controller with a connection that does not have any identified
	// networks. This allows us to ensure the behaviour is the same on hosts
	// which have no networks, as one that do.
	mockEmptyController := &Controller{
		logger:  hclog.NewNullLogger(),
		netConn: &libvirt_mock.ConnectEmpty{},
	}

	mockEmptyResp, err := mockEmptyController.networkNameFromBridgeName("virbr0")
	must.ErrorContains(t, err, "failed to find network with bridge")
	must.Eq(t, mockEmptyResp, "")
}

func TestController_discoverDHCPLeaseIP(t *testing.T) {
	// Create out controller which has a mocked connection with identified
	// networks and low discovery time durations, so the tests do not take ages
	// to run.
	controller := &Controller{
		logger:                     hclog.NewNullLogger(),
		netConn:                    &libvirt_mock.StaticConnect{},
		dhcpLeaseDiscoveryInterval: 1 * time.Nanosecond,
		dhcpLeaseDiscoveryTimeout:  100 * time.Microsecond,
	}

	defaultNet, err := controller.netConn.LookupNetworkByName("default")
	must.NoError(t, err)
	must.NotNil(t, defaultNet)
	defer defaultNet.Free()

	// Query for a domain that does not have a lease entry and ensure the
	// timeout is triggered.
	nonExistentResp, mac, err := controller.discoverDHCPLeaseIP(defaultNet, "non-existent-domain",
		"default", []string{"00:00:00:00:00:00"})
	must.ErrorContains(t, err, "timeout reached discovering DHCP lease")
	must.Eq(t, nonExistentResp, "")
	must.Eq(t, mac, "")

	// Query for a domain which does have a lease.
	existentResp, mac, err := controller.discoverDHCPLeaseIP(defaultNet, "nomad-0ea818bc",
		"default", []string{"52:54:00:1c:7c:14"})
	must.NoError(t, err)
	must.Eq(t, existentResp, "192.168.122.58")
	must.Eq(t, mac, "52:54:00:1c:7c:14")

	// Query for a domain which does have a lease using multiple MAC addresses.
	existentResp, mac, err = controller.discoverDHCPLeaseIP(defaultNet, "nomad-0ea818bc",
		"default", []string{"11:11:11:11:11:11", "52:54:00:1c:7c:14", "22:22:22:22:22:22"})
	must.NoError(t, err)
	must.Eq(t, existentResp, "192.168.122.58")
	must.Eq(t, mac, "52:54:00:1c:7c:14")

	// Query for a domain with several matching leases.
	multiResp, mac, err := controller.discoverDHCPLeaseIP(defaultNet, "nomad-3edc43aa",
		"default", []string{"11:22:33:44:55:66"})
	must.NoError(t, err)
	must.Eq(t, multiResp, "192.168.122.65")
	must.Eq(t, mac, "11:22:33:44:55:66")

	// Query for domain with matching expired lease.
	expiredResp, mac, err := controller.discoverDHCPLeaseIP(defaultNet, "nomad-eabba892",
		"default", []string{"66:55:44:33:22:11"})
	must.ErrorContains(t, err, "timeout reached discovering DHCP lease")
	must.Eq(t, expiredResp, "")
	must.Eq(t, mac, "")

	// Query for domain with matching MAC address only.
	macOnlyResp, mac, err := controller.discoverDHCPLeaseIP(defaultNet, "different-hostname",
		"default", []string{"52:54:00:1c:7c:14"})
	must.ErrorContains(t, err, "timeout reached discovering DHCP lease")
	must.Eq(t, macOnlyResp, "")
	must.Eq(t, mac, "")

	// Query for domain with matching MAC address and empty hostname on lease.
	macOnlyNoHostnameResp, mac, err := controller.discoverDHCPLeaseIP(defaultNet, "custom-hostname",
		"default", []string{"11:22:11:22:11:22"})
	must.NoError(t, err)
	must.Eq(t, macOnlyNoHostnameResp, "192.168.122.99")
	must.Eq(t, mac, "11:22:11:22:11:22")
}

func TestController_removeIPReservation(t *testing.T) {
	controller := &Controller{
		logger:  hclog.NewNullLogger(),
		netConn: &libvirt_mock.StaticConnect{},
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

		err = controller.removeIPReservation(tc.network, entry)
		if tc.err != "" {
			must.ErrorContains(t, err, tc.err)
		} else {
			must.NoError(t, err)
		}
	}
}

func TestController_ipReservationExists(t *testing.T) {
	controller := &Controller{
		logger:  hclog.NewNullLogger(),
		netConn: &libvirt_mock.StaticConnect{},
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

	network, err := controller.netConn.LookupNetworkByName("default")
	must.NoError(t, err)
	defer network.Free()

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			entry := tc.reservationRaw
			if tc.reservation != nil {
				entry, err = tc.reservation.Marshal()
				must.NoError(t, err)
			}

			exists, err := controller.ipReservationExists(network, entry)
			must.Eq(t, tc.exists, exists)
			if tc.err != "" {
				must.ErrorContains(t, err, tc.err)
			} else {
				must.NoError(t, err)
			}
		})
	}
}

func TestController_releaseDHCPLease(t *testing.T) {
	controller := &Controller{
		logger:              hclog.NewNullLogger(),
		netConn:             &libvirt_mock.StaticConnect{},
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

			err = controller.releaseDHCPLease(tc.network, reservation)
			if tc.err != "" {
				must.ErrorContains(t, err, tc.err)
			} else {
				must.NoError(t, err)

			}
		})
	}
}

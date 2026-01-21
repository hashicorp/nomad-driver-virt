// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package net

import (
	"github.com/hashicorp/nomad/plugins/drivers"
)

const (
	// FingerprintAttributeKeyPrefix is the key prefix to use when creating and
	// adding attributes during the fingerprint process.
	FingerprintAttributeKeyPrefix = "driver.virt.network."

	// NetworkStateActive is string representation to declare a network is in
	// active state. This is translated from "true" using the go-libvirt SDK
	// and 1 from the raw libvirt API when query if the network is active.
	NetworkStateActive = "active"

	// NetworkStateInactive is string representation to declare a network is in
	// inactive state. This is translated from "false" using the go-libvirt SDK
	// and 0 from the raw libvirt API when query if the network is active.
	NetworkStateInactive = "inactive"
)

// VMStartedBuildRequest is the request object used to ask the network
// sub-system to perform its configuration, once a VM has been started.
type VMStartedBuildRequest struct {
	VMName    string
	Hostname  string
	NetConfig *NetworkInterfacesConfig
	Resources *drivers.Resources
	Hwaddrs   []string
}

// VMStartedBuildResponse is the response sent object once the network
// sub-system has performed its configuration for a running VM.
type VMStartedBuildResponse struct {

	// DriverNetwork is the object returned to Nomad once the task is started
	// and is used to populate service discovery. The network sub-system should
	// fill in all details; the driver will not do this and simply pass the
	// object straight onto Nomad.
	DriverNetwork *drivers.DriverNetwork

	// TeardownSpec contains a specification which will be stored in the task
	// handle and used when stopping/killing the task. It should include
	// information which either expedites the process or is critical to the
	// process.
	TeardownSpec *TeardownSpec
}

// VMTerminatedTeardownRequest is the request object used to ask the network
// sub-system to perform its teardown of a VMs network configuration.
type VMTerminatedTeardownRequest struct {
	TeardownSpec *TeardownSpec
}

// VMTerminatedTeardownResponse is the response object returned when the
// network sub-system has performed its teardown of a VMs network
// configuration.
type VMTerminatedTeardownResponse struct{}

// TeardownSpec contains a specification which will be stored in the task
// handle and used when stopping/killing the task. It should include
// information which either expedites the process or is critical to the
// process.
type TeardownSpec struct {

	// IPTablesRules specifies the rules used to build the initial VM
	// networking. Each entry is a rule, the rule is a list of strings which
	// mimics how iptables is called.
	//   i[0] is the table name.
	//   i[1] is the chain name.
	//   i[2:] is the rule args.
	IPTablesRules [][]string

	// DHCPReservation specifies the reservation string used for registering
	// a DHCP address for a virtual machine.
	DHCPReservation string

	// Network is the name of the network used and which provided the
	// DHCP lease.
	Network string
}

// IsActiveString converts the boolean response from the IsActive call of
// libvirt network to a human-readable string. This string copies the
// vocabulary used by virsh for consistency.
func IsActiveString(active bool) string {
	if active {
		return NetworkStateActive
	}
	return NetworkStateInactive
}

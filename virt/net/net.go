// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package net

import (
	"slices"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
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
	NetConfig NetworkInterfacesConfig
	Resources *drivers.Resources
	Hwaddrs   []string
}

// Equal returns if the given VMStartBuildRequest is equal.
// NOTE: ignores Resources value
func (v *VMStartedBuildRequest) Equal(rhs *VMStartedBuildRequest) bool {
	if v == nil || rhs == nil {
		return false
	}

	if v.Hostname != rhs.Hostname {
		return false
	}

	if slices.Compare(v.Hwaddrs, rhs.Hwaddrs) != 0 {
		return false
	}

	if !v.NetConfig.Equal(rhs.NetConfig) {
		return false
	}

	return true
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

// Equal returns if the given VMTerminatedTeardownRequest is equal.
func (v *VMTerminatedTeardownRequest) Equal(rhs *VMTerminatedTeardownRequest) bool {
	if v == nil || rhs == nil {
		return false
	}

	if !v.TeardownSpec.Equal(rhs.TeardownSpec) {
		return false
	}

	return true
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

	// FilterRemoval contains the information to remove packet filtering
	// configuration for the virtual machine.
	FilterRemoval *FilterRemoval

	// DHCPReservation specifies the reservation string used for registering
	// a DHCP address for a virtual machine.
	DHCPReservation string

	// Network is the name of the network used and which provided the
	// DHCP lease.
	Network string
}

// FilterRemoval contains the information required to remove any configuration
// applied to the packet filter for routing to/from the task Virtual Machine.
type FilterRemoval struct {
	// Name is the name of the package that created the
	// FilterRemoval information.
	Name string

	// Data is an encodable type that provides the required information
	// to remove any packet filtering configuration related to the task.
	// The actual type will be dependent on the underlying package in use.
	Data any
}

// Equal returns if the given TeardownSpec is equal.
func (t *TeardownSpec) Equal(rhs *TeardownSpec) bool {
	if t == nil || rhs == nil {
		return t == rhs
	}

	if t.DHCPReservation != rhs.DHCPReservation {
		return false
	}

	if t.Network != rhs.Network {
		return false
	}

	if !cmp.Equal(t.FilterRemoval, rhs.FilterRemoval, cmp.Options{cmpopts.IgnoreUnexported()}) {
		return false
	}

	return true
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

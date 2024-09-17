// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package net

import (
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

// Net is the interface that defines the virtualization network sub-system. It
// should be the only link from the main driver and 	compute functionality, into
// the network. This helps encapsulate the logic making future development
// easier, even allowing for this code to be moved into its own application if
// desired.
type Net interface {

	// Fingerprint interrogates the host system and populates the attribute
	// mapping with relevant network information. Any errors performing this
	// should be logged by the implementor, but not considered terminal, which
	// explains the lack of error response. Each entry should use
	// FingerprintAttributeKeyPrefix as a base.
	Fingerprint(map[string]*structs.Attribute)

	// Init performs any initialization work needed by the network sub-system
	// prior to being used by the driver. This will be called when the plugin
	// is set up by Nomad and should be expected to run multiple times during
	// a Nomad client's lifecycle. It should therefore be idempotent. Any error
	// returned is considered fatal to the plugin.
	Init() error

	// VMStartedBuild performs any network configuration required once the
	// driver has successfully started a VM. Any error returned will be
	// considered terminal to the start of the VM and therefore halt any
	// further progress and result in the task being restarted.
	VMStartedBuild(*VMStartedBuildRequest) (*VMStartedBuildResponse, error)

	// VMTerminatedTeardown performs all the network teardown required to clean
	// the host and any systems of configuration specific to the task. If an
	// error is encountered, Nomad will retry the stop/kill process, so all
	// implementations must be able to support this and not enter death spirals
	// when an error occurs.
	VMTerminatedTeardown(*VMTerminatedTeardownRequest) (*VMTerminatedTeardownResponse, error)
}

// VMStartedBuildRequest is the request object used to ask the network
// sub-system to perform its configuration, once a VM has been started.
type VMStartedBuildRequest struct {
	DomainName string
	NetConfig  *NetworkInterfacesConfig
	Resources  *drivers.Resources
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
}

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

// IsActiveString converts the boolean response from the IsActive call of
// libvirt network to a human-readable string. This string copies the
// vocabulary used by virsh for consistency.
func IsActiveString(active bool) string {
	if active {
		return NetworkStateActive
	}
	return NetworkStateInactive
}

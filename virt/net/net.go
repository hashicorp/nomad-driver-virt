// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package net

import (
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

// Net is the interface that defines the virtualization network sub-system. It
// should be the only link from the main driver and compute functionality, into
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

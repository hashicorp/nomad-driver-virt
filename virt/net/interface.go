// Copyright IBM Corp. 2024, 2025
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

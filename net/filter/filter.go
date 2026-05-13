// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package filter

import (
	virtnet "github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/drivers"
)

// Filter is the interface to add and remove packet filtering
// configuration for virt tasks.
type Filter interface {
	Configure(*drivers.Resources, *virtnet.NetworkInterfaceBridgeConfig, string) (*virtnet.FilterRemoval, error)
	Teardown(*virtnet.FilterRemoval) error
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package net

import (
	"github.com/hashicorp/nomad-driver-virt/virt/net"
)

var (
	_ net.Net = (*MockNet)(nil)
	_ net.Net = (*StaticNet)(nil)
)

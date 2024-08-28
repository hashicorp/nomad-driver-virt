// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"libvirt.org/go/libvirt"
)

var (
	_ ConnectShim        = &Connect{}
	_ ConnectNetworkShim = &libvirt.Network{}
)

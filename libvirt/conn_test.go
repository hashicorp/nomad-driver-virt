// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"github.com/hashicorp/nomad-driver-virt/virt/shim"
	"libvirt.org/go/libvirt"
)

var (
	_ shim.Connect = &Connect{}
	_ shim.Network = &libvirt.Network{}
)

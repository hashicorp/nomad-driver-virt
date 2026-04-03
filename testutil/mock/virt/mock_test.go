// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package virt

import "github.com/hashicorp/nomad-driver-virt/virt"

var (
	_ virt.Virtualizer = (*MockVirt)(nil)
	_ virt.Virtualizer = (*StaticVirt)(nil)
)

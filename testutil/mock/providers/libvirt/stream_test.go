// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package libvirt

import "github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"

var (
	_ shims.Stream = (*StaticStream)(nil)
	_ shims.Stream = (*MockStream)(nil)
)

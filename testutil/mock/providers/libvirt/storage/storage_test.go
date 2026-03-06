// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
)

var (
	_ shims.StoragePool = (*StaticStoragePool)(nil)
	_ shims.StoragePool = (*MockStoragePool)(nil)
	_ shims.StorageVol  = (*StaticStorageVol)(nil)
	_ shims.StorageVol  = (*MockStorageVol)(nil)
)

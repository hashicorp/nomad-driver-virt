// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"github.com/hashicorp/nomad-driver-virt/storage"
)

var (
	_ storage.Storage = (*StaticStorage)(nil)
	_ storage.Storage = (*MockStorage)(nil)
	_ storage.Pool    = (*StaticPool)(nil)
	_ storage.Pool    = (*MockPool)(nil)
)

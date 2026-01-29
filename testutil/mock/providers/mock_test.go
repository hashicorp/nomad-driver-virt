// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package providers

import (
	"github.com/hashicorp/nomad-driver-virt/providers"
)

var (
	_ providers.Providers = (*StaticProviders)(nil)
	_ providers.Providers = (*MockProviders)(nil)
)

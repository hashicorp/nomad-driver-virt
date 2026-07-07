// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package nftables

import (
	"github.com/hashicorp/nomad-driver-virt/net/filter/linux/nftables"
)

var _ nftables.NFTables = (*MockNFTables)(nil)

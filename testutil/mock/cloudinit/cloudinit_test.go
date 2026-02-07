// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package cloudinit

import (
	"github.com/hashicorp/nomad-driver-virt/cloudinit"
)

var (
	_ cloudinit.CloudInit = (*MockCloudInit)(nil)
	_ cloudinit.CloudInit = (*StaticCloudInit)(nil)
)

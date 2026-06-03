// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package image_tools

import "github.com/hashicorp/nomad-driver-virt/storage/image_tools"

var (
	_ image_tools.ImageHandler = (*MockImageHandler)(nil)
	_ image_tools.ImageHandler = (*StaticImageHandler)(nil)
)

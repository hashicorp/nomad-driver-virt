// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

// ConfigSpec defines the HCL for the configuration.
var ConfigSpec = map[string]*hclspec.Spec{
	"libvirt": hclspec.NewBlock("libvirt", false, hclspec.NewObject(map[string]*hclspec.Spec{
		"uri": hclspec.NewDefault(
			hclspec.NewAttr("uri", "string", false),
			hclspec.NewLiteral(`"qemu:///system"`),
		),
		"user":     hclspec.NewAttr("user", "string", false),
		"password": hclspec.NewAttr("password", "string", false),
	})),
}

// Configuration supported by this provider.
type Config struct {
	URI      string `codec:"uri"`
	User     string `codec:"user"`
	Password string `codec:"password"`
}

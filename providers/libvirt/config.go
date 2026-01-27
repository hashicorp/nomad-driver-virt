// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

// configSpec defines the HCL for the configuration.
var configSpec = hclspec.NewObject(map[string]*hclspec.Spec{
	"libvirt": hclspec.NewBlock("libvirt", false, hclspec.NewObject(map[string]*hclspec.Spec{
		"uri": hclspec.NewDefault(
			hclspec.NewAttr("uri", "string", false),
			hclspec.NewLiteral(`"qemu:///system"`),
		),
		"user":     hclspec.NewAttr("user", "string", false),
		"password": hclspec.NewAttr("password", "string", false),
		"default":  hclspec.NewAttr("default", "bool", false),
	})),
})

// ConfigSpec returns the HCL spec for the libvirt provider configuration.
func ConfigSpec() *hclspec.Spec {
	return configSpec
}

// Configuration supported by this provider.
type Config struct {
	URI      string `codec:"uri"`
	User     string `codec:"user"`
	Password string `codec:"password"`
	Default  bool   `codec:"default"`
}

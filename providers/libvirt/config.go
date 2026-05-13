// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"fmt"

	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

// configSpec defines the HCL for the configuration.
var configSpec = hclspec.NewBlock("libvirt", false, hclspec.NewObject(map[string]*hclspec.Spec{
	"uri": hclspec.NewDefault(
		hclspec.NewAttr("uri", "string", false),
		hclspec.NewLiteral(fmt.Sprintf("%q", defaultURI)),
	),
	"user":                           hclspec.NewAttr("user", "string", false),
	"password":                       hclspec.NewAttr("password", "string", false),
	"allow_insecure_readonly_mounts": hclspec.NewAttr("allow_insecure_readonly_mounts", "bool", false),
}))

// ConfigSpec returns the HCL spec for the libvirt provider configuration.
func ConfigSpec() *hclspec.Spec {
	return configSpec
}

// Configuration supported by this provider.
type Config struct {
	URI                 string `codec:"uri"`
	User                string `codec:"user"`
	Password            string `codec:"password"`
	AllowInsecureMounts bool   `codec:"allow_insecure_readonly_mounts"`
}

// Validate validates the libvirt configuration.
func (c *Config) Validate() error {
	// NOTE: Nothing to validate currently.
	return nil
}

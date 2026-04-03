// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import "github.com/hashicorp/nomad/plugins/shared/hclspec"

var configSpec = hclspec.NewObject(map[string]*hclspec.Spec{
	"default": hclspec.NewAttr("default", "string", false),
	"directory": hclspec.NewBlockMap("directory", []string{"name"}, hclspec.NewObject(map[string]*hclspec.Spec{
		"path": hclspec.NewAttr("path", "string", true),
	})),
	"ceph": hclspec.NewBlockMap("ceph", []string{"name"}, hclspec.NewObject(map[string]*hclspec.Spec{
		"pool":  hclspec.NewAttr("pool", "string", true),
		"hosts": hclspec.NewAttr("hosts", "list(string)", true),
		"authentication": hclspec.NewBlock("authentication", true, hclspec.NewObject(map[string]*hclspec.Spec{
			"username": hclspec.NewAttr("username", "string", true),
			"secret":   hclspec.NewAttr("secret", "string", true),
		})),
	})),
})

// Config provides configuration for storage pools
type Config struct {
	// Default is the name of the storage pool that is the default
	Default string `codec:"default"`
	// Directory provides directory storage pool configuration
	Directory map[string]Directory `codec:"directory"`
	// Ceph provides ceph storage pool configuration
	Ceph map[string]Ceph `codec:"ceph"`
}

// Directory provides configuration for local directory storage pools
type Directory struct {
	Path string `codec:"path"` // Local path of the storage pool
}

// Ceph provides configuration for ceph rbd storage pools
type Ceph struct {
	Pool           string         `codec:"pool"`           // Name of the ceph storage pool
	Hosts          []string       `codec:"hosts"`          // List of ceph hosts
	Authentication Authentication `codec:"authentication"` // Autentication for ceph connection
}

// Authentication provides credentials
type Authentication struct {
	Username string `codec:"username"`
	Secret   string `codec:"secret"`
}

// NewConfig returns a new initialized config.
func NewConfig() *Config {
	return &Config{
		Directory: make(map[string]Directory),
		Ceph:      make(map[string]Ceph),
	}
}

// ConfigSpec returns the hcl spec for the storage pools configuration
func ConfigSpec() *hclspec.Spec {
	return configSpec
}

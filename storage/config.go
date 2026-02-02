// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import "github.com/hashicorp/nomad/plugins/shared/hclspec"

var configSpec = hclspec.NewObject(map[string]*hclspec.Spec{
	"directory": hclspec.NewBlockList("directory", hclspec.NewObject(map[string]*hclspec.Spec{
		"name":    hclspec.NewAttr("name", "string", true),
		"path":    hclspec.NewAttr("path", "string", true),
		"default": hclspec.NewAttr("default", "bool", false),
	})),
	"ceph": hclspec.NewBlockList("ceph", hclspec.NewObject(map[string]*hclspec.Spec{
		"name":  hclspec.NewAttr("name", "string", true),
		"pool":  hclspec.NewAttr("pool", "string", true),
		"hosts": hclspec.NewAttr("hosts", "list(string)", true),
		"authentication": hclspec.NewBlock("authentication", true, hclspec.NewObject(map[string]*hclspec.Spec{
			"username": hclspec.NewAttr("username", "string", true),
			"secret":   hclspec.NewAttr("secret", "string", true),
		})),
		"default": hclspec.NewAttr("default", "bool", false),
	})),
})

// Config provides configuration for storage pools
type Config struct {
	// Directory provides directory storage pool configuration
	Directory []Directory `codec:"directory"`
	// Ceph provides ceph storage pool configuration
	Ceph []Ceph `codec:"ceph"`
}

// Directory provides configuration for local directory storage pools
type Directory struct {
	Name    string `codec:"name"`    // Name of the storage pool
	Path    string `codec:"path"`    // Local path of the storage pool
	Default bool   `codec:"default"` // Pool is the default storage pool
}

// Ceph provides configuration for ceph rbd storage pools
type Ceph struct {
	Name           string         `codec:"name"`           // Name of the storage pool
	Pool           string         `codec:"pool"`           // Name of the ceph storage pool
	Hosts          []string       `codec:"hosts"`          // List of ceph hosts
	Authentication Authentication `codec:"authentication"` // Autentication for ceph connection
	Default        bool           `codec:"default"`        // Pool is the default storage pool
}

// Authentication provides credentials
type Authentication struct {
	Username string `codec:"username"`
	Secret   string `codec:"secret"`
}

// ConfigSpec returns the hcl spec for the storage pools configuration
func ConfigSpec() *hclspec.Spec {
	return configSpec
}

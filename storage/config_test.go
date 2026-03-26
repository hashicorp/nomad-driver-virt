// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"testing"

	"github.com/hashicorp/nomad/helper/pluginutils/hclutils"
	"github.com/shoenig/test/must"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		expected := &Config{
			Directory: map[string]Directory{
				"dir-pool": {
					Path: "/dev/null",
				},
			},
			Ceph: map[string]Ceph{
				"ceph-pool": {
					Pool:  "test-pool",
					Hosts: []string{"localhost"},
					Authentication: Authentication{
						Username: "test-user",
						Secret:   "test-secret",
					},
				},
			},
			Default: "ceph-pool",
		}
		parser := hclutils.NewConfigParser(configSpec)
		validHcl := `
config {
  directory "dir-pool" {
    path = "/dev/null"
  }

  ceph "ceph-pool" {
    pool = "test-pool"
    hosts = ["localhost"]
    authentication {
      username = "test-user"
      secret = "test-secret"
    }
  }

  default = "ceph-pool"
}
`
		var config *Config
		parser.ParseHCL(t, validHcl, &config)
		must.Eq(t, expected, config)
	})

	t.Run("valid multiples", func(t *testing.T) {
		expected := &Config{
			Directory: map[string]Directory{
				"dir-pool": {
					Path: "/dev/null",
				},
				"other-dir-pool": {
					Path: "/dev/null/other",
				},
			},
			Ceph: map[string]Ceph{
				"ceph-pool": {
					Pool:  "test-pool",
					Hosts: []string{"localhost"},
					Authentication: Authentication{
						Username: "test-user",
						Secret:   "test-secret",
					},
				},
			},
			Default: "ceph-pool",
		}
		parser := hclutils.NewConfigParser(configSpec)
		validHcl := `
config {
  directory "dir-pool" {
    path = "/dev/null"
  }

  directory "other-dir-pool" {
    path = "/dev/null/other"
  } 

  ceph "ceph-pool" {
    pool = "test-pool"
    hosts = ["localhost"]
    authentication {
      username = "test-user"
      secret = "test-secret"
    }
  }

  default = "ceph-pool"
}
`
		var config *Config
		parser.ParseHCL(t, validHcl, &config)
		must.Eq(t, expected, config)
	})
}

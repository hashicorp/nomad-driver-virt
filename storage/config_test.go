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
					Path:    "/dev/null",
					Default: false,
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
					Default: true,
				},
			},
		}
		parser := hclutils.NewConfigParser(configSpec)
		validHcl := `
config {
  directory "dir-pool" {
    path = "/dev/null"
    default = false
  }

  ceph "ceph-pool" {
    pool = "test-pool"
    hosts = ["localhost"]
    authentication {
      username = "test-user"
      secret = "test-secret"
    }
    default = true
  }
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
					Path:    "/dev/null",
					Default: false,
				},
				"other-dir-pool": {
					Path:    "/dev/null/other",
					Default: false,
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
					Default: true,
				},
			},
		}
		parser := hclutils.NewConfigParser(configSpec)
		validHcl := `
config {
  directory "dir-pool" {
    path = "/dev/null"
    default = false
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
    default = true
  }
}
`
		var config *Config
		parser.ParseHCL(t, validHcl, &config)
		must.Eq(t, expected, config)
	})
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"testing"

	"github.com/hashicorp/nomad-driver-virt/internal/errs"
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

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		desc        string
		config      *Config
		errContains string
		errType     error
	}{
		{
			desc: "ok",
			config: &Config{
				Directory: map[string]Directory{
					"pool": {Path: "/dev/null"},
				},
			},
		},
		{
			desc:        "error - no pools",
			config:      &Config{},
			errContains: "no storage pools",
			errType:     errs.ErrInvalidConfiguration,
		},
		{
			desc:        "error - bad default",
			config:      &Config{Default: "some-pool"},
			errContains: "default storage pool is unknown - some-pool",
			errType:     errs.ErrInvalidConfiguration,
		},
		{
			desc: "error - duplicate names",
			config: &Config{
				Directory: map[string]Directory{
					"test-pool": {},
				},
				Ceph: map[string]Ceph{
					"test-pool": {},
				},
			},
			errContains: "already defined - test-pool",
			errType:     errs.ErrInvalidConfiguration,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := tc.config.Validate()
			if tc.errContains == "" && tc.errType == nil {
				must.NoError(t, err)
			}

			if tc.errContains != "" {
				must.ErrorContains(t, err, tc.errContains)
			}

			if tc.errType != nil {
				must.ErrorIs(t, err, tc.errType)
			}
		})
	}
}

func TestDirectory_Validate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		desc        string
		config      Directory
		errContains string
		errType     error
	}{
		{
			desc:   "ok",
			config: Directory{Path: "/dev/null"},
		},
		{
			desc:        "error - no path",
			config:      Directory{},
			errContains: "storage_pool.directory.path",
			errType:     errs.ErrMissingAttribute,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := tc.config.Validate()
			if tc.errContains == "" && tc.errType == nil {
				must.NoError(t, err)
			}

			if tc.errContains != "" {
				must.ErrorContains(t, err, tc.errContains)
			}

			if tc.errType != nil {
				must.ErrorIs(t, err, tc.errType)
			}
		})
	}
}

func TestCeph_Validate(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		desc        string
		config      Ceph
		errContains string
		errType     error
	}{
		{
			desc: "ok",
			config: Ceph{
				Pool:  "test-pool",
				Hosts: []string{"locahost:3300"},
				Authentication: Authentication{
					Username: "test-user",
					Secret:   "test-key",
				},
			},
		},
		{
			desc: "error - missing pool",
			config: Ceph{
				Hosts: []string{"locahost:3300"},
				Authentication: Authentication{
					Username: "test-user",
					Secret:   "test-key",
				},
			},
			errContains: "storage_pool.ceph.pool",
			errType:     errs.ErrMissingAttribute,
		},
		{
			desc: "error - missing hosts",
			config: Ceph{
				Pool: "test-pool",
				Authentication: Authentication{
					Username: "test-user",
					Secret:   "test-key",
				},
			},
			errContains: "storage_pool.ceph.hosts",
			errType:     errs.ErrMissingAttribute,
		},
		{
			desc: "error - missing username",
			config: Ceph{
				Pool:  "test-pool",
				Hosts: []string{"locahost:3300"},
				Authentication: Authentication{
					Secret: "test-key",
				},
			},
			errContains: "storage_pool.ceph.authentication.username",
			errType:     errs.ErrMissingAttribute,
		},
		{
			desc: "error - missing secret",
			config: Ceph{
				Pool:  "test-pool",
				Hosts: []string{"locahost:3300"},
				Authentication: Authentication{
					Username: "test-user",
				},
			},
			errContains: "storage_pool.ceph.authentication.secret",
			errType:     errs.ErrMissingAttribute,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := tc.config.Validate()
			if tc.errContains == "" && tc.errType == nil {
				must.NoError(t, err)
			}

			if tc.errContains != "" {
				must.ErrorContains(t, err, tc.errContains)
			}

			if tc.errType != nil {
				must.ErrorIs(t, err, tc.errType)
			}
		})
	}
}

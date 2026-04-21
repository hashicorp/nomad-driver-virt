// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

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

// Validate validates the storage configuration.
func (c *Config) Validate() error {
	var mErr *multierror.Error

	// Track names to flag duplicates.
	names := map[string]struct{}{}

	// Validate the directory storage pools.
	for n, dir := range c.Directory {
		names[n] = struct{}{}

		if err := dir.Validate(); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	// Validate the ceph storage pools.
	for n, ceph := range c.Ceph {
		if _, ok := names[n]; ok {
			mErr = multierror.Append(mErr,
				fmt.Errorf("%w: storage pool name already defined - %s",
					errs.ErrInvalidConfiguration, n))
		}
		names[n] = struct{}{}

		if err := ceph.Validate(); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	// Must have at least one pool defined.
	if len(names) == 0 {
		mErr = multierror.Append(mErr,
			fmt.Errorf("%w: no storage pools defined", errs.ErrInvalidConfiguration))
	}

	// Check the default pool is defined if set.
	if c.Default != "" {
		if _, ok := names[c.Default]; !ok {
			mErr = multierror.Append(mErr,
				fmt.Errorf("%w: default storage pool is unknown - %s",
					errs.ErrInvalidConfiguration, c.Default))
		}
	}

	return mErr.ErrorOrNil()
}

// Directory provides configuration for local directory storage pools
type Directory struct {
	Path string `codec:"path"` // Local path of the storage pool
}

// Validate validates the directory pool configuration.
func (d Directory) Validate() error {
	var mErr *multierror.Error

	if d.Path == "" {
		mErr = multierror.Append(mErr,
			fmt.Errorf("%w: storage_pool.directory.path", errs.ErrMissingAttribute))
	}

	return mErr.ErrorOrNil()
}

// Ceph provides configuration for ceph rbd storage pools
type Ceph struct {
	Pool           string         `codec:"pool"`           // Name of the ceph storage pool
	Hosts          []string       `codec:"hosts"`          // List of ceph hosts
	Authentication Authentication `codec:"authentication"` // Autentication for ceph connection
}

// Validate validates the ceph pool configuration.
func (c Ceph) Validate() error {
	var mErr *multierror.Error

	if c.Pool == "" {
		mErr = multierror.Append(mErr,
			fmt.Errorf("%w: storage_pool.ceph.pool", errs.ErrMissingAttribute))
	}

	if len(c.Hosts) == 0 {
		mErr = multierror.Append(mErr,
			fmt.Errorf("%w: storage_pool.ceph.hosts", errs.ErrMissingAttribute))
	}

	if c.Authentication.Username == "" {
		mErr = multierror.Append(mErr,
			fmt.Errorf("%w: storage_pool.ceph.authentication.username", errs.ErrMissingAttribute))
	}

	if c.Authentication.Secret == "" {
		mErr = multierror.Append(mErr,
			fmt.Errorf("%w: storage_pool.ceph.authentication.secret", errs.ErrMissingAttribute))
	}

	return mErr.ErrorOrNil()
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

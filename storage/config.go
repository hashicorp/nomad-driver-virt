// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/go-set/v3"
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
	names := set.New[string](0)

	// Validate the directory storage pools.
	for n, dir := range c.Directory {
		names.Insert(n)
		mErr = multierror.Append(mErr, dir.Validate())
	}

	// Validate the ceph storage pools.
	for n, ceph := range c.Ceph {
		if !names.Insert(n) {
			mErr = multierror.Append(mErr,
				fmt.Errorf("%w: storage pool name already defined - %s",
					errs.ErrInvalidConfiguration, n))
		}
		mErr = multierror.Append(mErr, ceph.Validate())
	}

	// Must have at least one pool defined.
	if names.Empty() {
		mErr = multierror.Append(mErr,
			fmt.Errorf("%w: no storage pools defined", errs.ErrInvalidConfiguration))
	}

	// Check the default pool is defined if set.
	if c.Default != "" {
		if !names.Contains(c.Default) {
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

	mErr = multierror.Append(mErr,
		errs.MissingAttribute("storage_pool.directory.path", d.Path))

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

	mErr = multierror.Append(mErr,
		errs.MissingAttribute("storage_pool.ceph.pool", c.Pool),
		errs.MissingAttribute("storage_pool.ceph.hosts", c.Hosts),
		errs.MissingAttribute("storage_pool.ceph.authentication.username", c.Authentication.Username),
		errs.MissingAttribute("storage_pool.ceph.authentication.secret", c.Authentication.Secret),
	)

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

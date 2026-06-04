// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"github.com/hashicorp/nomad-driver-virt/virt/disks"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

var (
	configSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"provider": hclspec.NewBlock("provider", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"libvirt": libvirt.ConfigSpec(),
		})),
		"image_paths":   hclspec.NewAttr("image_paths", "list(string)", false),
		"storage_pools": hclspec.NewBlock("storage_pools", false, storage.ConfigSpec()),
	})

	// taskConfigSpec is the specification of the plugin's configuration for
	// a task
	// this is used to validated the configuration specified for the plugin
	// when a job is submitted.
	taskConfigSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"network_interface":               net.NetworkInterfaceHCLSpec(),
		"disk":                            disks.ConfigSpec(),
		"hostname":                        hclspec.NewAttr("hostname", "string", false),
		"user_data":                       hclspec.NewAttr("user_data", "string", false),
		"default_user_authorized_ssh_key": hclspec.NewAttr("default_user_authorized_ssh_key", "string", false),
		"default_user_password":           hclspec.NewAttr("default_user_password", "string", false),
		"cmds":                            hclspec.NewAttr("cmds", "list(string)", false),
		"timezone":                        hclspec.NewAttr("timezone", "string", false),
		"os": hclspec.NewBlock("os", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"arch":    hclspec.NewAttr("arch", "string", false),
			"machine": hclspec.NewAttr("machine", "string", false),
		})),
	})

	// validProviders is a list of valid provider names.
	validProviders = []string{
		libvirt.Name,
	}
)

func ConfigSpec() *hclspec.Spec {
	return configSpec
}

func TaskConfigSpec() *hclspec.Spec {
	return taskConfigSpec
}

// TaskConfig contains configuration information for a task that runs within
// this plugin.
type TaskConfig struct {
	Hostname            string      `codec:"hostname"`
	OS                  *OS         `codec:"os"`
	UserData            string      `codec:"user_data"`
	Timezone            string      `codec:"timezone"`
	CMDs                []string    `codec:"cmds"`
	DefaultUserSSHKey   string      `codec:"default_user_authorized_ssh_key"`
	DefaultUserPassword string      `codec:"default_user_password"`
	Disks               disks.Disks `codec:"disk"`
	// The list of network interfaces that should be added to the VM.
	net.NetworkInterfacesConfig `codec:"network_interface"`
}

type OS struct {
	Arch    string `codec:"arch"`
	Machine string `codec:"machine"`
}

// Config contains configuration information for the plugin
type Config struct {
	Provider     *Provider       `codec:"provider"`
	ImagePaths   []string        `codec:"image_paths"` // allow-list of host paths to load
	StoragePools *storage.Config `codec:"storage_pools"`
}

// Validate validates the configuration and sets default values.
func (c *Config) Validate() error {
	// If no provider configuration is set, default the libvirt provider.
	if c.Provider == nil {
		c.Provider = &Provider{Libvirt: &libvirt.Config{}}
	}

	var mErr *multierror.Error

	mErr = multierror.Append(mErr,
		c.Provider.Validate(),
		c.StoragePools.Validate(),
	)

	return mErr.ErrorOrNil()
}

// Provider contains provider specific configuration
type Provider struct {
	Default string          `codec:"default"`
	Libvirt *libvirt.Config `codec:"libvirt"`
}

// Validate validates the provider configuration.
func (p *Provider) Validate() error {
	var mErr *multierror.Error

	if p.Libvirt == nil {
		mErr = multierror.Append(mErr,
			fmt.Errorf("%w: no providers defined", errs.ErrInvalidConfiguration))
	}

	if p.Default != "" && slices.Contains(validProviders, p.Default) {
		mErr = multierror.Append(mErr,
			fmt.Errorf("%w: unknown default provider (supported: %s)",
				errs.ErrInvalidConfiguration, strings.Join(validProviders, ", ")))
	}

	if p.Libvirt != nil {
		if err := p.Libvirt.Validate(); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return mErr.ErrorOrNil()
}

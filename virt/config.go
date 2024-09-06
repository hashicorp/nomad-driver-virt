// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"time"

	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/drivers/fsisolation"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

var (
	configSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"emulator": hclspec.NewBlock("emulator", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"uri": hclspec.NewDefault(
				hclspec.NewAttr("uri", "string", false),
				hclspec.NewLiteral(`"qemu:///system"`),
			),
			"user":     hclspec.NewAttr("user", "string", false),
			"password": hclspec.NewAttr("password", "string", false),
		})),

		"data_dir": hclspec.NewDefault(
			hclspec.NewAttr("data_dir", "string", false),
			hclspec.NewLiteral(`"/opt/virt"`),
		),
		"image_paths": hclspec.NewAttr("image_paths", "list(string)", false),
	})

	// taskConfigSpec is the specification of the plugin's configuration for
	// a task
	// this is used to validated the configuration specified for the plugin
	// when a job is submitted.
	taskConfigSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"network_interface":               net.NetworkInterfaceHCLSpec(),
		"use_thin_copy":                   hclspec.NewAttr("use_thin_copy", "bool", false),
		"disk":                            hclspec.NewAttr("disk", "number", true),
		"image":                           hclspec.NewAttr("image", "string", true),
		"hostname":                        hclspec.NewAttr("hostname", "string", false),
		"user_data":                       hclspec.NewAttr("user_data", "string", false),
		"default_user_authorized_ssh_key": hclspec.NewAttr("default_user_authorized_ssh_key", "string", false),
		"default_user_password":           hclspec.NewAttr("default_user_password", "string", false),
		"cmds":                            hclspec.NewAttr("cmds", "list(string)", false),
		"os": hclspec.NewBlock("os", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"arch":    hclspec.NewAttr("arch", "string", false),
			"machine": hclspec.NewAttr("machine", "string", false),
		})),
	})

	// capabilities indicates what optional features this driver supports
	// this should be set according to the target run time.
	capabilities = &drivers.Capabilities{
		// TODO: set all the plugin's capabilities
		//
		// The plugin's capabilities signal Nomad which extra functionalities
		// are supported. For a list of available options check the docs page:
		// https://godoc.org/github.com/hashicorp/nomad/plugins/drivers#Capabilities
		// SendSignals:          false,
		Exec:                 true,
		DisableLogCollection: true,
		FSIsolation:          fsisolation.Image,

		// NetIsolationModes details that this driver only supports the network
		// isolation of host.
		NetIsolationModes: []drivers.NetIsolationMode{
			drivers.NetIsolationModeHost,
		},

		// MustInitiateNetwork is set to false, indicating the driver does not
		// implement and thus satisfy the Nomad drivers.DriverNetworkManager
		// interface.
		MustInitiateNetwork: false,

		//MountConfigs: MountConfigSupport
		//RemoteTasks: bool
		//DynamicWorkloadUsers: bool
	}
)

// TaskConfig contains configuration information for a task that runs within
// this plugin.
type TaskConfig struct {
	ImagePath           string         `codec:"image"`
	Hostname            string         `codec:"hostname"`
	OS                  *OS            `codec:"os"`
	UserData            string         `codec:"user_data"`
	TimeZone            *time.Location `codec:"timezone"`
	CMDs                []string       `codec:"cmds"`
	DefaultUserSSHKey   string         `codec:"default_user_authorized_ssh_key"`
	DefaultUserPassword string         `codec:"default_user_password"`
	UseThinCopy         bool           `codec:"use_thin_copy"`
	Disk                uint64         `codec:"disk"`
	// The list of network interfaces that should be added to the VM.
	net.NetworkInterfacesConfig `codec:"network_interface"`
}

type OS struct {
	Arch    string `codec:"arch"`
	Machine string `codec:"machine"`
}

type Emulator struct {
	URI      string `codec:"uri"`
	User     string `codec:"user"`
	Password string `codec:"password"`
}

// Config contains configuration information for the plugin
type Config struct {
	Emulator Emulator `codec:"emulator"`
	DataDir  string   `codec:"data_dir"`
	// ImagePaths is an allow-list of paths qemu is allowed to load an image from
	ImagePaths []string `codec:"image_paths"`
}

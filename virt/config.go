// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"time"

	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/drivers/fsisolation"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

var (
	// configSpec is the specification of the plugin's configuration
	// this is used to validate the configuration specified for the plugin
	// on the client.
	// this is not global, but can be specified on a per-client basis.
	configSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"emulator_uri": hclspec.NewDefault(
			hclspec.NewAttr("emulator_uri", "string", false),
			hclspec.NewLiteral("qemu:///system"),
		),
		"data_dir": hclspec.NewDefault(
			hclspec.NewAttr("data_dir", "string", false),
			hclspec.NewLiteral("/opt/virt"),
		),
		//"nvmeof_subsystem": hclspec.NewDefault(hclspec.NewBlock("nvmeof_subsystem", false, hclspec.NewObject(map[string]*hclspec.Spec{})), hclspec.NewLiteral("")),
	})

	// taskConfigSpec is the specification of the plugin's configuration for
	// a task
	// this is used to validated the configuration specified for the plugin
	// when a job is submitted.
	taskConfigSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"image":              hclspec.NewAttr("image", "string", true),
		"hostname":           hclspec.NewAttr("hostname", "string", false),
		"user_data":          hclspec.NewAttr("user_data", "string", false),
		"authorized_ssh_key": hclspec.NewAttr("authorized_ssh_key", "string", false),
		"password":           hclspec.NewAttr("password", "string", false),
		"cmds":               hclspec.NewAttr("cmds", "list(string)", false),
		"os": hclspec.NewBlock("os", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"arch":    hclspec.NewAttr("arch", "string", false),
			"machine": hclspec.NewAttr("machine", "string", false),
			"type":    hclspec.NewAttr("type", "string", false),
		})),
		"network_interface": hclspec.NewBlockList("network_interface", hclspec.NewObject(map[string]*hclspec.Spec{
			"network_name": hclspec.NewAttr("network_name", "string", false),
			"address":      hclspec.NewAttr("address", "string", false),
		})),
	})

	// capabilities indicates what optional features this driver supports
	// this should be set according to the target run time.
	capabilities = &drivers.Capabilities{
		// TODO: set plugin's capabilities
		//
		// The plugin's capabilities signal Nomad which extra functionalities
		// are supported. For a list of available options check the docs page:
		// https://godoc.org/github.com/hashicorp/nomad/plugins/drivers#Capabilities
		// SendSignals:          false,
		Exec:                 true,
		DisableLogCollection: true,
		FSIsolation:          fsisolation.Image,
		//NetIsolationModes: []NetIsolationMode{},
		//MustInitiateNetwork: false,
		//MountConfigs: MountConfigSupport
		//RemoteTasks: bool
		//DynamicWorkloadUsers: bool
	}
)

// TaskConfig contains configuration information for a task that runs within
// this plugin.
type TaskConfig struct {
	ImagePath        string             `codec:"image"`
	Hostname         string             `codec:"hostname"`
	OS               OS                 `codec:"os"`
	UserData         string             `codec:"user_data"`
	NetworkInterface []NetworkInterface `codec:"network_interface"`
	TimeZone         *time.Location     `codec:"timezone"`
	CMDs             []string           `codec:"cmds"`
	SSHKey           string             `codec:"authorized_ssh_key"`
	Password         string             `codec:"password"`
}

type OS struct {
	Arch    string `codec:"arch"`
	Machine string `codec:"machine"`
	Type    string `codec:"type"`
}

type NetworkInterface struct {
	NetworkName string `codec:"network_name"`
	Address     string `codec:"address"`
}

// Config contains configuration information for the plugin
type Config struct {
	EmulatorURI string `codec:"emulator_uri"`
	DataDir     string `codec:"data_dir"`
}

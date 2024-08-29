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
	// configSpec is the specification of the plugin's configuration
	// this is used to validate the configuration specified for the plugin
	// on the client.
	// this is not global, but can be specified on a per-client basis.
	configSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"emulator": hclspec.NewDefault(
			hclspec.NewAttr("emulator", "string", false),
			hclspec.NewLiteral(`"/usr/bin/qemu-system-x86_64"`),
		),
	})

	// taskConfigSpec is the specification of the plugin's configuration for
	// a task
	// this is used to validated the configuration specified for the plugin
	// when a job is submitted.
	taskConfigSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"type": hclspec.NewAttr("type", "string", true),

		"os": hclspec.NewBlock("os", true, hclspec.NewObject(map[string]*hclspec.Spec{
			"arch":    hclspec.NewAttr("arch", "string", true),
			"machine": hclspec.NewAttr("machine", "string", true),
			"type":    hclspec.NewAttr("type", "string", true),
		})),

		"disk": hclspec.NewBlockList("disk", hclspec.NewObject(map[string]*hclspec.Spec{
			"source": hclspec.NewAttr("source", "string", true),
			"driver": hclspec.NewBlock("driver", true, hclspec.NewObject(map[string]*hclspec.Spec{
				"name": hclspec.NewAttr("name", "string", true),
				"type": hclspec.NewAttr("type", "string", true),
			})),
			"target": hclspec.NewAttr("target", "string", true),
			"device": hclspec.NewAttr("device", "string", true),
		})),

		"network_interface": net.NetworkInterfaceHCLSpec(),

		"vnc": hclspec.NewBlock("vnc", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"port":      hclspec.NewAttr("port", "number", false),
			"websocket": hclspec.NewAttr("websocket", "number", false),
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

// Config contains configuration information for the plugin
type Config struct {
	URI string `codec:"uri"`
}

// TaskConfig contains configuration information for a task that runs within
// this plugin.
type TaskConfig struct {
	ImagePath        string         `codec:"image"`
	Type             string         `codec:"type"`
	OSVariant        OS             `codec:"os"`
	Disk             []Disk         `codec:"disk"`
	VNC              *VNC           `codec:"vnc"`
	TimeZone         *time.Location `codec:"timezone"`
	WorkloadIdentity string         `codec:"workload_identity"`

	// The list of network interfaces that should be added to the VM.
	net.NetworkInterfacesConfig `codec:"network_interface"`
}

type OS struct {
	Arch    string `codec:"arch"`
	Machine string `codec:"machine"`
	Type    string `codec:"type"`
}

type Disk struct {
	Source string `codec:"source"`
	Target string `codec:"target"`
	Device string `codec:"device"`
	Driver Driver `codec:"driver"`
}

type Driver struct {
	Name string `codec:"name"`
	Type string `codec:"type"`
}

type NetworkInterface struct {
	NetworkName string `codec:"network_name"`
	Address     string `codec:"address"`
}

type VNC struct {
	Port      int `codec:"port"`
	Websocket int `codec:"websocket"`
}

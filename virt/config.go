// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"time"

	domain "github.com/ccheshirecat/nomad-driver-ch/internal/shared"
	"github.com/ccheshirecat/nomad-driver-ch/virt/net"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/drivers/fsisolation"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

var (
	configSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"cloud_hypervisor": hclspec.NewBlock("cloud_hypervisor", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"bin": hclspec.NewDefault(
				hclspec.NewAttr("bin", "string", false),
				hclspec.NewLiteral(`"/usr/bin/cloud-hypervisor"`),
			),
			"remote_bin": hclspec.NewDefault(
				hclspec.NewAttr("remote_bin", "string", false),
				hclspec.NewLiteral(`"/usr/bin/ch-remote"`),
			),
			"virtiofsd_bin": hclspec.NewDefault(
				hclspec.NewAttr("virtiofsd_bin", "string", false),
				hclspec.NewLiteral(`"/usr/lib/virtiofsd"`),
			),
			"default_kernel":   hclspec.NewAttr("default_kernel", "string", false),
			"default_initramfs": hclspec.NewAttr("default_initramfs", "string", false),
			"firmware":         hclspec.NewAttr("firmware", "string", false),
			"seccomp": hclspec.NewDefault(
				hclspec.NewAttr("seccomp", "string", false),
				hclspec.NewLiteral(`"true"`),
			),
			"log_file": hclspec.NewAttr("log_file", "string", false),
		})),
		"network": hclspec.NewBlock("network", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"bridge": hclspec.NewDefault(
				hclspec.NewAttr("bridge", "string", false),
				hclspec.NewLiteral(`"br0"`),
			),
			"subnet_cidr": hclspec.NewDefault(
				hclspec.NewAttr("subnet_cidr", "string", false),
				hclspec.NewLiteral(`"192.168.1.0/24"`),
			),
			"gateway": hclspec.NewDefault(
				hclspec.NewAttr("gateway", "string", false),
				hclspec.NewLiteral(`"192.168.1.1"`),
			),
			"ip_pool_start": hclspec.NewDefault(
				hclspec.NewAttr("ip_pool_start", "string", false),
				hclspec.NewLiteral(`"192.168.1.100"`),
			),
			"ip_pool_end": hclspec.NewDefault(
				hclspec.NewAttr("ip_pool_end", "string", false),
				hclspec.NewLiteral(`"192.168.1.200"`),
			),
			"tap_prefix": hclspec.NewDefault(
				hclspec.NewAttr("tap_prefix", "string", false),
				hclspec.NewLiteral(`"tap-"`),
			),
		})),
		"vfio": hclspec.NewBlock("vfio", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"allowlist":           hclspec.NewAttr("allowlist", "list(string)", false),
			"iommu_address_width": hclspec.NewAttr("iommu_address_width", "number", false),
			"pci_segments":        hclspec.NewAttr("pci_segments", "number", false),
		})),
		"data_dir":     hclspec.NewAttr("data_dir", "string", false),
		"image_paths": hclspec.NewAttr("image_paths", "list(string)", false),
	})

	// taskConfigSpec is the specification of the plugin's configuration for
	// a task
	// this is used to validated the configuration specified for the plugin
	// when a job is submitted.
	taskConfigSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		// Existing fields
		"network_interface":               net.NetworkInterfaceHCLSpec(),
		"use_thin_copy":                   hclspec.NewAttr("use_thin_copy", "bool", false),
		"primary_disk_size":               hclspec.NewAttr("primary_disk_size", "number", true),
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
		// Cloud Hypervisor specific fields
		"kernel":       hclspec.NewAttr("kernel", "string", false),
		"initramfs":    hclspec.NewAttr("initramfs", "string", false),
		"cmdline":      hclspec.NewAttr("cmdline", "string", false),
		"max_vcpus":    hclspec.NewAttr("max_vcpus", "number", false),
		"features":     hclspec.NewAttr("features", "list(string)", false),
		"memory_hugepages": hclspec.NewAttr("memory_hugepages", "bool", false),
		"memory_shared":    hclspec.NewAttr("memory_shared", "bool", false),
		"hotplug_method":   hclspec.NewAttr("hotplug_method", "string", false),
		"hotplug_size":     hclspec.NewAttr("hotplug_size", "string", false),
		"disks": hclspec.NewBlockList("disk", hclspec.NewObject(map[string]*hclspec.Spec{
			"path":              hclspec.NewAttr("path", "string", true),
			"readonly":          hclspec.NewAttr("readonly", "bool", false),
			"serial":            hclspec.NewAttr("serial", "string", false),
			"rate_limit_group": hclspec.NewAttr("rate_limit_group", "string", false),
		})),
		"fs_mounts": hclspec.NewBlockList("fs_mount", hclspec.NewObject(map[string]*hclspec.Spec{
			"tag":         hclspec.NewAttr("tag", "string", true),
			"source":      hclspec.NewAttr("source", "string", true),
			"destination": hclspec.NewAttr("destination", "string", true),
			"num_queues":  hclspec.NewAttr("num_queues", "number", false),
			"queue_size":  hclspec.NewAttr("queue_size", "number", false),
		})),
		"vsock": hclspec.NewBlock("vsock", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"cid":    hclspec.NewAttr("cid", "number", true),
			"socket": hclspec.NewAttr("socket", "string", false),
		})),
		"rng": hclspec.NewBlock("rng", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"src": hclspec.NewAttr("src", "string", false),
		})),
		"devices": hclspec.NewBlockList("device", hclspec.NewObject(map[string]*hclspec.Spec{
			"path":   hclspec.NewAttr("path", "string", true),
			"id":     hclspec.NewAttr("id", "string", false),
			"iommu":  hclspec.NewAttr("iommu", "bool", false),
		})),
		"platform": hclspec.NewBlock("platform", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"num_pci_segments":   hclspec.NewAttr("num_pci_segments", "number", false),
			"iommu_segments":     hclspec.NewAttr("iommu_segments", "list(number)", false),
			"iommu_address_width": hclspec.NewAttr("iommu_address_width", "number", false),
		})),
	})

	// capabilities indicates what optional features this driver supports
	// this should be set according to the target run time.
	capabilities = &drivers.Capabilities{
		// The plugin's capabilities signal Nomad which extra functionalities
		// are supported. For a list of available options check the docs page:
		// https://godoc.org/github.com/hashicorp/nomad/plugins/drivers#Capabilities
		SendSignals:          false,
		Exec:                 false,
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

		// MountConfigs is currently not supported, although the plumbing is
		// ready to handle this.
		MountConfigs: drivers.MountConfigSupportNone,
	}
)

// TaskConfig contains configuration information for a task that runs within
// this plugin.
type TaskConfig struct {
	// Existing fields
	ImagePath           string         `codec:"image"`
	Hostname            string         `codec:"hostname"`
	OS                  *OS            `codec:"os"`
	UserData            string         `codec:"user_data"`
	TimeZone            *time.Location `codec:"timezone"`
	CMDs                []string       `codec:"cmds"`
	DefaultUserSSHKey   string         `codec:"default_user_authorized_ssh_key"`
	DefaultUserPassword string         `codec:"default_user_password"`
	UseThinCopy         bool           `codec:"use_thin_copy"`
	PrimaryDiskSize     uint64         `codec:"primary_disk_size"`
	// The list of network interfaces that should be added to the VM.
	net.NetworkInterfacesConfig `codec:"network_interface"`
	// Cloud Hypervisor specific fields
	Kernel          string           `codec:"kernel"`
	Initramfs       string           `codec:"initramfs"`
	Cmdline         string           `codec:"cmdline"`
	MaxVCPUs        uint             `codec:"max_vcpus"`
	Features        []string         `codec:"features"`
	MemoryHugepages bool             `codec:"memory_hugepages"`
	MemoryShared    bool             `codec:"memory_shared"`
	HotplugMethod   string           `codec:"hotplug_method"`
	HotplugSize     string           `codec:"hotplug_size"`
	Disks           []DiskConfig     `codec:"disks"`
	FSMounts        []FSMountConfig  `codec:"fs_mounts"`
	Vsock           *VsockConfig     `codec:"vsock"`
	Rng             *RngConfig       `codec:"rng"`
	Devices         []DeviceConfig   `codec:"devices"`
	Platform        *PlatformConfig  `codec:"platform"`
}

type OS struct {
	Arch    string `codec:"arch"`
	Machine string `codec:"machine"`
}


type DiskConfig struct {
	Path           string `codec:"path"`
	Readonly       bool   `codec:"readonly"`
	Serial         string `codec:"serial"`
	RateLimitGroup string `codec:"rate_limit_group"`
}

type FSMountConfig struct {
	Tag         string `codec:"tag"`
	Source      string `codec:"source"`
	Destination string `codec:"destination"`
	NumQueues   uint   `codec:"num_queues"`
	QueueSize   uint   `codec:"queue_size"`
}

type VsockConfig struct {
	CID    uint   `codec:"cid"`
	Socket string `codec:"socket"`
}

type RngConfig struct {
	Src string `codec:"src"`
}

type DeviceConfig struct {
	Path  string `codec:"path"`
	ID    string `codec:"id"`
	IOMMU bool   `codec:"iommu"`
}

type PlatformConfig struct {
	NumPCISegments    uint   `codec:"num_pci_segments"`
	IOMMUSegments     []uint `codec:"iommu_segments"`
	IOMMUAddressWidth uint   `codec:"iommu_address_width"`
}

// Config contains configuration information for the plugin
type Config struct {
	CloudHypervisor domain.CloudHypervisor `codec:"cloud_hypervisor"`
	Network         domain.Network         `codec:"network"`
	VFIO            domain.VFIO            `codec:"vfio"`
	DataDir         string          `codec:"data_dir"`
	// ImagePaths is an allow-list of paths cloud hypervisor is allowed to load an image from
	ImagePaths []string `codec:"image_paths"`
}

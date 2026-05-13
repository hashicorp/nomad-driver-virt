// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"testing"

	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/shoenig/test/must"
	"libvirt.org/go/libvirtxml"
)

func Test_generateDomainDeviceControllers(t *testing.T) {
	zero := uint(0)
	testCases := []struct {
		desc    string
		volumes []storage.Volume
		result  []libvirtxml.DomainController
	}{
		{
			desc: "ok",
			volumes: []storage.Volume{
				{BusType: storage.BusTypeUsb},
				{BusType: storage.BusTypeVirtio},
			},
			result: []libvirtxml.DomainController{
				{Type: "usb", Index: &zero},
				{Type: "virtio-serial", Index: &zero},
			},
		},
		{
			desc: "duplicates",
			volumes: []storage.Volume{
				{BusType: storage.BusTypeUsb},
				{BusType: storage.BusTypeVirtio},
				{BusType: storage.BusTypeUsb},
				{BusType: storage.BusTypeVirtio},
			},
			result: []libvirtxml.DomainController{
				{Type: "usb", Index: &zero},
				{Type: "virtio-serial", Index: &zero},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			p, _ := testNew(t, overrideFs(mountFs9p))
			must.Eq(t, tc.result, p.generateDomainDeviceControllers(tc.volumes))
		})
	}
}

func Test_generateDomainDeviceDisks(t *testing.T) {
	testCases := []struct {
		desc    string
		volumes []storage.Volume
		result  []libvirtxml.DomainDisk
	}{
		{
			desc: "ok",
			volumes: []storage.Volume{
				{
					Name:    "primary",
					Primary: true,
					Block:   "/dev/null",
				},
			},
			result: []libvirtxml.DomainDisk{
				{
					Source: &libvirtxml.DomainDiskSource{
						Block: &libvirtxml.DomainDiskSourceBlock{
							Dev: "/dev/null",
						},
					},
					Boot: &libvirtxml.DomainDeviceBoot{
						Order: 1,
					},
					Target: &libvirtxml.DomainDiskTarget{},
					Driver: &libvirtxml.DomainDiskDriver{},
				},
			},
		},
		{
			desc: "multiple",
			volumes: []storage.Volume{
				{
					Name:  "secondary",
					Block: "/dev/null/other",
				},
				{
					Name:    "primary",
					Primary: true,
					Block:   "/dev/null",
				},
			},
			result: []libvirtxml.DomainDisk{
				{
					Source: &libvirtxml.DomainDiskSource{
						Block: &libvirtxml.DomainDiskSourceBlock{
							Dev: "/dev/null/other",
						},
					},
					Target: &libvirtxml.DomainDiskTarget{},
					Driver: &libvirtxml.DomainDiskDriver{},
				},
				{
					Source: &libvirtxml.DomainDiskSource{
						Block: &libvirtxml.DomainDiskSourceBlock{
							Dev: "/dev/null",
						},
					},
					Boot: &libvirtxml.DomainDeviceBoot{
						Order: 1,
					},
					Target: &libvirtxml.DomainDiskTarget{},
					Driver: &libvirtxml.DomainDiskDriver{},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			p, _ := testNew(t, overrideFs(mountFs9p))
			disks, err := p.generateDomainDeviceDisks(tc.volumes)
			must.NoError(t, err)
			must.Eq(t, tc.result, disks)
		})
	}
}

func Test_generateDomainDeviceInterfaces(t *testing.T) {
	testCases := []struct {
		desc    string
		configs net.NetworkInterfacesConfig
		result  []libvirtxml.DomainInterface
	}{
		{
			desc: "bridge",
			configs: net.NetworkInterfacesConfig{
				{
					Bridge: &net.NetworkInterfaceBridgeConfig{
						Name: "virbr0",
					},
				},
			},
			result: []libvirtxml.DomainInterface{
				{
					Source: &libvirtxml.DomainInterfaceSource{
						Bridge: &libvirtxml.DomainInterfaceSourceBridge{
							Bridge: "virbr0",
						},
					},
					Model: &libvirtxml.DomainInterfaceModel{
						Type: defaultInterfaceModel,
					},
				},
			},
		},
		{
			desc: "macvtap",
			configs: net.NetworkInterfacesConfig{
				{
					Macvtap: &net.NetworkInterfaceMacvtapConfig{
						Device: "eth0",
						Mode:   "bridge",
					},
				},
			},
			result: []libvirtxml.DomainInterface{
				{
					Source: &libvirtxml.DomainInterfaceSource{
						Direct: &libvirtxml.DomainInterfaceSourceDirect{
							Dev:  "eth0",
							Mode: "bridge",
						},
					},
					Model: &libvirtxml.DomainInterfaceModel{
						Type: defaultInterfaceModel,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			p, _ := testNew(t, overrideFs(mountFs9p))
			must.Eq(t, tc.result, p.generateDomainDeviceInterfaces(tc.configs))
		})
	}
}

func Test_generateDomainDeviceFilesystems(t *testing.T) {
	testCases := []struct {
		desc    string
		configs []vm.MountFileConfig
		result  []libvirtxml.DomainFilesystem
		setup   func(p *provider)
	}{
		{
			desc: "basic",
			configs: []vm.MountFileConfig{
				{
					Source:      "/dev/null",
					Destination: "/mnt/null",
					Tag:         "null-tag",
				},
			},
			result: []libvirtxml.DomainFilesystem{
				{
					AccessMode: defaultSecurityMode,
					Source: &libvirtxml.DomainFilesystemSource{
						Mount: &libvirtxml.DomainFilesystemSourceMount{
							Dir: "/dev/null",
						},
					},
					Target: &libvirtxml.DomainFilesystemTarget{
						Dir: "null-tag",
					},
				},
			},
		},
		{
			desc: "readonly",
			configs: []vm.MountFileConfig{
				{
					Source:      "/dev/null",
					Destination: "/mnt/null",
					Tag:         "null-tag",
					ReadOnly:    true,
				},
			},
			result: []libvirtxml.DomainFilesystem{
				{
					AccessMode: defaultSecurityMode,
					Source: &libvirtxml.DomainFilesystemSource{
						Mount: &libvirtxml.DomainFilesystemSourceMount{
							Dir: "/dev/null",
						},
					},
					Target: &libvirtxml.DomainFilesystemTarget{
						Dir: "null-tag",
					},
					ReadOnly: &libvirtxml.DomainFilesystemReadOnly{},
				},
			},
		},
		{
			desc: "virtiofs",
			configs: []vm.MountFileConfig{
				{
					Source:      "/dev/null",
					Destination: "/mnt/null",
					Tag:         "null-tag",
					Driver:      mountFsVirtiofs,
				},
			},
			result: []libvirtxml.DomainFilesystem{
				{
					AccessMode: virtiofsSecurityMode,
					Source: &libvirtxml.DomainFilesystemSource{
						Mount: &libvirtxml.DomainFilesystemSourceMount{
							Dir: "/dev/null",
						},
					},
					Target: &libvirtxml.DomainFilesystemTarget{
						Dir: "null-tag",
					},
					Driver: &libvirtxml.DomainFilesystemDriver{
						Type:  "virtiofs",
						Queue: virtiofsQueueSize,
					},
				},
			},
		},
		{
			desc: "virtiofs readonly",
			configs: []vm.MountFileConfig{
				{
					Source:      "/dev/null",
					Destination: "/mnt/null",
					Tag:         "null-tag",
					Driver:      mountFsVirtiofs,
					ReadOnly:    true,
				},
			},
			result: []libvirtxml.DomainFilesystem{
				{
					AccessMode: virtiofsSecurityMode,
					Source: &libvirtxml.DomainFilesystemSource{
						Mount: &libvirtxml.DomainFilesystemSourceMount{
							Dir: "/dev/null",
						},
					},
					Target: &libvirtxml.DomainFilesystemTarget{
						Dir: "null-tag",
					},
					Driver: &libvirtxml.DomainFilesystemDriver{
						Type:  "virtiofs",
						Queue: virtiofsQueueSize,
					},
					ReadOnly: &libvirtxml.DomainFilesystemReadOnly{},
				},
			},
		},
		{
			desc: "virtiofs readonly disabled",
			configs: []vm.MountFileConfig{
				{
					Source:      "/dev/null",
					Destination: "/mnt/null",
					Tag:         "null-tag",
					Driver:      mountFsVirtiofs,
					ReadOnly:    true,
				},
			},
			result: []libvirtxml.DomainFilesystem{
				{
					AccessMode: virtiofsSecurityMode,
					Source: &libvirtxml.DomainFilesystemSource{
						Mount: &libvirtxml.DomainFilesystemSourceMount{
							Dir: "/dev/null",
						},
					},
					Target: &libvirtxml.DomainFilesystemTarget{
						Dir: "null-tag",
					},
					Driver: &libvirtxml.DomainFilesystemDriver{
						Type:  "virtiofs",
						Queue: virtiofsQueueSize,
					},
				},
			},
			setup: func(p *provider) { p.insecureReadonlyMounts = true },
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			p, _ := testNew(t, overrideFs(mountFs9p))
			if tc.setup != nil {
				tc.setup(p)
			}
			must.Eq(t, tc.result, p.generateDomainDeviceFilesystems(tc.configs))
		})
	}
}

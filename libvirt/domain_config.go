// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	domain "github/hashicorp/nomad-driver-virt/internal/shared"

	"libvirt.org/go/libvirtxml"
)

func parceConfiguration(config *domain.Config, cloudInitPath string) (string, error) {
	cero := uint(0)

	mounts := []libvirtxml.DomainFilesystem{}
	for _, m := range config.Mounts {
		m := libvirtxml.DomainFilesystem{
			AccessMode: "passthrough",
			Driver: &libvirtxml.DomainFilesystemDriver{
				Type: "virtiofs",
			},
			Binary: &libvirtxml.DomainFilesystemBinary{
				Path: "/usr/lib/qemu/virtiofsd",
			},
			Source: &libvirtxml.DomainFilesystemSource{
				Mount: &libvirtxml.DomainFilesystemSourceMount{
					Dir: m.Source,
				},
			},
			Target: &libvirtxml.DomainFilesystemTarget{
				Dir: m.Tag,
			},
		}

		mounts = append(mounts, m)
	}

	interfaces := []libvirtxml.DomainInterface{}
	for _, ni := range config.NetworkInterfaces {
		i := libvirtxml.DomainInterface{
			Source: &libvirtxml.DomainInterfaceSource{
				Bridge: &libvirtxml.DomainInterfaceSourceBridge{
					Bridge: ni,
				},
			},
			Model: &libvirtxml.DomainInterfaceModel{
				Type: "virtio",
			},
		}

		interfaces = append(interfaces, i)
	}

	os := &libvirtxml.DomainOS{
		Type: &libvirtxml.DomainOSType{
			Type: "hvm",
		},
		SMBios: &libvirtxml.DomainSMBios{
			Mode: "sysinfo",
		},
	}

	if config.Arch != "" {
		os.Type.Arch = config.Arch
	}

	if config.Machine != "" {
		os.Type.Machine = config.Machine
	}

	if config.OSType != "" {
		os.Type.Type = config.OSType
	}

	domcfg := &libvirtxml.Domain{
		OnPoweroff: "destroy",
		OnReboot:   "destroy",
		OnCrash:    "destroy",
		PM: &libvirtxml.DomainPM{
			SuspendToMem: &libvirtxml.DomainPMPolicy{
				Enabled: "no",
			},
			SuspendToDisk: &libvirtxml.DomainPMPolicy{
				Enabled: "no",
			},
		},
		Features: &libvirtxml.DomainFeatureList{
			VMPort: &libvirtxml.DomainFeatureState{
				State: "off",
			},
		},
		SysInfo: []libvirtxml.DomainSysInfo{
			{
				SMBIOS: &libvirtxml.DomainSysInfoSMBIOS{
					System: &libvirtxml.DomainSysInfoSystem{
						Entry: []libvirtxml.DomainSysInfoEntry{
							{
								Name:  "serial",
								Value: "ds=nocloud",
							},
						},
					},
				},
			},
		},
		OS: os,
		Devices: &libvirtxml.DomainDeviceList{
			Controllers: []libvirtxml.DomainController{
				{
					Type:  "virtio-serial",
					Index: &cero,
				},
				{
					Type:  "sata",
					Index: &cero,
				},
			},
			Serials: []libvirtxml.DomainSerial{
				{
					Target: &libvirtxml.DomainSerialTarget{
						Type: "isa-serial",
						Port: &cero,
						Model: &libvirtxml.DomainSerialTargetModel{
							Name: "isa-serial",
						},
					},
				},
			},
			Consoles: []libvirtxml.DomainConsole{
				{
					Target: &libvirtxml.DomainConsoleTarget{
						Type: "serial",
						Port: &cero,
					},
				},
			},
			RNGs: []libvirtxml.DomainRNG{
				{
					Model: "virtio",
					Backend: &libvirtxml.DomainRNGBackend{
						Random: &libvirtxml.DomainRNGBackendRandom{
							Device: "/dev/urandom",
						},
					},
				},
			},
			Disks: []libvirtxml.DomainDisk{
				{
					Device: "disk",
					Driver: &libvirtxml.DomainDiskDriver{
						Name: "qemu",
						Type: "qcow2",
					},
					Source: &libvirtxml.DomainDiskSource{
						File: &libvirtxml.DomainDiskSourceFile{
							File: config.BaseImage,
						},
					},
					Target: &libvirtxml.DomainDiskTarget{
						Dev: "vda",
						Bus: "virtio",
					},
				},
				{
					Device: "cdrom",
					Driver: &libvirtxml.DomainDiskDriver{
						Name: "qemu",
						Type: "raw",
					},
					Source: &libvirtxml.DomainDiskSource{
						File: &libvirtxml.DomainDiskSourceFile{
							File: cloudInitPath,
						},
					},
					Target: &libvirtxml.DomainDiskTarget{
						Dev: "sda",
						Bus: "sata",
					},
					ReadOnly: &libvirtxml.DomainDiskReadOnly{},
				},
			},
			Filesystems: mounts,
			Interfaces:  interfaces,
		},
		Type: "kvm",
		Name: config.Name,
		Memory: &libvirtxml.DomainMemory{
			Value: config.Memory,
		},
		MemoryBacking: &libvirtxml.DomainMemoryBacking{
			MemorySource: &libvirtxml.DomainMemorySource{
				Type: "memfd",
			},
			MemoryAccess: &libvirtxml.DomainMemoryAccess{
				Mode: "shared",
			},
		},
		VCPU: &libvirtxml.DomainVCPU{
			Placement: "static",
			Value:     uint(config.CPUs),
		},
		Resource: &libvirtxml.DomainResource{
			Partition: "/machine",
		},
		/*  CPU: &libvirtxml.DomainCPU{
			Topology: &libvirtxml.DomainCPUTopology{
				Cores:   config.CPUs,
				Sockets: 2,
				Threads: 1,
			},
		}, */
	}

	return domcfg.Marshal()
}

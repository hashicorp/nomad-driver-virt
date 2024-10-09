// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	domain "github.com/hashicorp/nomad-driver-virt/internal/shared"

	"libvirt.org/go/libvirtxml"
)

func parseConfiguration(config *domain.Config, cloudInitPath string) (string, error) {
	zero := uint(0)

	disks := []libvirtxml.DomainDisk{
		{
			Device: "disk",
			Driver: &libvirtxml.DomainDiskDriver{
				Name: "qemu",
				Type: config.DiskFmt,
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
		},
	}

	mounts := []libvirtxml.DomainFilesystem{}
	for _, m := range config.Mounts {

		var ro *libvirtxml.DomainFilesystemReadOnly
		if m.ReadOnly {
			ro = &libvirtxml.DomainFilesystemReadOnly{}
		}

		m := libvirtxml.DomainFilesystem{
			AccessMode: defaultSecurityMode,
			ReadOnly:   ro,
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

	osType := &libvirtxml.DomainOSType{
		Type: defaultVirtualizatioType,
	}

	if config.OsVariant != nil {
		osType.Arch = config.OsVariant.Arch
		osType.Machine = config.OsVariant.Machine
	}

	interfaces := []libvirtxml.DomainInterface{}
	if config.NetworkInterfaces != nil {
		for _, networkInterface := range config.NetworkInterfaces {
			if networkInterface.Bridge != nil {
				interfaces = append(interfaces, libvirtxml.DomainInterface{
					Source: &libvirtxml.DomainInterfaceSource{
						Bridge: &libvirtxml.DomainInterfaceSourceBridge{
							Bridge: networkInterface.Bridge.Name,
						},
					},
					Model: &libvirtxml.DomainInterfaceModel{
						Type: defaultInterfaceModel,
					},
				})
			}
		}
	}

	vcpus := &libvirtxml.DomainVCPU{
		Placement: "static",
		Value:     config.CPUs,
		CPUSet:    config.CPUset,
	}

	domcfg := &libvirtxml.Domain{
		VCPU: vcpus,
		MemoryTune: &libvirtxml.DomainMemoryTune{
			HardLimit: &libvirtxml.DomainMemoryTuneLimit{
				Value: uint64(config.Memory),
				Unit:  "M",
			},
		},
		MemoryBacking: &libvirtxml.DomainMemoryBacking{
			MemorySource: &libvirtxml.DomainMemorySource{
				Type: "memfd",
			},
			MemoryAccess: &libvirtxml.DomainMemoryAccess{
				Mode: "shared",
			},
		},
		OnPoweroff: "destroy",
		OnReboot:   "restart",
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
								Value: "ds=nocloud;",
							},
						},
					},
				},
			},
		},
		OS: &libvirtxml.DomainOS{
			Type: osType,
			SMBios: &libvirtxml.DomainSMBios{
				Mode: "sysinfo",
			},
		},
		Devices: &libvirtxml.DomainDeviceList{
			Controllers: []libvirtxml.DomainController{
				// Used for the base image disk
				{
					Type:  "virtio-serial",
					Index: &zero,
				},
				// Used for the cloud init iso (CDROM) disk
				{
					Type:  "sata",
					Index: &zero,
				},
			},
			Serials: []libvirtxml.DomainSerial{
				{
					Target: &libvirtxml.DomainSerialTarget{
						Type: "isa-serial",
						Port: &zero,
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
						Port: &zero,
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
			Channels: []libvirtxml.DomainChannel{
				// This is necessary for qemu agent, but it needs to be started inside the vm
				/* 	{

					Source: &libvirtxml.DomainChardevSource{
						UNIX: &libvirtxml.DomainChardevSourceUNIX{
							Mode: "bind",
							Path: "/var/lib/libvirt/qemu/f16x86_64.agent",
						},
					},
					Target: &libvirtxml.DomainChannelTarget{
						VirtIO: &libvirtxml.DomainChannelTargetVirtIO{
							Name: libvirtVirtioChannel,
						},
					},
				}, */
			},
			Disks:       disks,
			Filesystems: mounts,
			Interfaces:  interfaces,
		},
		Type: defaultAccelerator,
		Name: config.Name,
		Memory: &libvirtxml.DomainMemory{
			Value: config.Memory,
			Unit:  "M",
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

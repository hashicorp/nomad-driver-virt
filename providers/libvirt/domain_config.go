// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"libvirt.org/go/libvirtxml"
)

func (p *provider) parseVolumes(vols []storage.Volume) ([]libvirtxml.DomainDisk, error) {
	result := make([]libvirtxml.DomainDisk, 0, len(vols))
	for _, fd := range vols {
		disk, err := p.storage.VolumeToDisk(fd)
		if err != nil {
			return nil, err
		}

		// If this is the primary device, mark to boot
		if fd.Primary {
			disk.Boot = &libvirtxml.DomainDeviceBoot{Order: 1}
		}
		result = append(result, *disk)
	}

	return result, nil
}

func (p *provider) parseConfiguration(config *vm.Config) (string, error) {
	zero := uint(0)

	disks, err := p.parseVolumes(config.Volumes)
	if err != nil {
		return "", err
	}

	mounts := []libvirtxml.DomainFilesystem{}
	for _, m := range config.Mounts {

		var ro *libvirtxml.DomainFilesystemReadOnly
		if m.ReadOnly {
			ro = &libvirtxml.DomainFilesystemReadOnly{}
		}

		mnt := libvirtxml.DomainFilesystem{
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

		if m.Driver == mountFsVirtiofs {
			mnt.AccessMode = virtiofsSecurityMode
			mnt.Driver = &libvirtxml.DomainFilesystemDriver{
				Type:  "virtiofs",
				Queue: virtiofsQueueSize,
			}

			// If insecure read-only mounts are allowed, then disable the
			// read-only setting.
			if p.insecureReadonlyMounts {
				mnt.ReadOnly = nil
			}
		}

		mounts = append(mounts, mnt)
	}

	osType := &libvirtxml.DomainOSType{
		Type: defaultVirtualizationType,
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
			} else if networkInterface.Macvtap != nil {
				interfaces = append(interfaces, libvirtxml.DomainInterface{
					Source: &libvirtxml.DomainInterfaceSource{
						Direct: &libvirtxml.DomainInterfaceSourceDirect{
							Dev:  networkInterface.Macvtap.Device,
							Mode: string(networkInterface.Macvtap.Mode),
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
			// TODO: Check if these are actually needed with volumes
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

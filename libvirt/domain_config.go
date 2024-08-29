// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"fmt"

	domain "github.com/hashicorp/nomad-driver-virt/internal/shared"

	"libvirt.org/go/libvirtxml"
)

func parseConfiguration(config *domain.Config, cloudInitPath string) (string, error) {
	zero := uint(0)

	rootDisk := libvirtxml.DomainDisk{
		Device: "disk",
		Driver: &libvirtxml.DomainDiskDriver{
			Name: "qemu",
			Type: config.DiskFmt,
		},
		Source: &libvirtxml.DomainDiskSource{
			File: &libvirtxml.DomainDiskSourceFile{
				File: config.BaseImage,
			},
			Index: 2,
		},
		Target: &libvirtxml.DomainDiskTarget{
			Dev: "vda",
			Bus: "virtio",
		},
		// <backingStore type='file' index='3'>
		//   <format type='qcow2'/>
		//   <source file='/var/lib/libvirt/images/noble-server-cloudimg-amd64.img'/>
		//   <backingStore/>
		// </backingStore>
	}
	if config.BackingStore != "" {
		rootDisk.BackingStore = &libvirtxml.DomainDiskBackingStore{
			Format: &libvirtxml.DomainDiskFormat{
				Type: config.DiskFmt,
			},
			Source: &libvirtxml.DomainDiskSource{
				File: &libvirtxml.DomainDiskSourceFile{
					File: config.BackingStore,
				},
				Index: 3,
			},
		}
	}

	disks := []libvirtxml.DomainDisk{
		rootDisk,
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
				Index: 1,
			},
			Target: &libvirtxml.DomainDiskTarget{
				Dev: "sda",
				Bus: "sata",
			},
			ReadOnly: &libvirtxml.DomainDiskReadOnly{},
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

	/*
	   <interface type='network'>
	      <mac address='52:54:00:98:d6:b9'/>
	      <source network='default' portid='3f839fe8-141b-4616-ba7c-579f986d09cc' bridge='virbr0'/>
	      <target dev='vnet38'/>
	      <model type='virtio'/>
	      <alias name='net0'/>
	      <address type='pci' domain='0x0000' bus='0x01' slot='0x00' function='0x0'/>
	    </interface>
	*/

	interfaces := []libvirtxml.DomainInterface{}
	for i, ni := range config.NetworkInterfaces {
		i := libvirtxml.DomainInterface{
			Source: &libvirtxml.DomainInterfaceSource{
				Bridge: &libvirtxml.DomainInterfaceSourceBridge{
					Bridge: ni.Name,
				},
			},
			Model: &libvirtxml.DomainInterfaceModel{},
			Target: &libvirtxml.DomainInterfaceTarget{
				Dev: fmt.Sprintf("vnet%d_%s", i, config.Name),
			},
		}
		if ni.Model != "" {
			i.Model.Type = ni.Model
		} else {
			i.Model.Type = defaultInterfaceModel
		}
		if ni.MAC != "" {
			i.MAC = &libvirtxml.DomainInterfaceMAC{
				Address: ni.MAC,
			}
		}

		interfaces = append(interfaces, i)
	}

	domcfg := &libvirtxml.Domain{
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
			ACPI: &libvirtxml.DomainFeature{},
			APIC: &libvirtxml.DomainFeatureAPIC{},
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
			Emulator: "/usr/bin/qemu-system-x86_64",
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
					Protocol: &libvirtxml.DomainChardevProtocol{
						Type: "pty",
					},
					Target: &libvirtxml.DomainSerialTarget{
						Type: "isa-serial",
						Port: &zero,
						Model: &libvirtxml.DomainSerialTargetModel{
							Name: "isa-serial",
						},
					},
				},
			},
			// --video qxl --channel spicevmc
			Consoles: []libvirtxml.DomainConsole{
				{
					Target: &libvirtxml.DomainConsoleTarget{
						Type: "serial",
						Port: &zero,
					},
					Protocol: &libvirtxml.DomainChardevProtocol{
						Type: "pty",
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
			Graphics: []libvirtxml.DomainGraphic{
				{
					Spice: &libvirtxml.DomainGraphicSpice{
						Port:     9999, // TODO: need to port-broker this or hide the VM in a netns?
						AutoPort: "yes",
						Listen:   "127.0.0.1",
						Listeners: []libvirtxml.DomainGraphicListener{{
							Address: &libvirtxml.DomainGraphicListenerAddress{
								Address: "127.0.0.1",
							},
						}},
						Image: &libvirtxml.DomainGraphicSpiceImage{
							Compression: "off",
						},
					},
				},
			},
			Videos: []libvirtxml.DomainVideo{{
				Model: libvirtxml.DomainVideoModel{
					Type: "virtio",
				},
				// Model: libvirtxml.DomainVideoModel{
				// 	Type: "qxl",
				// 	// Ram:     65536,
				// 	// VRam:    65536,
				// 	// VGAMem:  16384,
				// 	// Heads:   1,
				// 	// Primary: "yes",
				// },
				//Address: &libvirtxml.DomainAddress{
				//	//
				//},
			}},
			// <video>
			//   <model type='qxl' ram='65536' vram='65536' vgamem='16384' heads='1' primary='yes'/>
			//   <alias name='video0'/>
			//   <address type='pci' domain='0x0000' bus='0x00' slot='0x01' function='0x0'/>
			// </video>
			Channels: []libvirtxml.DomainChannel{
				{
					Protocol: &libvirtxml.DomainChardevProtocol{
						Type: "spicevmc",
					},
					Target: &libvirtxml.DomainChannelTarget{
						VirtIO: &libvirtxml.DomainChannelTargetVirtIO{
							Name: "com.redhat.spice.0",
						},
					},
				},
				// <channel type='spicevmc'>
				//   <target type='virtio' name='com.redhat.spice.0' state='disconnected'/>
				//   <alias name='channel1'/>
				//   <address type='virtio-serial' controller='0' bus='0' port='2'/>
				// </channel>
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
		CPU: &libvirtxml.DomainCPU{
			Mode:       "host-passthrough",
			Check:      "none",
			Migratable: "on",
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

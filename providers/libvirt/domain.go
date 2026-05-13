// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"slices"

	"github.com/hashicorp/go-set/v3"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"libvirt.org/go/libvirtxml"
)

// generateDomain generates the marshalled domain configuration.
func (p *provider) generateDomain(config *vm.Config) (string, error) {
	// Define the basics of the domain.
	domain := &libvirtxml.Domain{
		Type:       defaultAccelerator,
		Name:       config.Name,
		OnPoweroff: "destroy",
		OnReboot:   "restart",
		OnCrash:    "destroy",
		Resource: &libvirtxml.DomainResource{
			Partition: "/machine",
		},
	}
	// Configure the domain.
	configureDomainProcessors(config, domain)
	configureDomainMemory(config, domain)
	configureDomainOS(config, domain)
	configureDomainPowerManagement(config, domain)
	configureDomainFeatures(config, domain)
	configureDomainSysInfo(config, domain)

	// Configure the domain's devices.
	devices := &libvirtxml.DomainDeviceList{}
	domain.Devices = devices
	configureDomainDeviceConsoles(config, devices)
	configureDomainDeviceChannels(config, devices)

	// Generate the disks and the required controllers.
	disks, err := p.generateDomainDeviceDisks(config.Volumes)
	if err != nil {
		return "", err
	}
	devices.Disks = disks
	devices.Controllers = p.generateDomainDeviceControllers(config.Volumes)

	// Generate the filesystems to be mounted.
	devices.Filesystems = p.generateDomainDeviceFilesystems(config.Mounts)

	// Generate the network interfaces.
	devices.Interfaces = p.generateDomainDeviceInterfaces(config.NetworkInterfaces)

	return domain.Marshal()
}

// configureDomainSysInfo configures the domain sysinfo settings.
func configureDomainSysInfo(config *vm.Config, dom *libvirtxml.Domain) {
	dom.SysInfo = []libvirtxml.DomainSysInfo{
		{
			SMBIOS: &libvirtxml.DomainSysInfoSMBIOS{
				System: &libvirtxml.DomainSysInfoSystem{
					// This sets the discovery information for cloud-init using the
					// virtual machine's serial number.
					// https://docs.cloud-init.io/en/latest/reference/datasources/nocloud.html#discovery-configuration
					Entry: []libvirtxml.DomainSysInfoEntry{
						{
							Name:  "serial",
							Value: "ds=nocloud;",
						},
					},
				},
			},
		},
	}
}

// configureDomainFeatures configures the domain features.
func configureDomainFeatures(config *vm.Config, dom *libvirtxml.Domain) {
	dom.Features = &libvirtxml.DomainFeatureList{
		VMPort: &libvirtxml.DomainFeatureState{
			State: "off",
		},
	}
}

// configureDomainPowerManagement configures the domain power management settings.
func configureDomainPowerManagement(config *vm.Config, dom *libvirtxml.Domain) {
	dom.PM = &libvirtxml.DomainPM{
		SuspendToMem: &libvirtxml.DomainPMPolicy{
			Enabled: "no",
		},
		SuspendToDisk: &libvirtxml.DomainPMPolicy{
			Enabled: "no",
		},
	}
}

// configureDomainOS configures the domain OS settings.
func configureDomainOS(config *vm.Config, dom *libvirtxml.Domain) {
	osType := &libvirtxml.DomainOSType{
		Type: defaultVirtualizationType,
	}

	if config.OsVariant != nil {
		osType.Arch = config.OsVariant.Arch
		osType.Machine = config.OsVariant.Machine
	}

	dom.OS = &libvirtxml.DomainOS{
		Type: osType,
		SMBios: &libvirtxml.DomainSMBios{
			Mode: "sysinfo",
		},
	}
}

// configureDomainMemory configures the domain memory settings.
func configureDomainMemory(config *vm.Config, dom *libvirtxml.Domain) {
	dom.Memory = &libvirtxml.DomainMemory{
		Value: config.Memory,
		Unit:  "M",
	}
	dom.MemoryTune = &libvirtxml.DomainMemoryTune{
		HardLimit: &libvirtxml.DomainMemoryTuneLimit{
			Value: uint64(config.Memory),
			Unit:  "M",
		},
	}
	dom.MemoryBacking = &libvirtxml.DomainMemoryBacking{
		MemorySource: &libvirtxml.DomainMemorySource{
			Type: "memfd",
		},
		MemoryAccess: &libvirtxml.DomainMemoryAccess{
			Mode: "shared",
		},
	}
}

// configureDomainProcessors configures the domain CPU settings.
func configureDomainProcessors(config *vm.Config, dom *libvirtxml.Domain) {
	dom.VCPU = &libvirtxml.DomainVCPU{
		Placement: "static",
		Value:     config.CPUs,
		CPUSet:    config.CPUset,
	}
}

// configureDomainDeviceChannels configures the domain channel devices.
// NOTE: This is stubbed for future use with provider specific configuration.
func configureDomainDeviceChannels(config *vm.Config, devices *libvirtxml.DomainDeviceList) {
	devices.Channels = []libvirtxml.DomainChannel{
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
	}
}

// configureDomainDeviceRNG configures the domain random number generator devices.
func configureDomainDeviceRNG(config *vm.Config, devices *libvirtxml.DomainDeviceList) {
	devices.RNGs = []libvirtxml.DomainRNG{
		{
			Model: "virtio",
			Backend: &libvirtxml.DomainRNGBackend{
				Random: &libvirtxml.DomainRNGBackendRandom{
					Device: "/dev/urandom",
				},
			},
		},
	}
}

// configureDomainDeviceConsoles configures the domain console devices.
func configureDomainDeviceConsoles(config *vm.Config, devices *libvirtxml.DomainDeviceList) {
	devices.Consoles = []libvirtxml.DomainConsole{
		{
			TTY: "pty",
			Target: &libvirtxml.DomainConsoleTarget{
				Type: "serial",
			},
		},
		{
			TTY: "pty",
			Target: &libvirtxml.DomainConsoleTarget{
				Type: "virtio",
			},
		},
	}

}

// generateDomainDeviceControllers generates required libvirt controllers from storage volumes.
func (p *provider) generateDomainDeviceControllers(vols []storage.Volume) []libvirtxml.DomainController {
	zero := uint(0)
	types := set.New[string](0)
	for _, v := range vols {
		switch v.BusType {
		case storage.BusTypeVirtio:
			types.Insert("virtio-serial")
		default:
			types.Insert(v.BusType)
		}
	}

	controllers := make([]libvirtxml.DomainController, 0)
	for _, t := range slices.Sorted(types.Items()) {
		controllers = append(controllers, libvirtxml.DomainController{Type: t, Index: &zero})
	}

	return controllers
}

// generateDomainDeviceDisks generates libvirt disks from storage volumes.
func (p *provider) generateDomainDeviceDisks(vols []storage.Volume) ([]libvirtxml.DomainDisk, error) {
	result := make([]libvirtxml.DomainDisk, len(vols))
	for i, fd := range vols {
		disk, err := p.storage.VolumeToDisk(fd)
		if err != nil {
			return nil, err
		}

		// If this is the primary device, mark to boot
		if fd.Primary {
			disk.Boot = &libvirtxml.DomainDeviceBoot{Order: 1}
		}

		result[i] = *disk
	}

	return result, nil
}

// generateDomainNetInterfaces generates libvirt interfaces from network interface configs.
func (p *provider) generateDomainDeviceInterfaces(ifaces net.NetworkInterfacesConfig) []libvirtxml.DomainInterface {
	result := make([]libvirtxml.DomainInterface, len(ifaces))
	for i, iface := range ifaces {
		if iface.Bridge != nil {
			result[i] = libvirtxml.DomainInterface{
				Source: &libvirtxml.DomainInterfaceSource{
					Bridge: &libvirtxml.DomainInterfaceSourceBridge{
						Bridge: iface.Bridge.Name,
					},
				},
				Model: &libvirtxml.DomainInterfaceModel{
					Type: defaultInterfaceModel,
				},
			}

			continue
		}

		if iface.Macvtap != nil {
			result[i] = libvirtxml.DomainInterface{
				Source: &libvirtxml.DomainInterfaceSource{
					Direct: &libvirtxml.DomainInterfaceSourceDirect{
						Dev:  iface.Macvtap.Device,
						Mode: string(iface.Macvtap.Mode),
					},
				},
				Model: &libvirtxml.DomainInterfaceModel{
					Type: defaultInterfaceModel,
				},
			}

			continue
		}
	}

	return result
}

// generateDomainDeviceFilesystems generates libvirt filesystem entries from mount file configs.
func (p *provider) generateDomainDeviceFilesystems(mnts []vm.MountFileConfig) []libvirtxml.DomainFilesystem {
	mounts := make([]libvirtxml.DomainFilesystem, len(mnts))
	for i, m := range mnts {
		var ro *libvirtxml.DomainFilesystemReadOnly
		if m.ReadOnly {
			ro = &libvirtxml.DomainFilesystemReadOnly{}
		}

		mount := libvirtxml.DomainFilesystem{
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
			mount.AccessMode = virtiofsSecurityMode
			mount.Driver = &libvirtxml.DomainFilesystemDriver{
				Type:  "virtiofs",
				Queue: virtiofsQueueSize,
			}

			// If insecure read-only mounts are allowed, then disable the
			// read-only setting.
			if p.insecureReadonlyMounts {
				mount.ReadOnly = nil
			}
		}

		mounts[i] = mount
	}

	return mounts
}

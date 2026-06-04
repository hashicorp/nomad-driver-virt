// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"fmt"

	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"libvirt.org/go/libvirtxml"
)

type generateDomainFn func(*vm.Config, *libvirtxml.Domain) error

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
		Devices: &libvirtxml.DomainDeviceList{},
	}

	// The generators are the list of functions to execute to
	// build the full domain configuration.
	generators := []generateDomainFn{
		p.configureDomainProcessors,
		p.configureDomainMemory,
		p.configureDomainOS,
		p.configureDomainPowerManagement,
		p.configureDomainFeatures,
		p.configureDomainDeviceConsoles,
		p.configureDomainDeviceConsoles,
		p.configureDomainDeviceChannels,
		p.generateDomainDeviceDisks,
		p.generateDomainDeviceFilesystems,
		p.generateDomainDeviceInterfaces,
		p.configureDomainDeviceRNG,
	}

	// Run all the generators.
	for _, genFn := range generators {
		if err := genFn(config, domain); err != nil {
			return "", fmt.Errorf("failed to create domain: %w", err)
		}
	}

	// Marshal the result.
	return domain.Marshal()
}

// configureDomainFeatures configures the domain features.
func (p *provider) configureDomainFeatures(config *vm.Config, dom *libvirtxml.Domain) error {
	dom.Features = &libvirtxml.DomainFeatureList{
		VMPort: &libvirtxml.DomainFeatureState{
			State: "off",
		},
	}

	return nil
}

// configureDomainPowerManagement configures the domain power management settings.
func (p *provider) configureDomainPowerManagement(config *vm.Config, dom *libvirtxml.Domain) error {
	dom.PM = &libvirtxml.DomainPM{
		SuspendToMem: &libvirtxml.DomainPMPolicy{
			Enabled: "no",
		},
		SuspendToDisk: &libvirtxml.DomainPMPolicy{
			Enabled: "no",
		},
	}

	return nil
}

// configureDomainOS configures the domain OS settings.
func (p *provider) configureDomainOS(config *vm.Config, dom *libvirtxml.Domain) error {
	guestCaps, err := p.findGuestCaps(config)
	if err != nil {
		return err
	}

	osType := &libvirtxml.DomainOSType{
		Type: guestCaps.OSType,
	}

	if config.OsVariant != nil {
		osType.Arch = config.OsVariant.Arch
		osType.Machine = config.OsVariant.Machine
	}

	dom.OS = &libvirtxml.DomainOS{Type: osType}

	return nil
}

// configureDomainMemory configures the domain memory settings.
func (p *provider) configureDomainMemory(config *vm.Config, dom *libvirtxml.Domain) error {
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

	return nil
}

// configureDomainProcessors configures the domain CPU settings.
func (p *provider) configureDomainProcessors(config *vm.Config, dom *libvirtxml.Domain) error {
	dom.VCPU = &libvirtxml.DomainVCPU{
		Placement: "static",
		Value:     config.CPUs,
		CPUSet:    config.CPUset,
	}

	return nil
}

// configureDomainDeviceChannels configures the domain channel devices.
// NOTE: This is stubbed for future use with provider specific configuration.
func (p *provider) configureDomainDeviceChannels(config *vm.Config, dom *libvirtxml.Domain) error {
	if dom.Devices == nil {
		dom.Devices = &libvirtxml.DomainDeviceList{}
	}

	dom.Devices.Channels = []libvirtxml.DomainChannel{
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

	return nil
}

// configureDomainDeviceRNG configures the domain random number generator devices.
func (p *provider) configureDomainDeviceRNG(config *vm.Config, dom *libvirtxml.Domain) error {
	if dom.Devices == nil {
		dom.Devices = &libvirtxml.DomainDeviceList{}
	}

	dom.Devices.RNGs = []libvirtxml.DomainRNG{
		{
			Model: "virtio",
			Backend: &libvirtxml.DomainRNGBackend{
				Random: &libvirtxml.DomainRNGBackendRandom{
					Device: "/dev/urandom",
				},
			},
		},
	}

	return nil
}

// configureDomainDeviceConsoles configures the domain console devices.
func (p *provider) configureDomainDeviceConsoles(config *vm.Config, dom *libvirtxml.Domain) error {
	if dom.Devices == nil {
		dom.Devices = &libvirtxml.DomainDeviceList{}
	}

	dom.Devices.Consoles = []libvirtxml.DomainConsole{
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

	return nil
}

// generateDomainDeviceDisks configures disks from storage volumes.
func (p *provider) generateDomainDeviceDisks(config *vm.Config, dom *libvirtxml.Domain) error {
	if dom.Devices == nil {
		dom.Devices = &libvirtxml.DomainDeviceList{}
	}

	vols := config.Volumes
	result := make([]libvirtxml.DomainDisk, len(vols))
	for i, fd := range vols {
		disk, err := p.storage.VolumeToDisk(fd)
		if err != nil {
			return err
		}

		// If this is the primary device, mark to boot
		if fd.Primary {
			disk.Boot = &libvirtxml.DomainDeviceBoot{Order: 1}
		}

		result[i] = *disk
	}

	dom.Devices.Disks = result

	return nil
}

// generateDomainNetInterfaces configures interfaces from network interface configs.
func (p *provider) generateDomainDeviceInterfaces(config *vm.Config, dom *libvirtxml.Domain) error {
	if dom.Devices == nil {
		dom.Devices = &libvirtxml.DomainDeviceList{}
	}

	ifaces := config.NetworkInterfaces
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

	dom.Devices.Interfaces = result

	return nil
}

// generateDomainDeviceFilesystems configures filesystem entries from mount file configs.
func (p *provider) generateDomainDeviceFilesystems(config *vm.Config, dom *libvirtxml.Domain) error {
	if dom.Devices == nil {
		dom.Devices = &libvirtxml.DomainDeviceList{}
	}

	mnts := config.Mounts
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

		if m.Driver == MountFsVirtiofs.String() {
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

	dom.Devices.Filesystems = mounts

	return nil
}

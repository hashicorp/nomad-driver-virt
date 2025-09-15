package disks

import (
	"github.com/hashicorp/nomad-driver-virt/virt/disks"
	"libvirt.org/go/libvirtxml"
	"slices"
	"strconv"
)

// ParseDisks parses DisksConfig into a list of libvirtxml DomainDisk
func ParseDisks(disks *disks.DisksConfig) []libvirtxml.DomainDisk {
	if disks == nil {
		return []libvirtxml.DomainDisk{}
	}

	return slices.Concat(
		ParseFileDisks(disks.FileDisksConfig),
		ParseRdbDisks(disks.RbdDisksConfig),
	)
}

// ParseFileDisks parses a map of FileDiskConfigs into libvirtxml DomainDisks
func ParseFileDisks(fds *disks.FileDisksConfig) []libvirtxml.DomainDisk {
	if fds == nil {
		return []libvirtxml.DomainDisk{}
	}

	result := make([]libvirtxml.DomainDisk, len(*fds))
	i := 0
	for label, fd := range *fds {
		result[i] = libvirtxml.DomainDisk{
			Device: "disk",
			Driver: &libvirtxml.DomainDiskDriver{
				Name: "qemu",
				Type: fd.Fmt,
			},
			Source: &libvirtxml.DomainDiskSource{
				File: &libvirtxml.DomainDiskSourceFile{
					File: fd.Path,
				},
			},
			Target: &libvirtxml.DomainDiskTarget{
				Dev: label,
				Bus: "virtio",
			},
		}
		i = i + 1
	}
	return result
}

// ParseNetworkHosts parses a list of FileDiskConfigs into libvirtxml DomainDisks
func ParseNetworkHosts(nhs *disks.NetworkHostsConfig) []libvirtxml.DomainDiskSourceHost {
	if nhs == nil {
		return []libvirtxml.DomainDiskSourceHost{}
	}

	result := make([]libvirtxml.DomainDiskSourceHost, len(*nhs))
	i := 0

	for label, nh := range *nhs {

		host := libvirtxml.DomainDiskSourceHost{
			Name: label,
		}
		if nh.Port > 0 {
			host.Port = strconv.Itoa(nh.Port)
		}
		if nh.Socket != "" {
			host.Socket = nh.Socket
		}
		if nh.Transport != "" {
			host.Transport = nh.Transport
		}

		result[i] = host
		i = i + 1
	}

	return result
}

func ParseRdbAuth(ra *disks.RbdAuthConfig) *libvirtxml.DomainDiskAuth {
	if ra == nil {
		return nil
	}
	return &libvirtxml.DomainDiskAuth{
		Username: ra.Username,
		Secret: &libvirtxml.DomainDiskSecret{
			Type: "ceph",
			UUID: ra.Uuid,
		},
	}
}

func ParseRdbDisks(rds *disks.RbdDisksConfig) []libvirtxml.DomainDisk {
	if rds == nil {
		return []libvirtxml.DomainDisk{}
	}

	result := make([]libvirtxml.DomainDisk, len(*rds))
	i := 0
	for label, rd := range *rds {
		networkSource := &libvirtxml.DomainDiskSourceNetwork{
			Name:     rd.Name,
			Protocol: "rbd",
			Hosts:    ParseNetworkHosts(rd.NetworkHostsConfig),
		}
		if rd.Config != "" {
			networkSource.Config = &libvirtxml.DomainDiskSourceNetworkConfig{
				File: rd.Config,
			}
		}
		if rd.Snapshot != "" {
			networkSource.Snapshot = &libvirtxml.DomainDiskSourceNetworkSnapshot{
				Name: rd.Snapshot,
			}
		}
		networkSource.Auth = ParseRdbAuth(rd.RbdAuthConfig)

		result[i] = libvirtxml.DomainDisk{
			Device: "disk",
			Driver: &libvirtxml.DomainDiskDriver{
				Name: "qemu",
				Type: rd.Fmt,
			},
			Source: &libvirtxml.DomainDiskSource{
				Network: networkSource,
			},
			Target: &libvirtxml.DomainDiskTarget{
				Dev: label,
				Bus: "virtio",
			},
		}
		i = i + 1
	}
	return result
}

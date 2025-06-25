// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package domain

import "libvirt.org/go/libvirtxml"

type FileDisk struct {
	Fmt  string
	Path string
}
type FileDisks map[string]*FileDisk

func (fds FileDisks) ToVirt() []libvirtxml.DomainDisk {
	result := make([]libvirtxml.DomainDisk, len(fds))
	i := 0
	for label, fd := range fds {
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

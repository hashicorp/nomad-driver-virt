// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"fmt"
	"strings"

	domain "github/hashicorp/nomad-driver-virt/internal/shared"
)

type cloudinitConfig struct {
	metadataPath string
	userDataPath string
}

func (d *driver) parceVirtInstallArgs(dc *domain.Config, ci *cloudinitConfig) []string {

	args := []string{
		"--debug",
		fmt.Sprintf("--connect=%s", d.uri),
		fmt.Sprintf("--name=%s", dc.Name),
		fmt.Sprintf("--ram=%d", dc.Memory),
		fmt.Sprintf("--vcpus=%d,cores=%d", dc.CPUs, dc.Cores),
		fmt.Sprintf("--os-variant=%s", dc.OsVariant),
		"--import", "--disk", fmt.Sprintf("path=%s,format=%s", dc.CloudImgPath, dc.DiskFmt),
		"--cloud-init", fmt.Sprintf("user-data=%s,meta-data=%s,disable=on", ci.userDataPath, ci.metadataPath),
		"--noautoconsole",
	}

	if dc.CloudInit.Enable {
		args = append(args, "--cloud-init", fmt.Sprintf("user-data=%s,meta-data=%s,disable=on", ci.userDataPath, ci.metadataPath))
	}

	for _, ni := range dc.NetworkInterfaces {
		args = append(args, "--network", fmt.Sprintf("bridge=%s,model=virtio", ni))
	}

	if len(dc.Mounts) > 0 {

		args = append(args, "--memorybacking=source.type=memfd,access.mode=shared")

		for _, m := range dc.Mounts {
			mArgs := []string{
				m.Source,
				m.Tag,
				"driver.type=virtiofs",
			}

			if m.AccessMode != "" {
				mArgs = append(mArgs, fmt.Sprintf("accessmode=%s", m.AccessMode))
			}

			args = append(args, fmt.Sprintf("--filesystem=%s", strings.Join(mArgs, ",")))
		}
	}

	return args
}

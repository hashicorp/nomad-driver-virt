// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/hashicorp/go-set/v3"
	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	"libvirt.org/go/libvirtxml"
)

type MountFilesystem string

func (m MountFilesystem) String() string {
	return string(m)
}

const (
	MountFsVirtiofs MountFilesystem = "vhost-user-fs-device"
	MountFs9p       MountFilesystem = "virtio-9p-device"
)

// Capabilities contains the host and guests capabilities as reported
// by libvirt.
type Capabilities struct {
	Host   *libvirtxml.CapsHost
	Guests map[string]*CapsGuest
}

// Guest looks up the guest capabilities from the provided architecture.
func (c *Capabilities) Guest(arch string) (*CapsGuest, error) {
	guest, ok := c.Guests[arch]
	if !ok {
		return nil, fmt.Errorf("architecture %q domain capabilities %w", arch, errs.ErrNotFound)
	}

	return guest, nil
}

// CapsGuest wraps the libvirtxml.CapsGuest to include the set of available
// mount filesystems.
type CapsGuest struct {
	*libvirtxml.CapsGuest
	MountFilesystems set.Collection[MountFilesystem]
}

// LoadMountFilesystems will collect the supported filesystems for host
// mounts into the Virtual Machine. These cannot be determined from the
// domain capabilities in libvirt so the emulator is checked directly.
func (c *CapsGuest) LoadMountFilesystems() error {
	c.MountFilesystems = set.New[MountFilesystem](0)

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(c.Arch.Emulator, "-device", "?")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to inspect emulator devices: %w", err)
	}

	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, fmt.Sprintf(`name "%s"`, MountFs9p)) {
			c.MountFilesystems.Insert(MountFs9p)
		}
		if strings.HasPrefix(line, fmt.Sprintf(`name "%s"`, MountFsVirtiofs)) {
			c.MountFilesystems.Insert(MountFsVirtiofs)
		}
	}

	return nil
}

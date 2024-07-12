// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
)

var (
	ErrEmptyName = errors.New("domain name can't be emtpy")
)

type cloudinitConfig struct {
	metadataPath string
	userDataPath string
}

type Users struct {
	IncludeDefault bool
	Users          []UserConfig
}

type File struct {
	Path        string
	Content     string
	Permissions string
	Owner       string
	Group       string
}

type UserGroups []string

func (ug UserGroups) Join() string {
	return strings.Join(ug, ", ")
}

type UserConfig struct {
	Name     string
	Password string
	SSHKeys  []string
	Sudo     string
	Groups   UserGroups
	Shell    string
}

type MountFileConfig struct {
	Source      string
	Destination string
	ReadOnly    bool
	AccessMode  string
	Tag         string
}

type CloudInit struct {
	Enable          bool
	ProvideUserData bool
	UserDataPath    string
	MetaDataPath    string
}

type DomainConfig struct {
	XMLConfig         string
	Name              string
	Memory            int
	Cores             int
	CPUs              int
	OsVariant         string
	CloudInit         CloudInit
	CloudImgPath      string
	DiskFmt           string
	NetworkInterfaces []string
	Type              string
	HostName          string
	UsersConfig       Users
	Files             []File
	EnvVariables      map[string]string
	RemoveConfigFiles bool
	Timezone          *time.Location
	Mounts            []MountFileConfig
	CMD               []string
}

func (d *driver) parceVirtInstallArgs(dc *DomainConfig, ci *cloudinitConfig) []string {

	args := []string{
		"--debug",
		fmt.Sprintf("--connect=%s", d.uri),
		fmt.Sprintf("--name=%s", dc.Name),
		fmt.Sprintf("--ram=%d", dc.Memory),
		fmt.Sprintf("--vcpus=%d,cores=%d", dc.CPUs, dc.Cores),
		fmt.Sprintf("--os-variant=%s", dc.OsVariant),
		"--import", "--disk", fmt.Sprintf("path=%s,format=%s", dc.CloudImgPath, dc.DiskFmt),
		"--graphics", "vnc,listen=0.0.0.0",
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

	fmt.Println(args)
	return args
}

func (dc *DomainConfig) Validate() error {
	var mErr multierror.Error
	if dc.Name == "" {
		return ErrEmptyName
	}

	return mErr.ErrorOrNil()
}

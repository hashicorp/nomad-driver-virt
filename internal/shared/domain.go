// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package domain

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
)

const (
	// mimMemoryMB   = 25600 // Minimum recommended for running linux distributions.
	minMemoryMB   = 500 // 500MB; Minimum size of AWS instance type.
	maxNameLength = 63  // According to RFC 1123 (https://www.rfc-editor.org/rfc/rfc1123.html) should be at most 63 characters
)

var (
	// matches valid DNS labels according to RFC 1123 (https://www.rfc-editor.org/rfc/rfc1123.html),
	// should be at most 63 characters according to the RFC
	validLabel = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)

	ErrEmptyName           = errors.New("domain name can not be empty")
	ErrMissingImage        = errors.New("image path can not be empty")
	ErrNotEnoughMemory     = errors.New("not enough memory assigned to task")
	ErrNotEnoughDisk       = errors.New("not enough disk space assigned to task")
	ErrIncompleteOSVariant = errors.New("provided os information is incomplete: arch and machine are mandatory ")
	ErrInvalidHostName     = fmt.Errorf("a resource name must consist of lower case alphanumeric characters or '-', must start and end with an alphanumeric character and be less than %d characters", maxNameLength+1)
	ErrPathNotAllowed      = fmt.Errorf("base_image is not in the allowed paths")
)

type File struct {
	Path        string
	Content     string
	Permissions string
	Encoding    string
	Owner       string
	Group       string
}

type MountFileConfig struct {
	Source      string
	Destination string
	ReadOnly    bool
	Tag         string
}

type OSVariant struct {
	Arch    string
	Machine string
}

type Config struct {
	RemoveConfigFiles bool
	XMLConfig         string
	Name              string
	Memory            uint
	// Cores             uint // unused?
	CPUs              int
	OsVariant         *OSVariant
	BackingStore      string
	BaseImage         string
	DiskFmt           string
	NetworkInterfaces []NetworkInterfaceConfig
	HostName          string
	Timezone          *time.Location
	Mounts            []MountFileConfig
	Files             []File
	SSHKey            string
	Password          string
	CMDs              []string
	BOOTCMDs          []string
	CIUserData        string
	CIUserDataPath    string
}

type NetworkInterfaceConfig struct {
	Name  string // required
	MAC   string // optional;<random>
	Model string // optional;virtio
}

func (dc *Config) Validate(allowedPaths []string) error {
	var mErr *multierror.Error
	if dc.Name == "" {
		mErr = multierror.Append(mErr, ErrEmptyName)
	}

	if dc.BaseImage == "" {
		mErr = multierror.Append(mErr, ErrMissingImage)
	} else {
		if !isAllowedImagePath(allowedPaths, dc.BaseImage) {
			mErr = multierror.Append(mErr, ErrPathNotAllowed)
		}
	}

	if dc.CPUs < 1 {
		mErr = multierror.Append(mErr, errors.New("no cpus configured"))
	}

	if dc.Memory < minMemoryMB {
		mErr = multierror.Append(mErr, ErrNotEnoughMemory)
	}

	if dc.OsVariant != nil {
		if dc.OsVariant.Arch == "" &&
			dc.OsVariant.Machine == "" {
			mErr = multierror.Append(mErr, ErrIncompleteOSVariant)
		}

	}

	if dc.HostName != "" && !IsValidLabel(dc.HostName) {
		mErr = multierror.Append(mErr, ErrInvalidHostName)
	}

	if dc.CIUserData != "" && dc.CIUserDataPath != "" {
		mErr = multierror.Append(mErr, errors.New(
			"cannot specify both user_data and user_data_path",
		))
	}

	return mErr.ErrorOrNil()
}

type NetworkInterface struct {
	NetworkName string
	Address     string
	PortMap     map[string]string
}

type VirtualizerInfo struct {
	Model           string
	Memory          uint64
	FreeMemory      uint64
	Cpus            uint
	Cores           uint32 // unused?
	EmulatorVersion uint32
	LibvirtVersion  uint32
	RunningDomains  uint
	InactiveDomains uint
	StoragePools    uint
}

type Info struct {
	State     string
	Memory    uint64
	CPUTime   uint64
	MaxMemory uint64
	NrVirtCPU uint
}

// IsValidLabel returns true if the string given is a valid DNS label (RFC 1123).
// Note: the only difference between RFC 1035 and RFC 1123 labels is that in
// RFC 1123 labels can begin with a number.
func IsValidLabel(name string) bool {
	return validLabel.MatchString(name)
}

// ValidateHostName returns an error a name is not a valid resource name.
// The error will contain reference to what constitutes a valid resource name.
func ValidateHostName(name string) error {
	if !IsValidLabel(name) || strings.ToLower(name) != name || len(name) > maxNameLength {
		return ErrInvalidHostName
	}

	return nil
}

func isParent(parent, path string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && !strings.HasPrefix(rel, "..")
}

func isAllowedImagePath(allowedPaths []string, imagePath string) bool {
	for _, ap := range allowedPaths {
		if isParent(ap, imagePath) {
			return true
		}
	}

	return false
}

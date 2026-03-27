// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package vm

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/nomad-driver-virt/cloudinit"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"github.com/hashicorp/nomad-driver-virt/virt/disks"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/drivers"
)

const (
	minMemoryMB   = 500
	maxNameLength = 63 // According to RFC 1123 (https://www.rfc-editor.org/rfc/rfc1123.html) should be at most 63 characters

	// FingerprintAttributeKeyPrefix is the key prefix to use when creating and
	// adding attributes during the fingerprint process.
	FingerprintAttributeKeyPrefix = "driver.virt"
)

var (
	// matches valid DNS labels according to RFC 1123 (https://www.rfc-editor.org/rfc/rfc1123.html),
	// should be at most 63 characters according to the RFC
	validLabel = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)

	ErrEmptyName            = errors.New("virtual machine name can not be empty")
	ErrMissingImage         = errors.New("image path can not be empty")
	ErrNotEnoughDisk        = errors.New("not enough disk space assigned to task")
	ErrNoCPUS               = errors.New("no cpus configured, use resources.cores to assign cores in the job spec")
	ErrNotEnoughMemory      = errors.New("not enough memory assigned to task")
	ErrIncompleteOSVariant  = errors.New("provided os information is incomplete: arch and machine are mandatory ")
	ErrInvalidHostName      = fmt.Errorf("a resource name must consist of lower case alphanumeric characters or '-', must start and end with an alphanumeric character and be less than %d characters", maxNameLength+1)
	ErrPathNotAllowed       = disks.ErrDisallowedPath
	ErrNotFound             = errors.New("not found")
	ErrNotImplemented       = errors.New("not implemented")
	ErrNotSupported         = errors.New("feature is not supported")
	ErrInvalidConfiguration = errors.New("invalid configuration")
)

type VMState string

func (v VMState) ToTaskState() drivers.TaskState {
	switch v {
	case VMStateStarting, VMStateRunning:
		return drivers.TaskStateRunning
	case VMStateShutdown, VMStatePowerOff, VMStateError:
		return drivers.TaskStateExited
	default:
		return drivers.TaskStateUnknown
	}
}

const (
	VMStateStarting  = VMState("starting")
	VMStateRunning   = VMState("running")
	VMStateShutdown  = VMState("shutdown")
	VMStatePowerOff  = VMState("poweroff")
	VMStateSuspended = VMState("suspended")
	VMStatePaused    = VMState("paused")
	VMStateError     = VMState("error")
	VMStateUnknown   = VMState("unknown")
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
	Driver      string
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
	CPUset            string
	CPUs              uint
	OsVariant         *OSVariant
	HostName          string
	Timezone          *time.Location
	Mounts            []MountFileConfig
	Files             []File
	SSHKey            string
	Password          string
	CMDs              []string
	BOOTCMDs          []string
	CIUserData        string
	Volumes           []storage.Volume
	NetworkInterfaces net.NetworkInterfacesConfig
}

// Validate validates the configuration.
func (vm *Config) Validate() error {
	var mErr *multierror.Error
	if vm.Name == "" {
		mErr = multierror.Append(mErr, ErrEmptyName)
	}

	if vm.Memory < minMemoryMB {
		mErr = multierror.Append(mErr, ErrNotEnoughMemory)
	}

	if vm.OsVariant != nil {
		if vm.OsVariant.Arch == "" &&
			vm.OsVariant.Machine == "" {
			mErr = multierror.Append(mErr, ErrIncompleteOSVariant)
		}
	}

	if vm.CPUs < 1 {
		mErr = multierror.Append(mErr, ErrNoCPUS)
	}

	if vm.HostName != "" && !IsValidLabel(vm.HostName) {
		mErr = multierror.Append(mErr, ErrInvalidHostName)
	}

	if err := vm.NetworkInterfaces.Validate(); err != nil {
		mErr = multierror.Append(mErr, err)
	}

	return mErr.ErrorOrNil()
}

// Copy makes a deep copy of the configuration.
func (vm *Config) Copy() *Config {
	copy := &Config{
		RemoveConfigFiles: vm.RemoveConfigFiles,
		XMLConfig:         vm.XMLConfig,
		Name:              vm.Name,
		Memory:            vm.Memory,
		CPUset:            vm.CPUset,
		CPUs:              vm.CPUs,
		NetworkInterfaces: slices.Clone(vm.NetworkInterfaces),
		HostName:          vm.HostName,
		Mounts:            slices.Clone(vm.Mounts),
		Files:             slices.Clone(vm.Files),
		SSHKey:            vm.SSHKey,
		Password:          vm.Password,
		CMDs:              slices.Clone(vm.CMDs),
		BOOTCMDs:          slices.Clone(vm.BOOTCMDs),
		CIUserData:        vm.CIUserData,
	}

	if vm.OsVariant != nil {
		copy.OsVariant = &OSVariant{
			Arch:    vm.OsVariant.Arch,
			Machine: vm.OsVariant.Machine,
		}
	}

	if vm.Timezone != nil {
		*copy.Timezone = *vm.Timezone
	}

	return copy
}

// CloudInitConfig generates the cloud-init configuration from
// the configuration.
func (vm *Config) CloudInitConfig() *cloudinit.Config {
	ms := []cloudinit.MountFileConfig{}
	for _, m := range vm.Mounts {
		mount := cloudinit.MountFileConfig{
			Destination: m.Destination,
			Tag:         m.Tag,
		}

		ms = append(ms, mount)
	}

	fs := []cloudinit.File{}
	for _, f := range vm.Files {
		file := cloudinit.File{
			Path:        f.Path,
			Content:     f.Content,
			Permissions: f.Permissions,
			Encoding:    f.Encoding,
			Owner:       f.Owner,
			Group:       f.Group,
		}

		fs = append(fs, file)
	}

	return &cloudinit.Config{
		MetaData: cloudinit.MetaData{
			InstanceID:    vm.Name,
			LocalHostname: vm.HostName,
		},
		VendorData: cloudinit.VendorData{
			BootCMD:  vm.BOOTCMDs,
			RunCMD:   vm.CMDs,
			Mounts:   ms,
			Files:    fs,
			Password: vm.Password,
			SSHKey:   vm.SSHKey,
		},
		UserData: vm.CIUserData,
	}
}

type NetworkInterface struct {
	NetworkName string
	DeviceName  string
	MAC         string
	Addrs       []netip.Addr
	Model       string
	Driver      string
}

type VirtualizerInfo struct {
	Model           string
	Memory          uint64
	FreeMemory      uint64
	Cpus            uint
	Cores           uint32
	EmulatorVersion uint32
	LibvirtVersion  uint32
	RunningDomains  uint
	InactiveDomains uint
	StoragePools    uint
}

type Info struct {
	RawState  string
	State     VMState
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

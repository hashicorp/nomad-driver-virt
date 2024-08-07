package domain

import (
	"errors"
	"time"

	"github.com/hashicorp/go-multierror"
)

var (
	ErrEmptyName = errors.New("domain name can't be emtpy")
)

type File struct {
	Path        string
	Content     string
	Permissions string
	Owner       string
	Group       string
}

type MountFileConfig struct {
	Source      string
	Destination string
	ReadOnly    bool
	Tag         string
}

type Config struct {
	RemoveConfigFiles bool
	XMLConfig         string
	Name              string
	Memory            uint
	DiskSize          int
	Cores             uint
	CPUs              int
	OSType            string //Further optimize the guest configuration for a specific operating system variant
	Arch              string //Request a non-native CPU architecture for the guest virtual machine.  If omitted, the host CPU architecture will be used in the guest.
	Machine           string //The machine type to emulate. This will typically not need to be specified for Xen or KVM, but is useful for choosing machine types of more exotic architectures.
	BaseImage         string
	DiskFmt           string // Image format to be used if creating managed storage. For file volumes, this can be 'raw', 'qcow2', 'vmdk', etc.
	NetworkInterfaces []string
	HostName          string
	Timezone          *time.Location
	Mounts            []MountFileConfig
	Files             []File
	SSHKey            string
	Password          string
	CMDs              []string
	BOOTCMDs          []string
	CIUserData        string
}

func (dc *Config) Validate() error {
	var mErr multierror.Error
	if dc.Name == "" {
		return ErrEmptyName
	}

	return mErr.ErrorOrNil()
}

type Info struct {
	Model           string
	Memory          uint64
	FreeMemory      uint64
	Cpus            uint
	Cores           uint32
	EmulatorVersion uint32
	LibvirtVersion  uint32
	RunningDomains  uint
	InactiveDomains uint
}

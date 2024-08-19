package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
)

const (
	mimMemoryMB   = 25600 // Minimun recommended for running linux distributions.
	maxNameLength = 63
)

var (
	// matches valid DNS labels according to RFC 1123, should be at most 63
	// characters according to the RFC
	validLabel = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?$`)

	ErrEmptyName           = errors.New("domain name can't be emtpy")
	ErrMissingImage        = errors.New("image path cant be empty")
	ErrNotEnoughDisk       = errors.New("not enough disk space assigned to task")
	ErrIncompleteOSVariant = errors.New("provided os information is incomplete: arch and machine are mandatory ")
	ErrInvalidHostName     = fmt.Errorf("a resource name must consist of lower case alphanumeric characters or '-', must start and end with an alphanumeric character and be less than %d characters", maxNameLength+1)
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
	DiskSize          int
	Cores             uint
	CPUs              int
	OsVariant         *OSVariant
	BaseImage         string
	DiskFmt           string
	// NetworkInterfaces []NetworkInterface
	NetworkInterfaces []string
	Type              string
	HostName          string
	Timezone          *time.Location
	Mounts            []MountFileConfig
	Env               map[string]string
	Files             []File
	SSHKey            string
	Password          string
	CMDs              []string
	BOOTCMDs          []string
	CIUserData        string
	CIVendorData      string
	CIMetaData        string
}

func (dc *Config) Validate() error {
	var mErr multierror.Error
	if dc.Name == "" {
		_ = multierror.Append(&mErr, ErrEmptyName)
	}

	if dc.BaseImage == "" {
		_ = multierror.Append(&mErr, ErrMissingImage)
	}

	if dc.Memory < mimMemoryMB {
		_ = multierror.Append(&mErr, ErrNotEnoughDisk)
	}

	if dc.OsVariant != nil {
		if dc.OsVariant.Arch == "" &&
			dc.OsVariant.Machine == "" {
			_ = multierror.Append(&mErr, ErrIncompleteOSVariant)
		}

	}

	if dc.HostName != "" && !IsValidLabel(dc.HostName) {
		_ = multierror.Append(&mErr, ErrInvalidHostName)
	}

	return mErr.ErrorOrNil()
}

type NetworkInterface struct {
	NetworkName string
	Address     string
	PortMap     map[string]string
}

type VirttualizerInfo struct {
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

type Info struct {
	State   string
	Memory  uint64
	CpuTime uint64
}

// IsValidLabel returns true if the string given is a valid DNS label (RFC 1123).
// Note: the only difference between RFC 1035 and RFC 1123 labels is that in
// RFC 1123 labels can begin with a number.
func IsValidLabel(name string) bool {
	return validLabel.MatchString(name)
}

// ValidateHostName returns an error a name is not a valid resource name. The
// error will contain reference to what constitutes a valid resource name.
func ValidateHostName(name string) error {
	if !IsValidLabel(name) || strings.ToLower(name) != name || len(name) > maxNameLength {
		return ErrInvalidHostName
	}

	return nil
}

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
	AccessMode  string
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
	OsVariant         string
	BaseImage         string
	DiskFmt           string
	NetworkInterfaces []string
	Type              string
	HostName          string
	Timezone          *time.Location
	Mounts            []MountFileConfig
	Files             []File
	SSHKey            string
	Password          string
	CMDs              []string
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

package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
)

var (
	ErrEmptyName = errors.New("domain name can't be emtpy")
)

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
	Name           string
	Password       string
	PathToPassword string
	SSHKeys        []string
	Sudo           string
	Groups         UserGroups
	Shell          string
}

type MountFileConfig struct {
	Source      string
	Destination string
	ReadOnly    bool
	AccessMode  string
	Tag         string
}

type CloudInit struct {
	RemoveConfigFiles bool
	Enable            bool
	UserDataPath      string
	MetaDataPath      string
}

type Config struct {
	XMLConfig         string
	Name              string
	Memory            uint
	DiskSize          int
	Cores             uint
	CPUs              int
	OsVariant         string
	BaseImage         string
	CloudInit         CloudInit
	DiskFmt           string
	NetworkInterfaces []string
	Type              string
	HostName          string
	UsersConfig       Users
	Files             []File
	EnvVariables      map[string]string
	Timezone          *time.Location
	Mounts            []MountFileConfig
	CMD               []string
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
	Network         string
	IP              string
	RunningDomains  uint
	InactiveDomains uint
}

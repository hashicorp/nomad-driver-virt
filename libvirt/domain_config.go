package libvirt

import (
	"errors"
	"fmt"
	"strings"

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

type DomainConfig struct {
	Name              string
	Metadata          map[string]string
	Memory            int
	Cores             int
	CPUs              int
	OsVariant         string
	CloudImgPath      string
	DiskFmt           string
	NetworkInterface  string
	Type              string
	HostName          string
	UsersConfig       Users
	Files             []File
	EnvVariables      map[string]string
	RemoveConfigFiles bool
}

func metadataAsString(m map[string]string) string {
	meta := []string{}
	for key, value := range m {
		meta = append(meta, fmt.Sprintf("%s=\"%s\"", key, value))
	}

	return strings.Join(meta, ",")
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
		"--network", fmt.Sprintf("bridge=%s,model=virtio", dc.NetworkInterface),
		"--graphics", "vnc,listen=0.0.0.0",
		"--cloud-init", fmt.Sprintf("user-data=%s,meta-data=%s,disable=on", ci.userDataPath, ci.metadataPath),
		"--noautoconsole",
	}

	return args
}

func (dc *DomainConfig) Validate() error {
	var mErr multierror.Error
	if dc.Name == "" {
		return ErrEmptyName
	}

	return mErr.ErrorOrNil()
}

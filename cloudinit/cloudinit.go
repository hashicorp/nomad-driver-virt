// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package cloudinit

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"text/template"

	"github.com/hashicorp/go-hclog"
)

const (
	templateFSRoot = "templates"
)

var (
	//go:embed templates
	templateFS         embed.FS
	vendorDataTemplate = "vendor-data.tmpl"
	userDataTemplate   = "user-data.tmpl"
	metaDataTemplate   = "meta-data.tmpl"
)

type Config struct {
	VendorData   VendorData
	MetaData     MetaData
	UserData     string
	UserDataPath string
}

type MetaData struct {
	InstanceID    string
	LocalHostname string
}

type VendorData struct {
	// The password and SSHKeys will be added to the default user of the cloud image
	// distribution.
	Password string
	SSHKey   string
	// RunCMD will be run inside the VM once it has started.
	RunCMD []string
	// BootCMD will be run inside the VM at booting time.
	BootCMD []string
	Mounts  []MountFileConfig
	Files   []File
}

type File struct {
	Path        string
	Content     string
	Permissions string
	Encoding    string
	Owner       string
	Group       string
}

type MountFileConfig struct {
	Destination string
	Tag         string
}

type Controller struct {
	logger hclog.Logger
}

func NewController(logger hclog.Logger) (*Controller, error) {
	c := &Controller{
		logger: logger.Named("cloud-init"),
	}

	return c, nil
}

// Apply takes the cloud init configuration and writes it into an iso (ISO-9660) disk.
// In order for cloud init to pick it up, the meta-data, user-data and vendor-data
// files need to be in the root of the disk, and it needs to be labeled with
// "cidata".
func (c *Controller) Apply(ci *Config, ciPath string) error {
	c.logger.Debug("creating ci config with", fmt.Sprintf("%+v", ci), "in", ciPath)

	mdb := &bytes.Buffer{}
	err := executeTemplate(ci, metaDataTemplate, mdb)
	if err != nil {
		return fmt.Errorf("cloudinit: unable to execute meta data template %s: %w",
			ci.MetaData.InstanceID, err)
	}

	c.logger.Debug("meta-data", mdb.String())

	vdb := &bytes.Buffer{}
	err = executeTemplate(ci, vendorDataTemplate, vdb)
	if err != nil {
		return fmt.Errorf("cloudinit: unable to execute vendor data template %s: %w",
			ci.MetaData.InstanceID, err)
	}

	c.logger.Debug("vendor-data", vdb.String())

	var userDataString string
	{
		switch {
		case ci.UserData != "":
			userDataString = ci.UserData
		case ci.UserDataPath != "":
			b, err := os.ReadFile(ci.UserDataPath)
			if err != nil {
				return fmt.Errorf("cloudinit: unable to open user data file %s: %w",
					ci.MetaData.InstanceID, err)
			}
			userDataString = string(b)
		default:
			var buf bytes.Buffer
			if err := executeTemplate(ci, userDataTemplate, &buf); err != nil {
				return fmt.Errorf("cloudinit: unable to execute user data template %s: %w",
					ci.MetaData.InstanceID, err)
			}
			userDataString = buf.String()
		}
	}

	// TODO: Verify the provided user data is valid, otherwise cloudinit will
	// fail to pick up the vendor data as well, since they are merged into
	// one big file inside the VM.
	var udb io.Reader = strings.NewReader(userDataString)

	l := []Entry{
		{
			Path:   "/meta-data",
			Reader: mdb,
		},
		{
			Path:   "/user-data",
			Reader: udb,
		},
		{
			Path:   "/vendor-data",
			Reader: vdb,
		},
	}

	if err := Write(ciPath, "cidata", l); err != nil {
		return fmt.Errorf("cloudinit: unable to write configuration to file %s: %w",
			ci.MetaData.InstanceID, err)
	}

	return nil
}

func executeTemplate(config *Config, in string, out io.Writer) error {
	fsys, err := fs.Sub(templateFS, templateFSRoot)
	if err != nil {
		return fmt.Errorf("cloudinit: unable to get templates fs %s: %w",
			config.MetaData.InstanceID, err)
	}

	tmpl, err := template.ParseFS(fsys, in)
	if err != nil {
		return fmt.Errorf("cloudinit: unable to parse template %s: %w",
			config.MetaData.InstanceID, err)
	}

	err = tmpl.Execute(out, config)
	if err != nil {
		return fmt.Errorf("cloudinit: unable to execute template %s: %w",
			config.MetaData.InstanceID, err)
	}
	return nil
}

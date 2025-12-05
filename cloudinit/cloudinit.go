// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package cloudinit

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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

	validFilenamePattern = `^[^<>:"/\\|?*\x00-\x1F]+$`
)

type Config struct {
	VendorData VendorData
	MetaData   MetaData
	UserData   string
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
	logger          hclog.Logger
	fileNamePattern *regexp.Regexp
}

func NewController(logger hclog.Logger) (*Controller, error) {
	re := regexp.MustCompile(validFilenamePattern)
	c := &Controller{
		logger:          logger.Named("cloud-init"),
		fileNamePattern: re,
	}

	return c, nil
}

// isValidFilePathSyntax checks if the string has a valid file path syntax.
func (c *Controller) isValidFilePathSyntax(filePath string) bool {
	if !filepath.IsAbs(filePath) && filePath == "" {
		return false
	}

	_, fileName := filepath.Split(filePath)
	if fileName == "" {
		return false
	}

	// On Unix-based systems, the only invalid character in a file name is '/'
	if !c.fileNamePattern.MatchString(fileName) {
		return false
	}

	return true
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

	c.logger.Debug("meta-data", "contents", mdb.String())

	vdb := &bytes.Buffer{}
	err = executeTemplate(ci, vendorDataTemplate, vdb)
	if err != nil {
		return fmt.Errorf("cloudinit: unable to execute vendor data template %s: %w",
			ci.MetaData.InstanceID, err)
	}

	c.logger.Debug("vendor-data", "contents", vdb.String())

	var udb io.ReadWriter
	// TODO: Verify the provided user data is valid, otherwise cloudinit will
	// fail to pick up the vendor data as well, since they are merged into
	// one big file inside the VM.
	if ci.UserData != "" {
		if c.isValidFilePathSyntax(ci.UserData) {
			udf, err := os.Open(ci.UserData)
			if err != nil {
				return fmt.Errorf("cloudinit: unable to open user data file %s: %w",
					ci.MetaData.InstanceID, err)
			}
			defer udf.Close()

			udb = udf
		} else {
			// If the provided userdata is not a path, asume it is a string containing the userdata.
			udb = bytes.NewBufferString(ci.UserData)
		}
	} else {
		udb = &bytes.Buffer{}
		err = executeTemplate(ci, userDataTemplate, udb)
		if err != nil {
			return fmt.Errorf("cloudinit: unable to execute user data template %s: %w",
				ci.MetaData.InstanceID, err)
		}
	}

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

	err = Write(ciPath, "cidata", l)
	if err != nil {
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

package cloudinit

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"

	domain "github/hashicorp/nomad-driver-virt/internal/shared"

	"github.com/hashicorp/go-hclog"
)

const (
	ISOName = "/cidata.iso"
)

var (
	vendorDataTemplate = "/cloudinit/vendor-data.tmpl"
	userDataTemplate   = "/cloudinit/user-data.tmpl"
	metaDataTemplate   = "/cloudinit/meta-data.tmpl"
)

type Controller struct {
	logger hclog.Logger
}

func NewController(logger hclog.Logger) (*Controller, error) {
	c := &Controller{
		logger: logger.Named("cloud-init"),
	}

	return c, nil
}

func (c *Controller) WriteConfigToISO(ci *domain.CloudInit, path string) (string, error) {
	ciPath := filepath.Join(path + ISOName)
	c.logger.Debug("creating ci config with", ci, "in", path)

	mdb := &bytes.Buffer{}
	err := executeTemplate(ci, metaDataTemplate, mdb)
	if err != nil {
		return "", err
	}

	c.logger.Debug("metadata", mdb.String())

	vdb := &bytes.Buffer{}
	err = executeTemplate(ci, vendorDataTemplate, vdb)
	if err != nil {
		return "", err
	}

	c.logger.Debug("vendor data", vdb.String())

	var udb io.ReadWriter
	if ci.UserDataPath != "" {
		udf, err := os.Open(ci.UserDataPath)
		if err != nil {
			return "", err
		}
		defer udf.Close()
		udb = udf

	} else {
		udb = &bytes.Buffer{}
		err = executeTemplate(ci, userDataTemplate, udb)
		if err != nil {
			return "", err
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
		return "", err
	}

	return ciPath, nil
}

func executeTemplate(config *domain.CloudInit, in string, out io.Writer) error {
	pwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("libvirt: unable to get path: %w", err)
	}

	tmpl, err := template.ParseFiles(pwd + in)
	if err != nil {
		return fmt.Errorf("libvirt: unable to parse template: %w", err)
	}

	err = tmpl.Execute(out, config)
	if err != nil {
		return fmt.Errorf("libvirt: unable to execute template: %w", err)
	}
	return nil
}

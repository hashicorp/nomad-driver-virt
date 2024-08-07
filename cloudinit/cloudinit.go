package cloudinit

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"text/template"

	domain "github/hashicorp/nomad-driver-virt/internal/shared"

	"github.com/hashicorp/go-hclog"
)

//go:embed templates
var templateFS embed.FS

const (
	templateFSRoot = "templates"
	ISOName        = "/cidata.iso"
)

var (
	vendorDataTemplate = "vendor-data.tmpl"
	userDataTemplate   = "user-data.tmpl"
	metaDataTemplate   = "meta-data.tmpl"
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
	//c.logger.Debug("creating ci config with", fmt.Sprintf("%+v", ci), "in", path)

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
	fsys, err := fs.Sub(templateFS, templateFSRoot)
	if err != nil {
		return fmt.Errorf("cloud-init: unable to get templates fs: %w", err)
	}

	tmpl, err := template.ParseFS(fsys, in)
	if err != nil {
		return fmt.Errorf("cloud-init: unable to parse template: %w", err)
	}

	err = tmpl.Execute(out, config)
	if err != nil {
		return fmt.Errorf("cloud-init: unable to execute template: %w", err)
	}
	return nil
}

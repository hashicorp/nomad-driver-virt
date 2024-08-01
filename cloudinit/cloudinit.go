package cloudinit

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"

	domain "github/hashicorp/nomad-driver-virt/internal/shared"

	"github.com/hashicorp/go-hclog"
)

const (
	userDataTemplate = "/cloudinit/user-data.tmpl"
	metaDataTemplate = "/cloudinit/meta-data.tmpl"
	ISOName          = "/cidata.iso"
)

type Controller struct {
	logger hclog.Logger
}

type Option func(*Controller)

func NewController(logger hclog.Logger) (*Controller, error) {
	c := &Controller{
		logger: logger.Named("cloud-init"),
	}

	return c, nil
}

func (c *Controller) WriteConfigToISO(ci *domain.CloudInit, path string) (string, error) {
	err := c.createAndPopulateCIFiles(ci, path)
	if err != nil {
		return "", err
	}
	writeToISO(path)
	if err != nil {
		return "", err
	}

	return filepath.Join(path + ISOName), nil
}

func (c *Controller) createAndPopulateCIFiles(cic *domain.CloudInit, path string) error {
	c.logger.Debug("creating ci config with", cic)

	mdf, err := os.Create(path + "/meta-data")
	defer mdf.Close()
	if err != nil {
		return fmt.Errorf("libvirt: unable create meta data file: %w", err)
	}

	// TODO: merge user provided cloud init configuration
	err = executeTemplate(cic, metaDataTemplate, mdf)
	if err != nil {
		return err
	}

	udf, err := os.Create(path + "/user-data")
	defer udf.Close()
	if err != nil {
		return fmt.Errorf("libvirt: unable to create user-data file: %w", err)
	}

	// TODO: merge user provided cloud init configuration
	err = executeTemplate(cic, userDataTemplate, udf)
	if err != nil {
		return err
	}

	return nil
}

func writeToISO(path string) error {
	mdf, err := os.Open(path + "/meta-data")
	if err != nil {
		return err
	}

	defer mdf.Close()

	udf, err := os.Open(path + "/user-data")
	if err != nil {
		return err
	}

	defer udf.Close()

	// Now `file` is an io.Reader
	var readerUdt io.Reader = udf
	var readerMdt io.Reader = mdf

	l := []Entry{
		{
			Path:   "/meta-data",
			Reader: readerMdt,
		},
		{
			Path:   "/user-data",
			Reader: readerUdt,
		},
	}

	return Write(filepath.Join(path+ISOName), "cidata", l)
}

func executeTemplate(config *domain.CloudInit, in string, out *os.File) error {
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

// Function to be implemented
func mergeConfigs(CIFiles ...string) {}

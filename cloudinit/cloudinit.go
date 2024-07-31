package cloudinit

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	domain "github/hashicorp/nomad-driver-virt/internal/shared"

	"github.com/hashicorp/go-hclog"
)

const (
	defaultURI             = "qemu:///system"
	defaultDataDir         = "/usr/local"
	dataDirPermissions     = 0777
	userDataDir            = "/virt"
	userDataDirPermissions = 0777
	userDataTemplate       = "/libvirt/user-data.tmpl"
	metaDataTemplate       = "/libvirt/meta-data.tmpl"
	envFile                = "/etc/profile.d/virt-envs.sh"
)

type cloudinitConfig struct {
	domainDir    string
	metadataPath string
	userdataPath string
}

func dirExists(dirname string) bool {
	info, err := os.Stat(dirname)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func deleteDir(name string) error {
	if dirExists(name) {
		err := os.RemoveAll(name)
		if err != nil {
			return fmt.Errorf("libvirt: failed to delete directory: %w", err)
		}
	}

	return nil
}

type Controller struct {
	logger  hclog.Logger
	dataDir string
}

type Option func(*Controller)

func WithDataDirectory(path string) Option {
	return func(c *Controller) {
		c.dataDir = path
	}
}

func NewController() (*Controller, error) {
	c := &Controller{
		dataDir: defaultDataDir,
	}

	path := filepath.Join(c.dataDir, userDataDir)
	err := os.MkdirAll(path, dataDirPermissions)
	if err != nil {
		return nil, err
	}

	return c, nil
}

func (c *Controller) GetCidataISO(ci domain.CloudInit) (string, error) {
	return "/home/ubuntu/test/cidata.iso", nil
}

func (cic *cloudinitConfig) createAndpopulateUserDataFiles(config *domain.Config) error {
	mdf, err := os.Create(cic.domainDir + "/meta-data")
	defer mdf.Close()
	if err != nil {
		return fmt.Errorf("libvirt: create file: %w", err)
	}

	err = executeTemplate(config, metaDataTemplate, mdf)
	if err != nil {
		return err
	}

	udf, err := os.Create(cic.domainDir + "/user-data")
	defer udf.Close()
	if err != nil {
		return fmt.Errorf("libvirt: create file: %w", err)
	}

	err = executeTemplate(config, userDataTemplate, udf)
	if err != nil {
		return err
	}

	return nil
}

func executeTemplate(config *domain.Config, in string, out *os.File) error {
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

func createDomainFolder(path string) error {
	err := os.MkdirAll(path, userDataDirPermissions)
	if err != nil {
		return err
	}

	return nil
}

func cleanUpDomainFolder(path string) error {
	return deleteDir(path)
}

func blah() {
	/* domainDir := filepath.Join(d.dataDir, userDataDir, config.Name)
	err = createDomainFolder(domainDir)
	if err != nil {
		return fmt.Errorf("libvirt: unable to build domain config: %w", err)
	}

	defer func() {
		if config.RemoveConfigFiles {
			err := cleanUpDomainFolder(domainDir)
			if err != nil {
				d.logger.Error("unable to clean up domain folder", err)
			}
		}
	}()

	ci := &cloudinitConfig{}
	if config.CloudInit.Enable {
		ci.domainDir = domainDir

		err = ci.createAndpopulateUserDataFiles(config)
		if err != nil {
			return err
		}

		ci.userdataPath = ci.domainDir + "/user-data"
		ci.metadataPath = ci.domainDir + "/meta-data"

	} */
}

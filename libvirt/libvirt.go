// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/hashicorp/go-hclog"
	"libvirt.org/go/libvirt"
)

const (
	defaultURI             = "qemu:///system"
	defaultDataDir         = "/home/ubuntu"
	dataDirPermissions     = 0777
	userDataDir            = "/virt"
	userDataDirPermissions = 0777
	userDataTemplate       = "/libvirt/user-data.tmpl"
	metaDataTemplate       = "/libvirt/meta-data.tmpl"
	envFile                = "/etc/profile.d/virt-envs.sh"
)

var (
	ErrEmptyURI     = errors.New("connection URI can't be empty")
	ErrDomainExists = errors.New("the domain exists already")
)

type driver struct {
	uri     string
	conn    *libvirt.Connect
	logger  hclog.Logger
	dataDir string
}

func (d *driver) monitorCtx(ctx context.Context) {
	select {
	case <-ctx.Done():
		d.conn.Close()
		return
	}
}

func validURI(uri string) error {
	if uri == "" {
		return ErrEmptyURI
	}

	return nil
}

type Option func(*driver)

func WithConnectionURI(URI string) Option {
	return func(d *driver) {
		d.uri = URI
	}
}

func WithDataDirectory(path string) Option {
	return func(d *driver) {
		d.dataDir = path
	}
}

func New(ctx context.Context, logger hclog.Logger, options ...Option) (*driver, error) {
	d := &driver{
		logger:  logger.Named("nomad-virt-plugin"),
		uri:     defaultURI,
		dataDir: defaultDataDir,
	}

	for _, opt := range options {
		opt(d)
	}

	path := filepath.Join(d.dataDir, userDataDir)
	err := os.MkdirAll(path, dataDirPermissions)
	if err != nil {
		return nil, err
	}

	conn, err := libvirt.NewConnect(d.uri)
	if err != nil {
		return nil, err
	}

	d.conn = conn

	go d.monitorCtx(ctx)

	return d, nil
}

type Info struct {
	Model           string
	Memory          uint64
	FreeMemory      uint64
	Cpus            uint
	Cores           uint32
	EmulatorVersion uint32
	LibvirtVersion  uint32
}

func (d *driver) GetURI() string {
	return d.uri
}

func (d *driver) GetInfo() (Info, error) {
	li := Info{}

	ni, err := d.conn.GetNodeInfo()
	if err != nil {
		return li, fmt.Errorf("libvirt: unable to get node info: %w", err)
	}

	ev, err := d.conn.GetVersion()
	if err != nil {
		return li, fmt.Errorf("libvirt: unable to get e version: %w", err)
	}

	lv, err := d.conn.GetLibVersion()
	if err != nil {
		return li, fmt.Errorf("libvirt: unable to get lib version: %w", err)
	}

	fm, err := d.conn.GetFreeMemory()
	if err != nil {
		return li, fmt.Errorf("libvirt: unable to free memory: %w", err)
	}

	p, _ := d.conn.GetNodeInfo()
	fmt.Println(p)
	li.Cores = ni.Cores
	li.Memory = ni.Memory
	li.Cpus = ni.Cpus
	li.Model = ni.Model
	li.EmulatorVersion = ev
	li.LibvirtVersion = lv
	li.FreeMemory = fm

	return li, nil
}

func (d *driver) Close() (int, error) {
	return d.conn.Close()
}

func (d *driver) createDomain(dc *DomainConfig, ci *cloudinitConfig) error {
	var outb, errb bytes.Buffer

	args := d.parceVirtInstallArgs(dc, ci)

	cmd := exec.Command("virt-install", args...)
	cmd.Dir = d.dataDir
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("libvirt: %w: %s", err, errb.String())
	}

	d.logger.Debug("logger", outb.String())
	d.logger.Debug("logger", errb.String())

	return nil
}

func executeTemplate(config *DomainConfig, in string, out string) error {
	pwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("libvirt: unable to get path: %w", err)
	}

	tmpl, err := template.ParseFiles(pwd + in)
	if err != nil {
		return fmt.Errorf("libvirt: unable to parse template: %w", err)
	}

	f, err := os.Create(out)
	defer f.Close()

	if err != nil {
		return fmt.Errorf("libvirt: create file: %w", err)
	}

	err = tmpl.Execute(f, config)
	if err != nil {
		return fmt.Errorf("libvirt: unable to execute template: %w", err)
	}
	return nil
}

func createCloudInitFilesFromTmpls(config *DomainConfig, domainDir string) (*cloudinitConfig, error) {

	err := executeTemplate(config, metaDataTemplate, domainDir+"/meta-data")
	if err != nil {
		return nil, err
	}

	err = executeTemplate(config, userDataTemplate, domainDir+"/user-data")
	if err != nil {
		return nil, err
	}

	ci := &cloudinitConfig{
		userDataPath: domainDir + "/user-data",
		metadataPath: domainDir + "/meta-data",
	}

	return ci, nil
}

func dirExists(dirname string) bool {
	info, err := os.Stat(dirname)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func deleteDir(dirname string) error {
	if dirExists(dirname) {
		err := os.RemoveAll(dirname)
		if err != nil {
			return fmt.Errorf("libvirt: failed to delete directory: %w", err)
		}
	}

	return nil
}

// CreateDomain verifies if the domains exists already, if it does, it returns
// an error, otherwise it creates a new domain with the provided configuration.
func (d *driver) CreateDomain(config *DomainConfig) error {
	dom, err := d.conn.LookupDomainByName(config.Name)
	if err != nil {
		return fmt.Errorf("libvirt: unable to verify exiting domains: %w", err)
	}

	if dom != nil {
		return ErrDomainExists
	}

	domainDir := filepath.Join(d.dataDir, userDataDir, config.Name)
	err = os.MkdirAll(domainDir, userDataDirPermissions)
	if err != nil {
		return err
	}

	ci, err := createCloudInitFilesFromTmpls(config, domainDir)
	if err != nil {
		return err
	}
	defer func() {
		if config.RemoveConfigFiles {
			err := deleteDir(config.Name)
			if err != nil {
				d.logger.Error("libvirt: unable to discard user data files after domain creation", err)
			}
		}
	}()

	err = d.createDomain(config, ci)
	if err != nil {
		return err
	}

	_, err = d.conn.LookupDomainByName(config.Name)
	if err != nil {
		return fmt.Errorf("libvirt: unable to verify domain creation: %w", err)
	}

	return nil
}

func (d *driver) GetDomain(name string) (*libvirt.Domain, error) {
	return d.conn.LookupDomainByName(name)
}

func (d *driver) StopDomain(name string) error {
	dom, err := d.conn.LookupDomainByName(name)
	if err != nil {
		return err
	}

	d.logger.Debug("stopping domain", name)

	return dom.ShutdownFlags(libvirt.DOMAIN_SHUTDOWN_SIGNAL)
}

func (d *driver) DestroyDomain(name string) error {
	dom, err := d.conn.LookupDomainByName(name)
	if err != nil {
		return err
	}

	d.logger.Debug("destroying domain", name)

	err = deleteDir(name)
	if err != nil {
		d.logger.Error("unable to discard user data files after domain creation", err)
	}

	return dom.DestroyFlags(libvirt.DOMAIN_DESTROY_GRACEFUL)
}

func (d *driver) GetVms() {
	doms, err := d.conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE)
	if err != nil {
		fmt.Println("errores", err)
	}

	fmt.Printf("%d running domains:\n", len(doms))
	for _, dom := range doms {
		name, err := dom.GetName()
		if err == nil {
			fmt.Printf("  %s\n", name)
		}
		nam, err := dom.GetInfo()
		if err == nil {
			fmt.Printf("  %+v\n", nam)
		}
		dom.Free()
	}
}

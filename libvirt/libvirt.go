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

	domain "github.com/hashicorp/nomad-driver-virt/internal/shared"

	"github.com/hashicorp/go-hclog"
	"libvirt.org/go/libvirt"
	libvirtxml "libvirt.org/libvirt-go-xml"
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

func (d *driver) GetInfo() (domain.Info, error) {
	li := domain.Info{}

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

func (d *driver) createDomainWithVirtInstall(dc *domain.Config, ci *cloudinitConfig) error {
	var outb, errb bytes.Buffer

	args := d.parceConfiguration(dc, ci)

	cmd := exec.Command("virt-install", args...)
	cmd.Dir = d.dataDir
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("libvirt: %w: %s", err, errb.String())
	}

	d.logger.Debug("logger", errb.String())
	d.logger.Info("logger", outb.String())

	return nil
}

// CreateDomain verifies if the domains exists already, if it does, it returns
// an error, otherwise it creates a new domain with the provided configuration.
func (d *driver) CreateDomain(config *domain.Config) error {
	dom, err := d.conn.LookupDomainByName(config.Name)
	if lverr, ok := err.(*libvirt.Error); ok {
		if lverr.Code != libvirt.ERR_NO_DOMAIN {
			return fmt.Errorf("libvirt: unable to verify exiting domains: %w", err)
		}
	}

	if dom != nil {
		return ErrDomainExists
	}

	domainDir := filepath.Join(d.dataDir, userDataDir, config.Name)
	err = createDomainFolder(domainDir)
	if err != nil {
		return fmt.Errorf("libvirt: unable to build domain config: %w", err)
	}

	defer func() {
		err := cleanUpDomainFolder(domainDir)
		if err != nil {
			d.logger.Error("unable to clean up domain folder", err)
		}
	}()

	ci := &cloudinitConfig{}
	if config.CloudInit.Enable {
		ci.domainDir = domainDir

		err = ci.createAndpopulateFiles(config)
		if err != nil {
			return err
		}

		ci.userdataPath = ci.domainDir + "/user-data"
		ci.metadataPath = ci.domainDir + "/meta-data"

	} else {
		ci.metadataPath = config.CloudInit.MetaDataPath
		ci.userdataPath = config.CloudInit.UserDataPath
	}

	err = d.createDomainWithVirtInstall(config, ci)
	if err != nil {
		return err
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

func cd() {
	domcfg := &libvirtxml.Domain{Type: "kvm", Name: "demo",
		UUID: "8f99e332-06c4-463a-9099-330fb244e1b3"}
	xmldoc, err := domcfg.Marshal()
	fmt.Println(xmldoc, err)
}

// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github/hashicorp/nomad-driver-virt/cloudinit"
	domain "github/hashicorp/nomad-driver-virt/internal/shared"

	"github.com/hashicorp/go-hclog"
	"libvirt.org/go/libvirt"
)

const (
	defaultURI         = "qemu:///system"
	defaultDataDir     = "/home/ubuntu/test/virt"
	dataDirPermissions = 0777
)

var (
	ErrDomainExists   = errors.New("the domain exists already")
	ErrDomainNotFound = errors.New("the domain does not exist")
)

type CloudInit interface {
	WriteConfigToISO(ci *domain.CloudInit, path string) (string, error)
}

type driver struct {
	uri      string
	conn     *libvirt.Connect
	logger   hclog.Logger
	user     string
	password string
	ci       CloudInit
	dataDir  string
}

type Option func(*driver)

func WithDataDirectory(path string) Option {
	return func(d *driver) {
		d.dataDir = path
	}
}

func WithConnectionURI(URI string) Option {
	return func(d *driver) {
		d.uri = URI
	}
}

func WithAuth(user, password string) Option {
	return func(d *driver) {
		d.user = user
		d.password = password
	}
}

func WithCIController(ci *cloudinit.Controller) Option {
	return func(d *driver) {
		d.ci = ci
	}
}

func newConnection(uri string, user string, pass string) (*libvirt.Connect, error) {
	if user == "" {
		return libvirt.NewConnect(uri)
	}

	callback := func(creds []*libvirt.ConnectCredential) {
		for _, cred := range creds {
			if cred.Type == libvirt.CRED_AUTHNAME {
				cred.Result = user
				cred.ResultLen = len(cred.Result)
			} else if cred.Type == libvirt.CRED_PASSPHRASE {
				cred.Result = pass
				cred.ResultLen = len(cred.Result)
			}
		}
	}

	auth := &libvirt.ConnectAuth{
		CredType: []libvirt.ConnectCredentialType{
			libvirt.CRED_AUTHNAME, libvirt.CRED_PASSPHRASE,
		},
		Callback: callback,
	}
	virConn, err := libvirt.NewConnectWithAuth(uri, auth, 0)

	return virConn, err
}

func (d *driver) monitorCtx(ctx context.Context) {
	select {
	case <-ctx.Done():
		d.conn.Close()
		return
	}
}

func New(ctx context.Context, logger hclog.Logger, options ...Option) (*driver, error) {
	ci, err := cloudinit.NewController(logger)
	if err != nil {
		return nil, err
	}

	d := &driver{
		logger:  logger.Named("nomad-virt-plugin"),
		uri:     defaultURI,
		ci:      ci,
		dataDir: defaultDataDir,
	}

	for _, opt := range options {
		opt(d)
	}

	conn, err := newConnection(d.uri, d.user, d.password)
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
		return li, fmt.Errorf("libvirt: unable to get free memory: %w", err)
	}

	aDoms, err := d.conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE)
	if err != nil {
		return li, fmt.Errorf("libvirt: unable to get active domains: %w", err)
	}

	iDoms, err := d.conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_INACTIVE)
	if err != nil {
		return li, fmt.Errorf("libvirt: unable to get inactive domains: %w", err)
	}

	li.Cores = ni.Cores
	li.Memory = ni.Memory
	li.Cpus = ni.Cpus
	li.Model = ni.Model
	li.EmulatorVersion = ev
	li.LibvirtVersion = lv
	li.FreeMemory = fm
	li.RunningDomains = uint(len(aDoms))
	li.InactiveDomains = uint(len(iDoms))

	return li, nil
}

func (d *driver) Close() (int, error) {
	return d.conn.Close()
}

func createCloudInitConfig(config *domain.Config) *domain.CloudInit {
	cmds := []string{}
	for _, m := range config.Mounts {

		c := []string{
			fmt.Sprintf("mkdir -p %s", m.Destination),
			fmt.Sprintf("mount -t virtiofs %s %s", m.Tag, m.Destination),
		}
		cmds = append(cmds, c...)
	}

	return &domain.CloudInit{
		MetaData: domain.MetaData{
			InstanceID:    config.Name,
			LocalHostname: config.Name,
		},
		UserData: domain.UserData{
			Users:  config.UsersConfig,
			RunCMD: cmds,
			Mounts: config.Mounts,
		},
	}
}

func createAllocFileMount() domain.MountFileConfig {
	return domain.MountFileConfig{
		Source:      "/home/ubuntu/test/alloc", // TODO: Define how to pass this value
		Tag:         "allocDir",
		Destination: "/alloc",
	}
}

// CreateDomain verifies if the domains exists already, if it does, it returns
// an error, otherwise it creates a new domain with the provided configuration.
func (d *driver) CreateDomain(config *domain.Config) error {
	dom, err := d.GetDomain(config.Name)
	if err != nil {
		return err
	}

	if dom != nil {
		return ErrDomainExists
	}

	err = config.Validate()
	if err != nil {
		return err
	}

	config.Mounts = append(config.Mounts, createAllocFileMount())

	dDir := filepath.Join(d.dataDir, config.Name)
	err = createDomainFolder(dDir)
	if err != nil {
		return fmt.Errorf("libvirt: unable to create domain folder: %w", err)
	}

	defer func() {
		if config.RemoveConfigFiles {
			err := cleanUpDomainFolder(dDir)
			if err != nil {
				d.logger.Info("unable to remove ci config files", err)
			}
		}
	}()

	cic := createCloudInitConfig(config)

	isoPath, err := d.ci.WriteConfigToISO(cic, dDir)
	if err != nil {
		return fmt.Errorf("libvirt: unable to create cidata: %w", err)
	}

	var domXML string
	if config.XMLConfig != "" {
		domXML = config.XMLConfig
	} else {
		domXML, err = parceConfiguration(config, isoPath)
		if err != nil {
			return fmt.Errorf("libvirt: unable to parce domain configuration: %w", err)
		}
	}

	d.logger.Debug("creating domain with", domXML)

	_, err = d.conn.DomainCreateXML(domXML, 0)
	if err != nil {
		return fmt.Errorf("libvirt: unable to parce create domain: %w", err)
	}

	return nil
}

func (d *driver) GetDomain(name string) (*libvirt.Domain, error) {
	dom, err := d.conn.LookupDomainByName(name)
	if err != nil {
		if lverr, ok := err.(*libvirt.Error); ok {
			if lverr.Code != libvirt.ERR_NO_DOMAIN {
				return nil, fmt.Errorf("libvirt: unable to verify exiting domains: %w", err)
			}
		}
	}

	return dom, nil
}

func (d *driver) StopDomain(name string) error {
	dom, err := d.GetDomain(name)
	if err != nil {
		return err
	}

	if dom == nil {
		return ErrDomainNotFound
	}

	d.logger.Debug("stopping domain", name)

	return dom.ShutdownFlags(libvirt.DOMAIN_SHUTDOWN_SIGNAL)
}

func (d *driver) DestroyDomain(name string) error {
	dom, err := d.GetDomain(name)
	if err != nil {
		return err
	}

	if dom == nil {
		return ErrDomainNotFound
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

func cleanUpDomainFolder(path string) error {
	return deleteDir(path)
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

func createDomainFolder(path string) error {
	err := os.MkdirAll(path, dataDirPermissions)
	if err != nil {
		return err
	}

	return nil
}

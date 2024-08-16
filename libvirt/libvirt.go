// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"

	"github/hashicorp/nomad-driver-virt/cloudinit"

	domain "github.com/hashicorp/nomad-driver-virt/internal/shared"

	"github.com/hashicorp/go-hclog"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

const (
	defaultURI               = "qemu:///system"
	defaultVirtualizatioType = "hvm"
	defaultAccelerator       = "kvm"
	defaultSecurityMode      = "mapped" // "passthrough"
	defaultInterfaceModel    = "virtio"
	libvirtVirtioChannel     = "org.qemu.guest_agent.0" // This is is the only channel libvirt will use to connect to the qemu agent.

	defaultDataDir     = "/var/lib/virt"
	dataDirPermissions = 777
	storagePoolName    = "virt-sp"

	envVariblesFilePath        = "/etc/profile.d/virt.sh" //Only valid for linux OS
	envVariblesFilePermissions = "777"
)

var (
	ErrDomainExists   = errors.New("the domain exists already")
	ErrDomainNotFound = errors.New("the domain does not exist")

	nomadDomainStates = map[libvirt.DomainState]string{
		libvirt.DOMAIN_RUNNING:     "running",
		libvirt.DOMAIN_NOSTATE:     "unknown",
		libvirt.DOMAIN_BLOCKED:     "blocked",
		libvirt.DOMAIN_PAUSED:      "paused",
		libvirt.DOMAIN_SHUTDOWN:    "shutdown",
		libvirt.DOMAIN_CRASHED:     "crashed",
		libvirt.DOMAIN_PMSUSPENDED: "pmsuspended",
		libvirt.DOMAIN_SHUTOFF:     "shutoff",
	}
)

type CloudInit interface {
	WriteConfigToISO(ci *cloudinit.CloudInit, path string) error
}

type driver struct {
	uri      string
	conn     *libvirt.Connect
	logger   hclog.Logger
	user     string
	password string
	ci       CloudInit
	dataDir  string
	sp       *libvirt.StoragePool
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

func (d *driver) lookForExistingStoragePool(name string) (*libvirt.StoragePool, error) {
	sps, err := d.conn.ListAllStoragePools(libvirt.CONNECT_LIST_STORAGE_POOLS_ACTIVE)
	if err != nil {
		return nil, err
	}

	for _, sp := range sps {
		n, err := sp.GetName()
		if err != nil {
			return nil, err
		}

		if n == name {
			return &sp, nil
		}
	}

	return nil, nil
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

	d.logger.Debug("setting up data directory", d.dataDir)

	found, err := d.lookForExistingStoragePool(storagePoolName)
	if err != nil {
		return nil, fmt.Errorf("libvirt: unable to list existing storage pools: %w", err)

	}

	if found == nil {
		sp := libvirtxml.StoragePool{
			Name: storagePoolName,
			Target: &libvirtxml.StoragePoolTarget{
				Path: d.dataDir,
			},
			Type: "dir",
		}

		spXML, err := sp.Marshal()
		if err != nil {
			return nil, err
		}

		d.logger.Debug("creating storage pool")
		found, err = d.conn.StoragePoolCreateXML(spXML, 0)
		if err != nil {
			return nil, fmt.Errorf("libvirt: unable to create storage pool: %w", err)

		}
	}

	d.logger.Debug("assigning storage pool")
	d.sp = found

	return d, nil
}

func (d *driver) GetInfo() (domain.VirttualizerInfo, error) {
	li := domain.VirttualizerInfo{}

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

func createEnvsFile(envs map[string]string) cloudinit.File {
	con := []string{}

	for k, v := range envs {
		con = append(con, fmt.Sprintf("export %s=%s", k, v))
	}

	return cloudinit.File{
		Encoding:    "b64",
		Path:        envVariblesFilePath,
		Permissions: envVariblesFilePermissions,
		Content:     base64.StdEncoding.EncodeToString([]byte(strings.Join(con, "\n\t"))),
	}
}

func createCloudInitConfig(config *domain.Config) *cloudinit.CloudInit {

	files := createEnvsFile(config.Env)
	mounts := addCMDsForMounts(config.Mounts)

	return &cloudinit.CloudInit{
		MetaData: cloudinit.MetaData{
			InstanceID:    config.Name,
			LocalHostname: config.HostName,
		},
		VendorData: cloudinit.VendorData{
			BootCMD:  append(config.BOOTCMDs, mounts...),
			RunCMD:   config.CMDs,
			Mounts:   config.Mounts,
			Files:    append(config.Files, files),
			Password: config.Password,
			SSHKey:   config.SSHKey,
		},
		UserDataPath: config.CIUserData,
	}
}

func addCMDsForMounts(mounts []domain.MountFileConfig) []string {
	cmds := []string{}
	for _, m := range mounts {
		c := []string{
			fmt.Sprintf("mkdir -p %s", m.Destination),
			fmt.Sprintf("mountpoint -q %s || mount -t 9p -o trans=virtio %s %s", m.Destination, m.Tag, m.Destination),
		}

		cmds = append(cmds, c...)
	}

	return cmds
}

func (d *driver) getDomain(name string) (*libvirt.Domain, error) {
	dom, err := d.conn.LookupDomainByName(name)
	if err != nil {
		lverr, ok := err.(libvirt.Error)
		if ok {
			if lverr.Code != libvirt.ERR_NO_DOMAIN {
				return nil, fmt.Errorf("libvirt: unable to verify exiting domains: %w", err)
			}
		}
	}

	return dom, nil
}

func (d *driver) StopDomain(name string) error {
	d.logger.Warn("suspending domain", name)
	dom, err := d.getDomain(name)
	if err != nil {
		return err
	}

	if dom == nil {
		return ErrDomainNotFound
	}

	err = dom.Destroy()
	if err != nil {
		return fmt.Errorf("libvirt: unable to shut domain %s: %w", name, err)
	}

	return nil
}

func (d *driver) DestroyDomain(name string) error {
	d.logger.Info("destroying domain", name)
	dom, err := d.getDomain(name)
	if err != nil {
		return err
	}

	if dom == nil {
		return ErrDomainNotFound
	}

	err = dom.Destroy()
	if err != nil {
		return fmt.Errorf("libvirt: unable to destroy domain%s: %w", name, err)
	}

	err = dom.Undefine()
	if err != nil {
		return fmt.Errorf("libvirt: unable to undefine doman %s: %w", name, err)
	}

	return nil
}

// CreateDomain verifies if the domains exists already, if it does, it returns
// an error, otherwise it creates a new domain with the provided configuration.
func (d *driver) CreateDomain(config *domain.Config) error {
	dom, err := d.getDomain(config.Name)
	if err != nil {
		return err
	}

	if dom != nil {
		return ErrDomainExists
	}

	d.logger.Debug("domain doesn't exits, creating it")

	err = config.Validate()
	if err != nil {
		return err
	}

	d.logger.Debug("configuration is valid")

	isoPath := d.dataDir + "/" + config.Name + ".iso"
	defer func() {
		if config.RemoveConfigFiles {
			err := os.Remove(isoPath)
			if err != nil {
				d.logger.Warn("unable to remove iso", err)
			}
		}
	}()

	cic := createCloudInitConfig(config)
	d.logger.Debug("creating iso with data: ", fmt.Sprintf("%+v", cic))

	err = d.ci.WriteConfigToISO(cic, isoPath)
	if err != nil {
		return fmt.Errorf("libvirt: unable to create cidata: %w", err)
	}

	err = d.sp.Refresh(0)
	if err != nil {
		return fmt.Errorf("libvirt: to refresh storage pool: %w", err)
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

	dom, err = d.conn.DomainDefineXML(domXML)
	if err != nil {
		return fmt.Errorf("libvirt: unable to define domain: %w", err)
	}

	err = dom.Create()
	if err != nil {
		return fmt.Errorf("libvirt: unable to create domain: %w", err)
	}

	return nil
}

func (d *driver) GetDomain(name string) (*domain.Info, error) {
	dom, err := d.getDomain(name)
	if err != nil {
		return nil, fmt.Errorf("libvirt: unable to get domains: %w", err)
	}

	if dom == nil {
		return nil, nil
	}

	info, err := dom.GetInfo()
	if err != nil {
		return nil, fmt.Errorf("libvirt: unable to get domains: %w", err)
	}

	return &domain.Info{
		State:   nomadDomainStates[info.State],
		Memory:  info.Memory,
		CpuTime: info.CpuTime,
	}, nil
}

func (d *driver) GetAllDomains() ([]string, error) {
	doms, err := d.conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE)
	if err != nil {
		return nil, fmt.Errorf("libvirt: unable to list domains: %w", err)
	}

	dns := []string{}
	for _, dom := range doms {
		name, err := dom.GetName()
		if err == nil {
			return nil, fmt.Errorf("libvirt: unable get domain name: %w", err)
		}
		dns = append(dns, name)
		dom.Free()
	}

	return dns, nil
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

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

	"github.com/hashicorp/nomad-driver-virt/cloudinit"
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

	defaultDataDir     = "/opt/virt/"
	dataDirPermissions = 777
	storagePoolName    = "virt-sp"

	envVariblesFilePath        = "/etc/profile.d/virt.sh" //Only valid for linux OS
	envVariblesFilePermissions = "777"

	DOMAIN_RUNNING     = "running"
	DOMAIN_NOSTATE     = "unknown"
	DOMAIN_BLOCKED     = "blocked"
	DOMAIN_PAUSED      = "paused"
	DOMAIN_SHUTDOWN    = "shutdown"
	DOMAIN_CRASHED     = "crashed"
	DOMAIN_PMSUSPENDED = "pmsuspended"
	DOMAIN_SHUTOFF     = "shutoff"
)

var (
	ErrDomainExists   = errors.New("the domain exists already")
	ErrDomainNotFound = errors.New("the domain does not exist")

	nomadDomainStates = map[libvirt.DomainState]string{
		libvirt.DOMAIN_RUNNING:     DOMAIN_RUNNING,
		libvirt.DOMAIN_NOSTATE:     DOMAIN_NOSTATE,
		libvirt.DOMAIN_BLOCKED:     DOMAIN_BLOCKED,
		libvirt.DOMAIN_PAUSED:      DOMAIN_PAUSED,
		libvirt.DOMAIN_SHUTDOWN:    DOMAIN_SHUTDOWN,
		libvirt.DOMAIN_CRASHED:     DOMAIN_CRASHED,
		libvirt.DOMAIN_PMSUSPENDED: DOMAIN_PMSUSPENDED,
		libvirt.DOMAIN_SHUTOFF:     DOMAIN_SHUTOFF,
	}
)

type CloudInit interface {
	Apply(ci *cloudinit.Config, path string) error
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

	err = createDataDirectory(d.dataDir)
	if err != nil {
		return nil, fmt.Errorf("libvirt: unable to create data dir: %w", err)
	}

	conn, err := newConnection(d.uri, d.user, d.password)
	if err != nil {
		return nil, err
	}

	d.conn = conn

	go d.monitorCtx(ctx)

	d.logger.Info("setting up data directory", d.dataDir)

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

		d.logger.Info("creating storage pool")
		found, err = d.conn.StoragePoolCreateXML(spXML, 0)
		if err != nil {
			return nil, fmt.Errorf("libvirt: unable to create storage pool: %w", err)

		}
	}

	d.logger.Info("assigning storage pool")
	d.sp = found

	return d, nil
}

func (d *driver) GetInfo() (domain.VirtualizerInfo, error) {
	li := domain.VirtualizerInfo{}

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

func createCloudInitConfig(config *domain.Config) *cloudinit.Config {

	ms := []cloudinit.MountFileConfig{}
	for _, m := range config.Mounts {
		mount := cloudinit.MountFileConfig{
			Destination: m.Destination,
			Tag:         m.Tag,
		}

		ms = append(ms, mount)
	}

	// The process to have directories mounted on the VM consists on two steps,
	// one is declaring them as backing storage in the VM and the second is to
	// create the directory inside the VM and executing the mount. These commands
	// are added here for cloud init to execute on boot.
	mountCMDs := addCMDsForMounts(ms)

	fs := []cloudinit.File{}
	for _, f := range config.Files {
		file := cloudinit.File{
			Path:        f.Path,
			Content:     f.Content,
			Permissions: f.Permissions,
			Encoding:    f.Encoding,
			Owner:       f.Owner,
			Group:       f.Group,
		}

		fs = append(fs, file)
	}

	//fs = append(fs, envFile)

	return &cloudinit.Config{
		MetaData: cloudinit.MetaData{
			InstanceID:    config.Name,
			LocalHostname: config.HostName,
		},
		VendorData: cloudinit.VendorData{
			BootCMD:  append(config.BOOTCMDs, mountCMDs...),
			RunCMD:   config.CMDs,
			Mounts:   ms,
			Files:    fs,
			Password: config.Password,
			SSHKey:   config.SSHKey,
		},
		UserDataPath: config.CIUserData,
	}
}

func addCMDsForMounts(mounts []cloudinit.MountFileConfig) []string {
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
	d.logger.Warn("stoping domain", name)
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
	d.logger.Warn("destroying domain", "name", name)
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

	d.logger.Debug("domain doesn't exits, creating it", config.Name)

	cloudInitConfigPath := d.dataDir + "/" + config.Name + ".iso"
	defer func() {
		if config.RemoveConfigFiles {
			err := os.Remove(cloudInitConfigPath)
			if err != nil {
				d.logger.Warn("unable to remove cloudinit configFile", err)
			}
		}
	}()

	cic := createCloudInitConfig(config)
	d.logger.Debug("creating ci configuration: ", fmt.Sprintf("%+v", cic))

	err = d.ci.Apply(cic, cloudInitConfigPath)
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
		domXML, err = parceConfiguration(config, cloudInitConfigPath)
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

	/* 	cpu_stats, err := dom.GetCPUStats(-1, 1, 0) // DomainCPUStats
	   	if err != nil {
	   		return nil, fmt.Errorf("libvirt: can not get cpu_stats: %w", err)
	   	} */

	/* RSSMem, err := dom.MemoryStats(uint32(libvirt.DOMAIN_MEMORY_STAT_RSS), 0) // MemoryStats
	if err != nil {
		return nil, fmt.Errorf("libvirt: can not get memory_stats: %w", err)
	} */

	return &domain.Info{
		State:     nomadDomainStates[info.State],
		Memory:    info.Memory,
		MaxMemory: info.MaxMem,
		CPUTime:   info.CpuTime,
		NrVirtCPU: info.NrVirtCpu,
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

func createDataDirectory(path string) error {
	err := os.MkdirAll(path, dataDirPermissions)
	if err != nil {
		return err
	}

	return nil
}

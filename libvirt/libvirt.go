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

	domain "github/hashicorp/nomad-driver-virt/internal/shared"

	"github.com/hashicorp/go-hclog"
	libvirtxml "github.com/libvirt/libvirt-go-xml"
	"libvirt.org/go/libvirt"
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

var (
	ErrDomainExists   = errors.New("the domain exists already")
	ErrDomainNotFound = errors.New("the domain does not exist")
)

type driver struct {
	uri      string
	conn     *libvirt.Connect
	logger   hclog.Logger
	dataDir  string
	user     string
	password string
}

type Option func(*driver)

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

func WithDataDirectory(path string) Option {
	return func(d *driver) {
		d.dataDir = path
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

	domainDir := filepath.Join(d.dataDir, userDataDir, config.Name)
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

	}

	cero := uint(0)
	domcfg := &libvirtxml.Domain{
		OnPoweroff: "destroy",
		OnReboot:   "destroy",
		OnCrash:    "destroy",
		PM: &libvirtxml.DomainPM{
			SuspendToMem: &libvirtxml.DomainPMPolicy{
				Enabled: "no",
			},
			SuspendToDisk: &libvirtxml.DomainPMPolicy{
				Enabled: "no",
			},
		},
		Features: &libvirtxml.DomainFeatureList{
			VMPort: &libvirtxml.DomainFeatureState{
				State: "off",
			},
		},
		SysInfo: []libvirtxml.DomainSysInfo{
			{
				SMBIOS: &libvirtxml.DomainSysInfoSMBIOS{
					System: &libvirtxml.DomainSysInfoSystem{
						Entry: []libvirtxml.DomainSysInfoEntry{
							{
								Name:  "serial",
								Value: "ds=nocloud",
							},
						},
					},
				},
			},
		},
		OS: &libvirtxml.DomainOS{
			Type: &libvirtxml.DomainOSType{
				//	Arch:    "x86_64",
				//	Machine: "pc-i440fx-jammy",
				Type: "hvm",
			},
			SMBios: &libvirtxml.DomainSMBios{
				Mode: "sysinfo",
			},
		},
		Devices: &libvirtxml.DomainDeviceList{
			Controllers: []libvirtxml.DomainController{
				{
					Type:  "virtio-serial",
					Index: &cero,
				},
				{
					Type:  "sata",
					Index: &cero,
				},
			},
			Serials: []libvirtxml.DomainSerial{
				{
					Target: &libvirtxml.DomainSerialTarget{
						Type: "isa-serial",
						Port: &cero,
						Model: &libvirtxml.DomainSerialTargetModel{
							Name: "isa-serial",
						},
					},
				},
			},
			Consoles: []libvirtxml.DomainConsole{
				{
					Target: &libvirtxml.DomainConsoleTarget{
						Type: "serial",
						Port: &cero,
					},
				},
			},
			RNGs: []libvirtxml.DomainRNG{
				{
					Model: "virtio",
					Backend: &libvirtxml.DomainRNGBackend{
						Random: &libvirtxml.DomainRNGBackendRandom{
							Device: "/dev/urandom",
						},
					},
				},
			},
			Disks: []libvirtxml.DomainDisk{
				{
					Device: "disk",
					Driver: &libvirtxml.DomainDiskDriver{
						Name: "qemu",
						Type: "qcow2",
					},
					Source: &libvirtxml.DomainDiskSource{
						File: &libvirtxml.DomainDiskSourceFile{
							File: config.BaseImage,
						},
					},
					BackingStore: &libvirtxml.DomainDiskBackingStore{
						Index: 3,
						Format: &libvirtxml.DomainDiskFormat{
							Type: "qcow2",
						},
						Source: &libvirtxml.DomainDiskSource{
							File: &libvirtxml.DomainDiskSourceFile{
								File: config.OriginalImage,
							},
						},
					},
					Target: &libvirtxml.DomainDiskTarget{
						Dev: "vda",
						Bus: "virtio",
					},
				},
				{
					Device: "cdrom",
					Driver: &libvirtxml.DomainDiskDriver{
						Name: "qemu",
						Type: "raw",
					},
					Source: &libvirtxml.DomainDiskSource{
						File: &libvirtxml.DomainDiskSourceFile{
							File: "/home/ubuntu/test/cidata.iso",
						},
					},
					Target: &libvirtxml.DomainDiskTarget{
						Dev: "sda",
						Bus: "sata",
					},
					ReadOnly: &libvirtxml.DomainDiskReadOnly{},
				},
			},
			Filesystems: []libvirtxml.DomainFilesystem{
				{
					AccessMode: "passthrough",
					Driver: &libvirtxml.DomainFilesystemDriver{
						Type: "virtiofs",
					},
					Binary: &libvirtxml.DomainFilesystemBinary{
						Path: "/usr/lib/qemu/virtiofsd",
					},
					Source: &libvirtxml.DomainFilesystemSource{
						Mount: &libvirtxml.DomainFilesystemSourceMount{
							Dir: "/home/ubuntu/test/alloc",
						},
					},
					Target: &libvirtxml.DomainFilesystemTarget{
						Dir: "allocDir",
					},
					Alias: &libvirtxml.DomainAlias{
						Name: "fs0",
					},
				},
			},
			Interfaces: []libvirtxml.DomainInterface{
				{
					Source: &libvirtxml.DomainInterfaceSource{
						Bridge: &libvirtxml.DomainInterfaceSourceBridge{
							Bridge: "virbr0",
						},
					},
					Model: &libvirtxml.DomainInterfaceModel{
						Type: "virtio",
					},
				},
			},
		},
		Type: "kvm",
		Name: config.Name,
		Memory: &libvirtxml.DomainMemory{
			Value: config.Memory,
		},
		MemoryBacking: &libvirtxml.DomainMemoryBacking{
			MemorySource: &libvirtxml.DomainMemorySource{
				Type: "memfd",
			},
			MemoryAccess: &libvirtxml.DomainMemoryAccess{
				Mode: "shared",
			},
		},
		VCPU: &libvirtxml.DomainVCPU{
			Placement: "static",
			Value:     uint(config.CPUs),
		},
		Resource: &libvirtxml.DomainResource{
			Partition: "/machine",
		},
		/*  CPU: &libvirtxml.DomainCPU{
			Topology: &libvirtxml.DomainCPUTopology{
				Cores:   config.CPUs,
				Sockets: 2,
				Threads: 1,
			},
		}, */
	}
	xmldoc, err := domcfg.Marshal()
	fmt.Println(xmldoc, err)

	ddomLi, err := d.conn.DomainCreateXML(xmldoc, 0)
	fmt.Println(ddomLi, err)

	/* err = d.createDomainWithVirtInstall(config, ci)
	if err != nil {
		return err
	} */

	return nil
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

	//d.logger.Debug("logger", errb.String())
	//d.logger.Info("logger", outb.String())
	fmt.Println(outb.String())

	dom, err := d.conn.DomainCreateXML(outb.String(), 0)
	fmt.Println(dom, err)
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

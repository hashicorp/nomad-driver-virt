// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sync"

	"github.com/hashicorp/nomad-driver-virt/cloudinit"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	libvirtnet "github.com/hashicorp/nomad-driver-virt/providers/libvirt/net"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
	virtnet "github.com/hashicorp/nomad-driver-virt/virt/net"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/plugins/shared/structs"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

const (
	defaultURI               = "qemu:///system"
	defaultVirtualizatioType = "hvm"
	defaultAccelerator       = "kvm"
	defaultSecurityMode      = "mapped"
	defaultInterfaceModel    = "virtio"
	libvirtVirtioChannel     = "org.qemu.guest_agent.0" // This is is the only channel libvirt will use to connect to the qemu agent.

	defaultDataDir     = "/var/lib/"
	dataDirPermissions = 777
	storagePoolName    = "virt-sp"

	// URI for running in test mode
	TestURI = "test:///default"

	// Known domain states
	DomainRunning     = "running"
	DomainNoState     = "unknown"
	DomainBlocked     = "blocked"
	DomainPaused      = "paused"
	DomainShutdown    = "shutdown"
	DomainCrashed     = "crashed"
	DomainPMSuspended = "pmsuspended"
	DomainShutOff     = "shutoff"
)

var (
	ErrConnectionClosed = errors.New("libvirt connection is closed")
	ErrDomainExists     = errors.New("the domain exists already")
	ErrDomainNotFound   = errors.New("the domain does not exist")

	// nomadDomainStates is a mapping of the libvirt domain state to local string constant
	nomadDomainStates = map[libvirt.DomainState]string{
		libvirt.DOMAIN_RUNNING:     DomainRunning,
		libvirt.DOMAIN_NOSTATE:     DomainNoState,
		libvirt.DOMAIN_BLOCKED:     DomainBlocked,
		libvirt.DOMAIN_PAUSED:      DomainPaused,
		libvirt.DOMAIN_SHUTDOWN:    DomainShutdown,
		libvirt.DOMAIN_CRASHED:     DomainCrashed,
		libvirt.DOMAIN_PMSUSPENDED: DomainPMSuspended,
		libvirt.DOMAIN_SHUTOFF:     DomainShutOff,
	}

	// vmDomainStates is a mapping of the libvirt domain state to common vm state
	vmDomainStates = map[libvirt.DomainState]vm.VMState{
		libvirt.DOMAIN_RUNNING:     vm.VMStateRunning,
		libvirt.DOMAIN_NOSTATE:     vm.VMStateUnknown,
		libvirt.DOMAIN_BLOCKED:     vm.VMStateError,
		libvirt.DOMAIN_PAUSED:      vm.VMStatePaused,
		libvirt.DOMAIN_SHUTDOWN:    vm.VMStateShutdown,
		libvirt.DOMAIN_CRASHED:     vm.VMStateError,
		libvirt.DOMAIN_PMSUSPENDED: vm.VMStateSuspended,
		libvirt.DOMAIN_SHUTOFF:     vm.VMStatePowerOff,
	}
)

// TODO: Add function to test for correct cloudinit userdata syntax, and dont
// use it if it fails, throw a warning!
type CloudInit interface {
	Apply(ci *cloudinit.Config, path string) error
}

type driver struct {
	ctx      context.Context
	uri      string
	conn     *libvirt.Connect
	logger   hclog.Logger
	user     string
	password string
	ci       CloudInit
	dataDir  string
	sp       *libvirt.StoragePool
	opts     []Option
	closed   bool
	m        sync.Mutex
}

// Copy creates a copy of this driver.
func (d *driver) Copy() *driver {
	d.m.Lock()
	defer d.m.Unlock()

	dCopy := &driver{
		ctx:      d.ctx,
		closed:   d.closed,
		uri:      d.uri,
		logger:   d.logger,
		user:     d.user,
		password: d.password,
		ci:       d.ci,
		dataDir:  d.dataDir,
		sp:       d.sp,
	}
	go dCopy.monitorCtx()

	return dCopy
}

type Option func(*driver)

func WithDataDir(dir string) Option {
	return func(d *driver) {
		d.dataDir = dir
	}
}

func WithConfig(c *Config) Option {
	return func(d *driver) {
		if c == nil {
			return
		}

		if c.URI != "" {
			d.uri = c.URI
		}
		if c.User != "" {
			d.user = c.User
		}
		if c.Password != "" {
			d.password = c.Password
		}
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

func WithCIController(ci CloudInit) Option {
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

func (d *driver) connection() (*libvirt.Connect, error) {
	d.m.Lock()
	defer d.m.Unlock()

	// if marked as closed, no new connections should established
	if d.closed {
		return nil, ErrConnectionClosed
	}

	if d.conn != nil {
		alive, err := d.conn.IsAlive()
		if alive {
			return d.conn, nil
		}

		if err != nil {
			d.logger.Warn("error on connection alive check", "error", err)
		}
	}

	var err error
	d.conn, err = newConnection(d.uri, d.user, d.password)
	if err != nil {
		return nil, err
	}

	return d.conn, nil
}

func (d *driver) monitorCtx() {
	select {
	case <-d.ctx.Done():
		d.m.Lock()
		defer d.m.Unlock()

		d.closed = true
		if d.conn != nil {
			d.conn.Close()
		}
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

func New(ctx context.Context, logger hclog.Logger, opt ...Option) *driver {
	d := &driver{
		ctx:     ctx,
		logger:  logger.Named("nomad-virt-plugin"),
		uri:     defaultURI,
		opts:    opt,
		dataDir: defaultDataDir,
	}

	for _, opt := range d.opts {
		opt(d)
	}

	go d.monitorCtx()

	return d
}

// Start executes all the options passed at creation and initializes all
// the dependencies, they are not initialized at creation because the NewPlugin
// function does not return errors. It also starts or reataches a storage pool
// to place the disks for the virtual machines.
func (d *driver) Init() error {
	if d.ci == nil {
		ci, err := cloudinit.NewController(d.logger.Named("cloud-init"))
		if err != nil {
			return err
		}
		d.ci = ci
	}
	conn, err := d.connection()
	if err != nil {
		return err
	}

	// In order to make host files visible to the virtual machines, they need to be
	// placed inside a defined storage pool.
	found, err := d.lookForExistingStoragePool(storagePoolName)
	if err != nil {
		return fmt.Errorf("libvirt: unable to list existing storage pools: %w", err)

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
			return err
		}

		d.logger.Info("creating storage pool")
		found, err = conn.StoragePoolCreateXML(spXML, 0)
		if err != nil {
			return fmt.Errorf("libvirt: unable to create storage pool: %w", err)

		}

		d.logger.Info("assigning storage pool")
	}
	d.sp = found

	return nil
}

func (d *driver) ListNetworks() ([]string, error) {
	conn, err := d.connection()
	if err != nil {
		return nil, err
	}

	return conn.ListNetworks()
}

func (d *driver) LookupNetworkByName(name string) (shims.ConnectNetwork, error) {
	conn, err := d.connection()
	if err != nil {
		return nil, err
	}

	return conn.LookupNetworkByName(name)
}

func (d *driver) GetInfo() (vm.VirtualizerInfo, error) {
	li := vm.VirtualizerInfo{}

	conn, err := d.connection()
	if err != nil {
		return li, err
	}

	ni, err := conn.GetNodeInfo()
	if err != nil {
		return li, fmt.Errorf("libvirt: unable to get node info: %w", err)
	}

	ev, err := conn.GetVersion()
	if err != nil {
		return li, fmt.Errorf("libvirt: unable to get e version: %w", err)
	}

	lv, err := conn.GetLibVersion()
	if err != nil {
		return li, fmt.Errorf("libvirt: unable to get lib version: %w", err)
	}

	fm, err := conn.GetFreeMemory()
	if err != nil {
		return li, fmt.Errorf("libvirt: unable to get free memory: %w", err)
	}

	aDoms, err := conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE)
	if err != nil {
		return li, fmt.Errorf("libvirt: unable to get active domains: %w", err)
	}

	iDoms, err := conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_INACTIVE)
	if err != nil {
		return li, fmt.Errorf("libvirt: unable to get inactive domains: %w", err)
	}

	sps, err := conn.ListAllStoragePools(libvirt.CONNECT_LIST_STORAGE_POOLS_ACTIVE)
	if err != nil {
		return li, fmt.Errorf("libvirt: unable to get storage pools: %w", err)
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
	li.StoragePools = uint(len(sps))

	return li, nil
}

func (d *driver) Close() (int, error) {
	d.m.Lock()
	defer d.m.Unlock()

	if d.conn == nil {
		return 0, nil
	}

	return d.conn.Close()
}

func createCloudInitConfig(config *vm.Config) *cloudinit.Config {

	ms := []cloudinit.MountFileConfig{}
	for _, m := range config.Mounts {
		mount := cloudinit.MountFileConfig{
			Destination: m.Destination,
			Tag:         m.Tag,
		}

		ms = append(ms, mount)
	}

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

	return &cloudinit.Config{
		MetaData: cloudinit.MetaData{
			InstanceID:    config.Name,
			LocalHostname: config.HostName,
		},
		VendorData: cloudinit.VendorData{
			BootCMD:  config.BOOTCMDs,
			RunCMD:   config.CMDs,
			Mounts:   ms,
			Files:    fs,
			Password: config.Password,
			SSHKey:   config.SSHKey,
		},
		UserData: config.CIUserData,
	}
}

// getDomain looks up a domain by name, if an error ocurred, it will be returned.
// If no domain is found, nil is returned.
func (d *driver) getDomain(name string) (*libvirt.Domain, error) {
	conn, err := d.connection()
	if err != nil {
		return nil, err
	}

	dom, err := conn.LookupDomainByName(name)
	if err != nil {
		lverr, ok := err.(libvirt.Error)
		if ok {
			if lverr.Code != libvirt.ERR_NO_DOMAIN {
				return nil, fmt.Errorf("libvirt: unable to verify exiting domains %s: %w", name, err)
			}
		}
	}

	return dom, nil
}

// StopVM will set a domain to shutoff, but it will still be present as
// inactive and can be restarted.
func (d *driver) StopVM(name string) error {
	d.logger.Warn("stopping domain", "name", name)

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

// DestroyVM destroys and undefines a domain, meaning it will be completely
// removes it.
func (d *driver) DestroyVM(name string) error {
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
		// In case we want to destroy a domain that was previoulsy stopped, destroy
		// is not idempotent and will throw the error operation invalid if the
		// domain is already stopped.
		lverr, ok := err.(libvirt.Error)
		if ok {
			if lverr.Code != libvirt.ERR_OPERATION_INVALID {
				return fmt.Errorf("libvirt: unable to destroy domain %s: %w", name, err)
			}
		}
	}

	err = dom.Undefine()
	if err != nil {
		return fmt.Errorf("libvirt: unable to undefine domain %s: %w", name, err)
	}

	return nil
}

// CreateVM verifies if the domains exists already, if it does, it returns
// an error, otherwise it creates a new domain with the provided configuration.
func (d *driver) CreateVM(config *vm.Config) error {
	conn, err := d.connection()
	if err != nil {
		return err
	}

	dom, err := d.getDomain(config.Name)
	if err != nil {
		return err
	}

	if dom != nil {
		return fmt.Errorf("libvirt: %s: %w", config.Name, ErrDomainExists)
	}

	d.logger.Debug("domain doesn't exist, creating it", "name", config.Name)

	cloudInitConfigPath := filepath.Join(d.dataDir, config.Name+".iso")

	cic := createCloudInitConfig(config)
	d.logger.Debug("creating cloud init configuration: ", fmt.Sprintf("%+v", cic))

	if err := d.ci.Apply(cic, cloudInitConfigPath); err != nil {
		return fmt.Errorf("libvirt: unable to create cidata %s: %w", config.Name, err)
	}

	if config.RemoveConfigFiles {
		defer func() {
			err := os.Remove(cloudInitConfigPath)
			if err != nil {
				d.logger.Warn("libvirt: unable to remove cloudinit configFile", "error", err)
			}
		}()
	}

	if err := d.sp.Refresh(0); err != nil {
		return fmt.Errorf("libvirt: unable to refresh storage pool %s: %w", config.Name, err)
	}

	var domXML string
	if config.XMLConfig != "" {
		domXML = config.XMLConfig
	} else {
		domXML, err = parseConfiguration(config, cloudInitConfigPath)
		if err != nil {
			return fmt.Errorf("libvirt: unable to parse domain configuration %s: %w", config.Name, err)
		}
	}

	d.logger.Debug("creating domain", "xml", domXML)

	dom, err = conn.DomainDefineXML(domXML)
	if err != nil {
		return fmt.Errorf("libvirt: unable to define domain %s: %w", config.Name, err)
	}

	if err := dom.Create(); err != nil {
		return fmt.Errorf("libvirt: unable to create domain %s: %w", config.Name, err)
	}

	return nil
}

// GetVM find a domain by name and returns basic functionality information
// including current state, memory and CPU. If no domain is found nil is returned.
func (d *driver) GetVM(name string) (*vm.Info, error) {
	dom, err := d.getDomain(name)
	if err != nil {
		return nil, fmt.Errorf("libvirt: unable to get domain %s: %w", name, err)
	}

	if dom == nil {
		return nil, fmt.Errorf("libvirt: domain %s %w", name, vm.ErrNotFound)
	}

	info, err := dom.GetInfo()
	if err != nil {
		return nil, fmt.Errorf("libvirt: unable to get domain %s: %w", name, err)
	}

	return &vm.Info{
		RawState:  nomadDomainStates[info.State],
		State:     vmDomainStates[info.State],
		Memory:    info.Memory,
		MaxMemory: info.MaxMem,
		CPUTime:   info.CpuTime,
		NrVirtCPU: info.NrVirtCpu,
	}, nil
}

// GetNetworkInterfaces retrieves the network interfaces defined for the given
// vm. Interfaces information population is best effort, as not all information
// will be available depending on the state of the vm.
func (d *driver) GetNetworkInterfaces(name string) ([]vm.NetworkInterface, error) {
	dom, err := d.getDomain(name)
	if err != nil {
		d.logger.Error("cannot get domain", "domain", name, "error", err)
		return nil, fmt.Errorf("libvirt: unable to get domain %s: %w", name, err)
	}
	xml, err := dom.GetXMLDesc(0)
	if err != nil {
		d.logger.Error("cannot get domain XML", "domain", name, "error", err)
		return nil, fmt.Errorf("libvirt: unable to get domain XML %s: %w", name, err)
	}

	dxml := &libvirtxml.Domain{}
	if err := dxml.Unmarshal(xml); err != nil {
		d.logger.Error("cannot parse domain XML", "domain", name, "error", err)
		return nil, fmt.Errorf("libvirt: unable to parse domain XML %s: %w", name, err)
	}

	interfaces := make([]vm.NetworkInterface, len(dxml.Devices.Interfaces))

	allNetworks, err := d.conn.ListAllNetworks(libvirt.CONNECT_LIST_NETWORKS_ACTIVE)
	if err != nil {
		d.logger.Error("cannot list available networks", "domain", name, "error", err)
		return nil, fmt.Errorf("libvirt: unable to get domain interfaces list %s: %w", name, err)
	}

	bridgeNetworks := map[string]string{}
	for _, network := range allNetworks {
		netname, err := network.GetName()
		if err != nil {
			d.logger.Debug("failed to get network name", "domain", name, "error", err)
			continue
		}
		bridge, err := network.GetBridgeName()
		if err != nil {
			d.logger.Debug("failed to get network bridge", "domain", name, "network", netname,
				"error", err)
			continue
		}

		bridgeNetworks[bridge] = netname
	}

	for i, iface := range dxml.Devices.Interfaces {
		interfaces[i] = vm.NetworkInterface{}

		if iface.Source != nil && iface.Source.Bridge != nil {
			if netName, ok := bridgeNetworks[iface.Source.Bridge.Bridge]; ok {
				interfaces[i].NetworkName = netName
			} else {
				d.logger.Debug("no matching network found for bridge", "domain", name,
					"bridge", iface.Source.Bridge.Bridge)
			}
		}

		if iface.Model != nil {
			interfaces[i].Model = iface.Model.Type
		}

		if iface.Driver != nil {
			interfaces[i].Driver = iface.Driver.Name
		}

		if iface.MAC != nil {
			interfaces[i].MAC = iface.MAC.Address
		}

		// The Guest and IP values will only be set if the guest agent
		// is available (as it is responsible for reporting these values).
		// If it is not, these value will remain unset.
		if iface.Guest != nil {
			interfaces[i].DeviceName = iface.Guest.Dev
		}

		interfaces[i].Addrs = make([]netip.Addr, len(iface.IP))
		for j, addr := range iface.IP {
			interfaces[i].Addrs[j], err = netip.ParseAddr(addr.Address)
			if err != nil {
				d.logger.Warn("failed to parse interface address",
					"domain", name, "address", addr.Address, "error", err)
			}
		}

		d.logger.Debug("domain network interface retrieved", "domain", name,
			"interface", interfaces[i])
	}

	return interfaces, nil
}

func (d *driver) GetAllDomains() ([]string, error) {
	conn, err := d.connection()
	if err != nil {
		return nil, err
	}

	doms, err := conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE)
	if err != nil {
		return nil, fmt.Errorf("libvirt: unable to list domains: %w", err)
	}

	dns := []string{}
	for _, dom := range doms {
		name, err := dom.GetName()
		if err != nil {
			return nil, fmt.Errorf("libvirt: unable get domain name: %w", err)
		}
		dns = append(dns, name)
		err = dom.Free()
		if err != nil {
			d.logger.Error("unable to free domain", "name", name)
		}
	}

	return dns, nil
}

// UseCloudInit informs that cloud-init is supported by this provider.
func (d *driver) UseCloudInit() bool {
	return true
}

// Networking returns the virtualization network sub-system
func (d *driver) Networking() (virtnet.Net, error) {
	return libvirtnet.NewController(d.logger, d), nil
}

// Fingerprint generates the fingerprint attributes for this provider.
func (d *driver) Fingerprint() (map[string]*structs.Attribute, error) {
	conn, err := d.connection()
	if err != nil {
		return nil, err
	}

	// Collect all the information required
	driver, err := conn.GetType()
	if err != nil {
		return nil, err
	}

	driverVersion, err := conn.GetVersion()
	if err != nil {
		return nil, err
	}

	libvirtVersion, err := conn.GetLibVersion()
	if err != nil {
		return nil, err
	}

	n, err := d.Networking()
	if err != nil {
		return nil, err
	}

	attrs := map[string]*structs.Attribute{
		"version":                 structs.NewIntAttribute(int64(libvirtVersion), ""),
		"version.readable":        structs.NewStringAttribute(computeVersion(libvirtVersion)),
		"driver":                  structs.NewStringAttribute(driver),
		"driver.version":          structs.NewIntAttribute(int64(driverVersion), ""),
		"driver.version.readable": structs.NewStringAttribute(computeVersion(driverVersion)),
	}

	// Add any fingerprint information from the networking subsystem
	n.Fingerprint(attrs)

	return attrs, nil
}

const (
	majorDivisor = 1_000_000
	minorDivisor = 1_000
)

// computeVersion will compute the version string from the
// integer value provided by libvirt.
func computeVersion(version uint32) string {
	major := version / majorDivisor
	version -= version % majorDivisor
	minor := version / minorDivisor
	version -= version % minorDivisor

	return fmt.Sprintf("%d.%d.%d", major, minor, version)
}

func folderExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func createDataDirectory(path string) error {
	if folderExists(path) {
		return nil
	}

	err := os.MkdirAll(path, dataDirPermissions)
	if err != nil {
		return err
	}

	return nil
}

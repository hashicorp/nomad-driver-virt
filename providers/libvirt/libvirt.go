// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"

	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	libvirtnet "github.com/hashicorp/nomad-driver-virt/providers/libvirt/net"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
	libvirt_storage "github.com/hashicorp/nomad-driver-virt/providers/libvirt/storage"
	"github.com/hashicorp/nomad-driver-virt/storage"
	virtnet "github.com/hashicorp/nomad-driver-virt/virt/net"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/plugins/shared/structs"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

const (
	defaultURI                = "qemu:///system"
	defaultVirtualizationType = "hvm"
	defaultAccelerator        = "kvm"
	defaultSecurityMode       = "mapped"
	defaultInterfaceModel     = "virtio"
	libvirtVirtioChannel      = "org.qemu.guest_agent.0" // This is is the only channel libvirt will use to connect to the qemu agent.
	libvirtNoFlags            = 0
	mountFsVirtiofs           = "vhost-user-fs-pci"
	mountFs9p                 = "virtio-9p-pci"
	virtiofsQueueSize         = 1024
	virtiofsSecurityMode      = "passthrough"

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
	ErrDomainNotFound   = fmt.Errorf("domain %w", vm.ErrNotFound)
	ErrPoolNotFound     = fmt.Errorf("storage pool %w", vm.ErrNotFound)

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

type driver struct {
	ctx              context.Context
	uri              string
	conn             *libvirt.Connect
	logger           hclog.Logger
	user             string
	password         string
	opts             []Option
	closed           bool
	storage          *libvirt_storage.Storage
	networking       *libvirtnet.Controller
	cancel           context.CancelFunc
	availableMountFs map[string]struct{}
	libvirtVersion   uint32
	m                sync.Mutex

	availableMountFsOverride map[string]struct{} // used for testing
}

// Copy creates a copy of this driver.
func (d *driver) Copy(ctx context.Context) *driver {
	d.m.Lock()
	defer d.m.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	dCopy := &driver{
		ctx:              ctx,
		cancel:           cancel,
		closed:           d.closed,
		uri:              d.uri,
		logger:           d.logger,
		user:             d.user,
		password:         d.password,
		availableMountFs: d.availableMountFs,
		libvirtVersion:   d.libvirtVersion,
	}
	dCopy.storage = d.storage.Copy(ctx, dCopy)
	dCopy.networking = d.networking.Copy(dCopy)

	go dCopy.monitorCtx()

	return dCopy
}

type Option func(*driver)

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

func newConnection(uri string, user string, pass string) (*libvirt.Connect, error) {
	if user == "" {
		return libvirt.NewConnect(uri)
	}

	callback := func(creds []*libvirt.ConnectCredential) {
		for _, cred := range creds {
			switch cred.Type {
			case libvirt.CRED_AUTHNAME:
				cred.Result = user
				cred.ResultLen = len(cred.Result)
			case libvirt.CRED_PASSPHRASE:
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
	virConn, err := libvirt.NewConnectWithAuth(uri, auth, libvirtNoFlags)

	return virConn, err
}

// connection returns a working connection to the libvirt daemon
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

		// it's not alive so close it to free the connection resources
		d.conn.Close()
	}

	var err error
	d.conn, err = newConnection(d.uri, d.user, d.password)
	if err != nil {
		return nil, err
	}

	return d.conn, nil
}

// monitorCtx monitors the context and once done closes
// the connection and marks the driver as closed
func (d *driver) monitorCtx() {
	<-d.ctx.Done()
	d.m.Lock()
	defer d.m.Unlock()

	d.closed = true
	if d.conn != nil {
		d.conn.Close()
		d.conn = nil
	}
}

func New(ctx context.Context, logger hclog.Logger, opt ...Option) *driver {
	ctx, cancel := context.WithCancel(ctx)
	d := &driver{
		ctx:    ctx,
		logger: logger.Named("libvirt"),
		uri:    defaultURI,
		opts:   opt,
		cancel: cancel,
	}
	d.networking = libvirtnet.NewController(d.logger, d)

	for _, opt := range d.opts {
		opt(d)
	}

	go d.monitorCtx()

	return d
}

// Init initializes the driver.
func (d *driver) Init() error {
	c, err := d.connection()
	if err != nil {
		return err
	}

	d.libvirtVersion, err = c.GetVersion()
	if err != nil {
		d.logger.Debug("unable to get libvirt version", "error", err)
		return err
	}

	// Determine what filesystems are supported for mounting within
	// the guest.
	if d.availableMountFs, err = d.findAvailableMountFs(); err != nil {
		return err
	}

	return nil
}

// SetupStorage loads the storage interface enabling connfigured storage pool
func (d *driver) SetupStorage(config *storage.Config) error {
	if config == nil {
		return fmt.Errorf("%w: missing storage pool configuration", vm.ErrInvalidConfiguration)
	}

	s, err := libvirt_storage.New(d.ctx, d.logger, d, config)
	if err != nil {
		return err
	}

	d.storage = s
	return nil
}

// NewStream creates a new libvirt stream
// NOTE: caller is responsible to free result
func (d *driver) NewStream() (shims.Stream, error) {
	c, err := d.connection()
	if err != nil {
		return nil, err
	}

	s, err := c.NewStream(libvirtNoFlags)
	if err != nil {
		return nil, err
	}

	return shims.WrapStream(s), nil
}

// FindStoragePool finds the named storage pool
// NOTE: caller is responsible to free result
func (d *driver) FindStoragePool(name string) (shims.StoragePool, error) {
	conn, err := d.connection()
	if err != nil {
		return nil, err
	}

	pool, err := conn.LookupStoragePoolByName(name)
	if err != nil && !errors.Is(err, libvirt.ERR_NO_STORAGE_POOL) {
		return nil, err
	}

	if pool == nil {
		return nil, ErrPoolNotFound
	}

	return shims.WrapStoragePool(pool), nil
}

// CreateStoragePool creates a new libvirt storage pool
// NOTE: caller is responsible to free result
func (d *driver) CreateStoragePool(def *libvirtxml.StoragePool) (shims.StoragePool, error) {
	conn, err := d.connection()
	if err != nil {
		return nil, err
	}

	poolDef, err := def.Marshal()
	if err != nil {
		return nil, err
	}

	// Define the pool to make the pool persistent.
	pool, err := conn.StoragePoolDefineXML(poolDef, libvirtNoFlags)
	if err != nil {
		return nil, err
	}

	// Enable autostart on the pool so it will be turned
	// on after the libvirt service is restarted.
	if err := pool.SetAutostart(true); err != nil {
		return nil, err
	}

	// Create starts the pool.
	if err := pool.Create(libvirtNoFlags); err != nil {
		return nil, err
	}

	return shims.WrapStoragePool(pool), nil
}

// UpdateStoragePool updates an existing libvirt storage pool
func (d *driver) UpdateStoragePool(def *libvirtxml.StoragePool) error {
	conn, err := d.connection()
	if err != nil {
		return err
	}

	poolDef, err := def.Marshal()
	if err != nil {
		return err
	}

	// This will update the pool definition
	pool, err := conn.StoragePoolDefineXML(poolDef, libvirtNoFlags)
	if err != nil {
		return err
	}
	defer pool.Free()

	// Now refresh the pool to pick up any changes
	if err := pool.Refresh(libvirtNoFlags); err != nil {
		return err
	}

	return nil
}

// Storage returns the storage interface
func (d *driver) Storage() storage.Storage {
	return d.storage
}

// ListNetworks lists the available networks to libvirt
func (d *driver) ListNetworks() ([]string, error) {
	conn, err := d.connection()
	if err != nil {
		return nil, err
	}

	return conn.ListNetworks()
}

// LookupNetworkByName looks up a network by its name
// NOTE: caller is responsible to free result
func (d *driver) LookupNetworkByName(name string) (shims.ConnectNetwork, error) {
	conn, err := d.connection()
	if err != nil {
		return nil, err
	}

	return conn.LookupNetworkByName(name)
}

// GetInfo returns information about this virtualizer
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
	defer func() {
		for _, d := range aDoms {
			d.Free()
		}
	}()

	iDoms, err := conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_INACTIVE)
	if err != nil {
		return li, fmt.Errorf("libvirt: unable to get inactive domains: %w", err)
	}
	defer func() {
		for _, d := range iDoms {
			d.Free()
		}
	}()

	sps, err := conn.ListAllStoragePools(libvirt.CONNECT_LIST_STORAGE_POOLS_ACTIVE)
	if err != nil {
		return li, fmt.Errorf("libvirt: unable to get storage pools: %w", err)
	}
	defer func() {
		for _, p := range sps {
			p.Free()
		}
	}()

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

// Close closes the connection to libvirtd.
func (d *driver) Close() error {
	d.m.Lock()
	defer d.m.Unlock()
	var err error

	// If the connection is still available, close it
	// and unset it
	if d.conn != nil {
		_, err = d.conn.Close()
		d.conn = nil
	}

	// Mark as closed
	d.closed = true

	// Cancel the context to stop the monitor
	d.cancel()

	return err
}

// getDomain looks up a domain by name, if an error ocurred, it will be returned.
// NOTE: caller is responsible to free result
func (d *driver) getDomain(name string) (*libvirt.Domain, error) {
	conn, err := d.connection()
	if err != nil {
		return nil, err
	}

	dom, err := conn.LookupDomainByName(name)
	if err != nil && !errors.Is(err, libvirt.ERR_NO_DOMAIN) {
		return nil, fmt.Errorf("libvirt: unable to verify existing domain %s: %w", name, err)
	}

	if dom == nil {
		return nil, ErrDomainNotFound
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
	defer dom.Free()

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
	defer dom.Free()

	// Collect storage volumes attached to the domain
	vols, err := d.getDomainVolumes(dom)
	if err != nil {
		return err
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

	// Now that the domain is destroyed, remove the associated volumes
	for _, vol := range vols {
		d.logger.Debug("deleting volume", "domain", name, "volume", vol)
		pool, err := d.storage.GetPool(vol.Pool)
		if err != nil {
			return err
		}

		if err := pool.DeleteVolume(vol.Name); err != nil {
			return err
		}
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
	if err != nil && !errors.Is(err, vm.ErrNotFound) {
		return err
	}

	if dom != nil {
		dom.Free()
		return fmt.Errorf("libvirt: %s: %w", config.Name, ErrDomainExists)
	}

	d.logger.Debug("domain doesn't exist, creating it", "name", config.Name)

	var domXML string
	if config.XMLConfig != "" {
		domXML = config.XMLConfig
	} else {
		domXML, err = d.parseConfiguration(config)
		if err != nil {
			return fmt.Errorf("libvirt: unable to parse domain configuration %s: %w", config.Name, err)
		}
	}

	d.logger.Debug("creating domain", "xml", domXML)

	dom, err = conn.DomainDefineXML(domXML)
	if err != nil {
		return fmt.Errorf("libvirt: unable to define domain %s: %w", config.Name, err)
	}
	defer dom.Free()

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
	defer dom.Free()

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
	defer dom.Free()

	xml, err := dom.GetXMLDesc(libvirtNoFlags)
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
	defer func() {
		for _, n := range allNetworks {
			n.Free()
		}
	}()

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
	defer func() {
		for _, d := range doms {
			d.Free()
		}
	}()

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
	return d.networking, nil
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

	// Add any fingerprint information from the storage subsystem
	d.storage.Fingerprint(attrs)

	return attrs, nil
}

// GenerateMountCommands implements virt.Virtualizer
func (d *driver) GenerateMountCommands(mounts []*vm.MountFileConfig) ([]string, error) {
	cmds := []string{}
	// read-only volumes are not supported in libvirt until 11.0.0
	virtiofsRO := d.requiresLibvirtVersion("11.0.0")
	for _, m := range mounts {
		// if the mount is read-only, only 9p is supported.
		if m.ReadOnly && !virtiofsRO {
			if !d.mountFsAvailable(mountFs9p) {
				return nil, fmt.Errorf("read-only virtiofs mount %w - libvirt version 11.0.0 or greater required", vm.ErrNotSupported)
			}

			cmds = append(cmds, d.generate9pMountCmds(m)...)
			continue
		}

		// Prefer using virtiofs if available
		if d.mountFsAvailable(mountFsVirtiofs) {
			cmds = append(cmds, d.generateVirtiofsMountCmds(m)...)
			continue
		}

		// Use 9pfs if virtiofs was not avaialble
		if d.mountFsAvailable(mountFs9p) {
			cmds = append(cmds, d.generate9pMountCmds(m)...)
			continue
		}

		// If here then no support filesystem detected
		return nil, fmt.Errorf("mounting %w - no supported filesystems available", vm.ErrNotSupported)
	}

	return cmds, nil
}

// GetCephSecret will return the ceph credential for the provided name
// base64 encoded.
func (d *driver) GetCephSecret(name string) (string, error) {
	conn, err := d.connection()
	if err != nil {
		return "", err
	}

	secret, err := conn.LookupSecretByUsage(libvirt.SECRET_USAGE_TYPE_CEPH, name)
	if err != nil {
		return "", err
	}
	defer secret.Free()

	val, err := secret.GetValue(libvirtNoFlags)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(val), nil
}

// GetCephSecretID will return the secret UUID if it exists.
func (d *driver) GetCephSecretID(name string) (string, error) {
	conn, err := d.connection()
	if err != nil {
		return "", err
	}

	secret, err := conn.LookupSecretByUsage(libvirt.SECRET_USAGE_TYPE_CEPH, name)
	if err != nil {
		return "", err
	}
	defer secret.Free()

	return secret.GetUUIDString()
}

// SetCephSecret will create or update a ceph credential returning the UUID
// for referencing the secret.
// NOTE: supplied credential is expected to be base64 encoded
func (d *driver) SetCephSecret(name, credential string) (string, error) {
	conn, err := d.connection()
	if err != nil {
		return "", err
	}

	decodedCred, err := base64.StdEncoding.DecodeString(credential)
	if err != nil {
		return "", err
	}

	// Attempt to locate the secret if it already exists
	secret, err := conn.LookupSecretByUsage(libvirt.SECRET_USAGE_TYPE_CEPH, name)
	if err != nil && !errors.Is(err, libvirt.ERR_NO_SECRET) {
		return "", err
	}

	// Be sure to free the secret if set on the way out
	defer func() {
		if secret != nil {
			secret.Free()
		}
	}()

	// If the secret is already registered, check and update secret if required
	// and return the identifier.
	if err == nil {
		value, err := secret.GetValue(libvirtNoFlags)
		if err != nil {
			return "", err
		}

		if !slices.Equal(value, decodedCred) {
			if err := secret.SetValue(decodedCred, libvirtNoFlags); err != nil {
				return "", err
			}
		}

		return secret.GetUUIDString()
	}

	secretDef := &libvirtxml.Secret{
		Ephemeral:   "no",
		Private:     "no",
		Description: "nomad-driver-virt storage pool credential",
		Usage: &libvirtxml.SecretUsage{
			Type: "ceph",
			Name: name,
		},
	}
	secretXml, err := secretDef.Marshal()
	if err != nil {
		return "", err
	}

	secret, err = conn.SecretDefineXML(secretXml, libvirt.SECRET_DEFINE_VALIDATE)
	if err != nil {
		return "", err
	}

	if err := secret.SetValue([]byte(credential), libvirtNoFlags); err != nil {
		return "", err
	}

	return secret.GetUUIDString()
}

// getDomainVolumes collects the list of storage volumes attached to the domain
func (d *driver) getDomainVolumes(dom *libvirt.Domain) ([]storage.Volume, error) {
	info := new(libvirtxml.Domain)
	if xmlDesc, err := dom.GetXMLDesc(libvirtNoFlags); err != nil {
		return nil, err
	} else if err := info.Unmarshal(xmlDesc); err != nil {
		return nil, err
	}

	if info.Devices == nil || len(info.Devices.Disks) == 0 {
		return []storage.Volume{}, nil
	}

	return d.storage.DiscoverVolumes(info.Devices.Disks)
}

// mountFsAvailable returns if the requested filesystems support is available.
func (d *driver) mountFsAvailable(name string) bool {
	_, ok := d.availableMountFs[name]
	return ok
}

// generateVirtiofsMountCmds generates mounts commands for virtiofs.
func (d *driver) generateVirtiofsMountCmds(m *vm.MountFileConfig) []string {
	m.Driver = mountFsVirtiofs

	return []string{
		fmt.Sprintf(`mkdir -p "%s"`, m.Destination),
		fmt.Sprintf(`mountpoint -q "%s" || mount -t virtiofs %s "%s"`, m.Destination, m.Tag, m.Destination),
	}
}

// generate9pMountCmds generates mount commands for 9pfs.
func (d *driver) generate9pMountCmds(m *vm.MountFileConfig) []string {
	m.Driver = mountFs9p

	return []string{
		fmt.Sprintf(`mkdir -p "%s"`, m.Destination),
		fmt.Sprintf(`mountpoint -q "%s" || mount -t 9p -o trans=virtio %s "%s"`, m.Destination, m.Tag, m.Destination),
	}
}

// findAvailableMountFs will check for available guest filesystem device
// support in the qemu emulator. It allows for overrides to be set which
// are used in testing.
func (d *driver) findAvailableMountFs() (map[string]struct{}, error) {
	// If direct override values are set, return those.
	if d.availableMountFsOverride != nil {
		return d.availableMountFsOverride, nil
	}

	// If a function override is set, call that.
	if mountFsAvailabilityOverride != nil {
		return mountFsAvailabilityOverride()
	}

	c, err := d.connection()
	if err != nil {
		return nil, err
	}

	capsRaw, err := c.GetCapabilities()
	if err != nil {
		return nil, err
	}

	caps := &libvirtxml.Caps{}
	if err := caps.Unmarshal(capsRaw); err != nil {
		return nil, err
	}

	avail := map[string]struct{}{}

	for _, guest := range caps.Guests {
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(guest.Arch.Emulator, "-device", "?")
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			d.logger.Error("qemu device list failure", "stderr", stderr.String())
			return nil, err
		}

		scanner := bufio.NewScanner(&stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, fmt.Sprintf(`name "%s"`, mountFs9p)) {
				avail[mountFs9p] = struct{}{}
			}
			if strings.HasPrefix(line, fmt.Sprintf(`name "%s"`, mountFsVirtiofs)) {
				avail[mountFsVirtiofs] = struct{}{}
			}
		}
	}

	return avail, nil
}

// requiresLibvirtVersion returns true if the version of libvirt
// is the same or newer than the provided version.
func (d *driver) requiresLibvirtVersion(version string) bool {
	return d.libvirtVersion >= genVersion(version)
}

const (
	majorDivisor = 1_000_000
	minorDivisor = 1_000
)

// genVersion will generate the version integer from
// the provided value. Requires version in format of
// MAJOR.MINOR.PATCH
func genVersion(version string) uint32 {
	var v uint32
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return 0
	}

	for i, div := range []int{majorDivisor, minorDivisor, 1} {
		p, err := strconv.Atoi(parts[i])
		if err != nil {
			return 0
		}
		v += uint32(p * div)
	}

	return v
}

// computeVersion will compute the version string from the
// integer value provided by libvirt.
func computeVersion(version uint32) string {
	major := version / majorDivisor
	version = version % majorDivisor
	minor := version / minorDivisor
	version = version % minorDivisor

	return fmt.Sprintf("%d.%d.%d", major, minor, version)
}

// ModifyMountFsAvailability is provided for testing to override the
// available filesystems for guest mounts.
func ModifyMountFsAvailability(fn mountFsAvailabilityFn) {
	logger := hclog.L()
	if fn != nil {
		logger.Warn("mount filesystem availability function override set")
	} else {
		logger.Warn("mount filesystem availability function override removed")
	}

	mountFsAvailabilityOverride = fn
}

type mountFsAvailabilityFn func() (map[string]struct{}, error)

// mountFsAvailabilityOverride is an override to manually set available guest
// filesystem support. It is used for testing and can be set outside of the
// package using the [ModifyMountFsAvailability] function.
var mountFsAvailabilityOverride mountFsAvailabilityFn

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

	"github.com/hashicorp/nomad-driver-virt/internal/errs"
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
	mountFsVirtiofs           = "vhost-user-fs-device"
	mountFs9p                 = "virtio-9p-device"
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

	Name = "libvirt" // Name of the provider.
)

var (
	ErrConnectionClosed = errors.New("libvirt connection is closed")
	ErrDomainExists     = errors.New("the domain exists already")
	ErrDomainNotFound   = fmt.Errorf("domain %w", errs.ErrNotFound)
	ErrPoolNotFound     = fmt.Errorf("storage pool %w", errs.ErrNotFound)

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

type provider struct {
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

	// insecureReadonlyMounts can be used to allow virtiofs to be used for read-only host
	// mounts even if unsupported by libvirt. This relies on the mount command only for
	// making the mount read-only, which is not secure due to the ability to remount
	// and remove the read-only option.
	insecureReadonlyMounts bool

	availableMountFsOverride map[string]struct{} // used for testing
}

// Copy creates a copy of this provider.
func (p *provider) Copy(ctx context.Context) *provider {
	p.m.Lock()
	defer p.m.Unlock()

	ctx, cancel := context.WithCancel(ctx)
	dCopy := &provider{
		ctx:                    ctx,
		cancel:                 cancel,
		closed:                 p.closed,
		uri:                    p.uri,
		logger:                 p.logger,
		user:                   p.user,
		password:               p.password,
		availableMountFs:       p.availableMountFs,
		libvirtVersion:         p.libvirtVersion,
		insecureReadonlyMounts: p.insecureReadonlyMounts,
	}
	dCopy.storage = p.storage.Copy(ctx, dCopy)
	dCopy.networking = p.networking.Copy(dCopy)

	go dCopy.monitorCtx()

	return dCopy
}

type Option func(*provider)

func WithConfig(c *Config) Option {
	return func(p *provider) {
		if c == nil {
			return
		}

		if c.URI != "" {
			p.uri = c.URI
		}
		if c.User != "" {
			p.user = c.User
		}
		if c.Password != "" {
			p.password = c.Password
		}
		if c.AllowInsecureMounts {
			p.insecureReadonlyMounts = true
		}
	}
}

func WithConnectionURI(URI string) Option {
	return func(p *provider) {
		p.uri = URI
	}
}

func WithAuth(user, password string) Option {
	return func(p *provider) {
		p.user = user
		p.password = password
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

func New(ctx context.Context, logger hclog.Logger, opt ...Option) *provider {
	ctx, cancel := context.WithCancel(ctx)
	p := &provider{
		ctx:    ctx,
		logger: logger.Named("libvirt"),
		uri:    defaultURI,
		opts:   opt,
		cancel: cancel,
	}
	p.networking = libvirtnet.NewController(p.logger, p)

	for _, opt := range p.opts {
		opt(p)
	}

	go p.monitorCtx()

	return p
}

// connection returns a working connection to the libvirt daemon
func (p *provider) connection() (*libvirt.Connect, error) {
	p.m.Lock()
	defer p.m.Unlock()

	// if marked as closed, no new connections should established
	if p.closed {
		return nil, ErrConnectionClosed
	}

	if p.conn != nil {
		alive, err := p.conn.IsAlive()
		if alive {
			return p.conn, nil
		}

		if err != nil {
			p.logger.Warn("error on connection alive check", "error", err)
		}

		// it's not alive so close it to free the connection resources
		p.conn.Close()
	}

	var err error
	p.conn, err = newConnection(p.uri, p.user, p.password)
	if err != nil {
		return nil, err
	}

	return p.conn, nil
}

// monitorCtx monitors the context and once done closes
// the connection and marks the provider as closed
func (p *provider) monitorCtx() {
	<-p.ctx.Done()
	p.m.Lock()
	defer p.m.Unlock()

	p.closed = true
	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
	}
}

// Close closes the connection to libvirtp.
func (p *provider) Close() error {
	p.m.Lock()
	defer p.m.Unlock()
	var err error

	// If the connection is still available, close it
	// and unset it
	if p.conn != nil {
		_, err = p.conn.Close()
		p.conn = nil
	}

	// Mark as closed
	p.closed = true

	// Cancel the context to stop the monitor
	p.cancel()

	return err
}

// Init initializes the provider.
// implements virt.Virtualizer
func (p *provider) Init() error {
	c, err := p.connection()
	if err != nil {
		return err
	}

	p.libvirtVersion, err = c.GetVersion()
	if err != nil {
		p.logger.Debug("unable to get libvirt version", "error", err)
		return err
	}

	// Determine what filesystems are supported for mounting within
	// the guest.
	if p.availableMountFs, err = p.findAvailableMountFs(); err != nil {
		return err
	}

	return nil
}

// CreateVM creates new virtual machine using the provider configuration.
// implements virt.Virtualizer
func (p *provider) CreateVM(config *vm.Config) error {
	conn, err := p.connection()
	if err != nil {
		return err
	}

	dom, err := p.getDomain(config.Name)
	if err != nil && !errors.Is(err, errs.ErrNotFound) {
		return err
	}

	if dom != nil {
		dom.Free()
		return fmt.Errorf("libvirt: %s: %w", config.Name, ErrDomainExists)
	}

	p.logger.Debug("domain doesn't exist, creating it", "name", config.Name)

	var domXML string
	if config.XMLConfig != "" {
		domXML = config.XMLConfig
	} else {
		domXML, err = p.parseConfiguration(config)
		if err != nil {
			return fmt.Errorf("libvirt: unable to parse domain configuration %s: %w", config.Name, err)
		}
	}

	p.logger.Debug("creating domain", "xml", domXML)

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

// StopVM stops the named virtual machine. The domain will be shutoff, but will still
// be present as inactive and can be restartep.
// implements virt.Virtualizer
func (p *provider) StopVM(name string) error {
	p.logger.Warn("stopping domain", "name", name)

	dom, err := p.getDomain(name)
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

// DestroyVM destroys the named virtual machine.
// implements virt.Virtualizer
func (p *provider) DestroyVM(name string) error {
	p.logger.Warn("destroying domain", "name", name)

	dom, err := p.getDomain(name)
	if err != nil {
		return err
	}
	defer dom.Free()

	// Collect storage volumes attached to the domain
	vols, err := p.getDomainVolumes(dom)
	if err != nil {
		return err
	}

	err = dom.Destroy()
	if err != nil {
		// In case we want to destroy a domain that was previoulsy stopped, destroy
		// is not idempotent and will throw the error operation invalid if the
		// domain is already stoppep.
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
		p.logger.Debug("deleting volume", "domain", name, "volume", vol)
		pool, err := p.storage.GetPool(vol.Pool)
		if err != nil {
			return err
		}

		if err := pool.DeleteVolume(vol.Name); err != nil {
			return err
		}
	}

	return nil
}

// GetVM gets information abou the name virtual machine.
// implements virt.Virtualizer
func (p *provider) GetVM(name string) (*vm.Info, error) {
	dom, err := p.getDomain(name)
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

// GetInfo returns information about this virtualization provider.
// implements virt.Virtualizer
func (p *provider) GetInfo() (vm.VirtualizerInfo, error) {
	li := vm.VirtualizerInfo{}

	conn, err := p.connection()
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

// GetNetworkInterfaces returns the network interfaces for the named virtual machine.
// Interfaces information population is best effort, as not all information will be
// available depending on the state of the vm.
// implements virt.Virtualizer
func (p *provider) GetNetworkInterfaces(name string) ([]vm.NetworkInterface, error) {
	dom, err := p.getDomain(name)
	if err != nil {
		p.logger.Error("cannot get domain", "domain", name, "error", err)
		return nil, fmt.Errorf("libvirt: unable to get domain %s: %w", name, err)
	}
	defer dom.Free()

	xml, err := dom.GetXMLDesc(libvirtNoFlags)
	if err != nil {
		p.logger.Error("cannot get domain XML", "domain", name, "error", err)
		return nil, fmt.Errorf("libvirt: unable to get domain XML %s: %w", name, err)
	}

	dxml := &libvirtxml.Domain{}
	if err := dxml.Unmarshal(xml); err != nil {
		p.logger.Error("cannot parse domain XML", "domain", name, "error", err)
		return nil, fmt.Errorf("libvirt: unable to parse domain XML %s: %w", name, err)
	}

	interfaces := make([]vm.NetworkInterface, len(dxml.Devices.Interfaces))

	allNetworks, err := p.conn.ListAllNetworks(libvirt.CONNECT_LIST_NETWORKS_ACTIVE)
	if err != nil {
		p.logger.Error("cannot list available networks", "domain", name, "error", err)
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
			p.logger.Debug("failed to get network name", "domain", name, "error", err)
			continue
		}
		bridge, err := network.GetBridgeName()
		if err != nil {
			p.logger.Debug("failed to get network bridge", "domain", name, "network", netname,
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
				p.logger.Debug("no matching network found for bridge", "domain", name,
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
				p.logger.Warn("failed to parse interface address",
					"domain", name, "address", addr.Address, "error", err)
			}
		}

		p.logger.Debug("domain network interface retrieved", "domain", name,
			"interface", interfaces[i])
	}

	return interfaces, nil
}

// UseCloudInit informs that cloud-init is supported by this provider.
// implements virt.Virtualizer
func (p *provider) UseCloudInit() bool {
	return true
}

// Networking returns the virtualization network subsystem.
// implements virt.Virtualizer
func (p *provider) Networking() (virtnet.Net, error) {
	return p.networking, nil
}

// Fingerprint generates the fingerprint attributes for this provider.
// implements virt.Virtualizer
func (p *provider) Fingerprint() (map[string]*structs.Attribute, error) {
	conn, err := p.connection()
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

	n, err := p.Networking()
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
	p.storage.Fingerprint(attrs)

	return attrs, nil
}

// SetupStorage prepares the configured storage pools for usage.
// implements virt.Virtualizer
func (p *provider) SetupStorage(config *storage.Config) error {
	if config == nil {
		return fmt.Errorf("%w: missing storage pool configuration", errs.ErrInvalidConfiguration)
	}

	s, err := libvirt_storage.New(p.ctx, p.logger, p, config)
	if err != nil {
		return err
	}

	p.storage = s
	return nil
}

// Storage returns the storage interface
// implements virt.Virtualizer
func (p *provider) Storage() storage.Storage {
	return p.storage
}

// GenerateMountCommands generates the commands required to mount the provided
// mount configurations in the virtual machine. It will also set the driver
// on the MountFileConfig based on file system usep.
// implements virt.Virtualizer
func (p *provider) GenerateMountCommands(mounts []*vm.MountFileConfig) ([]string, error) {
	cmds := []string{}
	// read-only volumes are not supported in libvirt until 11.0.0
	virtiofsRO := p.requiresLibvirtVersion("11.0.0")
	// if virtiofs does not support read-only mounts, check if insecure
	// mounts have been enabled to bypass the version restriction.
	if !virtiofsRO && p.insecureReadonlyMounts {
		p.logger.Warn("configuration allows insecure read-only host mounts")
		virtiofsRO = true
	}

	for _, m := range mounts {
		// if the mount is read-only, only 9p is supported (unless insecure mounts enabled).
		if m.ReadOnly && !virtiofsRO {
			if !p.mountFsAvailable(mountFs9p) {
				return nil, fmt.Errorf("read-only virtiofs mount %w - libvirt version 11.0.0 or greater required", errs.ErrNotSupported)
			}

			cmds = append(cmds, p.generate9pMountCmds(m)...)
			continue
		}

		// Prefer using virtiofs if available
		if p.mountFsAvailable(mountFsVirtiofs) {
			cmds = append(cmds, p.generateVirtiofsMountCmds(m)...)
			continue
		}

		// Use 9pfs if virtiofs was not avaialble
		if p.mountFsAvailable(mountFs9p) {
			cmds = append(cmds, p.generate9pMountCmds(m)...)
			continue
		}

		// If here then no support filesystem detected
		return nil, fmt.Errorf("mounting %w - no supported filesystems available", errs.ErrNotSupported)
	}

	return cmds, nil
}

// NewStream creates a new libvirt stream
// NOTE: caller is responsible to free result
func (p *provider) NewStream() (shims.Stream, error) {
	c, err := p.connection()
	if err != nil {
		return nil, err
	}

	s, err := c.NewStream(libvirtNoFlags)
	if err != nil {
		return nil, err
	}

	// Sparse uploads were broken starting with the 9.6.0 release resulting in
	// the client connection being throttled and the upload timing out. It was
	// fixed in 10.1.0. Unless we really need to support sparse uploads for
	// pre-9.6.0 releases, just check that libvirt is recent enough.
	sparseSupported := p.requiresLibvirtVersion("10.1.0")
	if !sparseSupported {
		p.logger.Debug("sparse stream disabled due to libvirt version")
	}

	return shims.WrapStream(s, p.ctx, sparseSupported), nil
}

// FindStoragePool finds the named storage pool
// NOTE: caller is responsible to free result
func (p *provider) FindStoragePool(name string) (shims.StoragePool, error) {
	conn, err := p.connection()
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
func (p *provider) CreateStoragePool(def *libvirtxml.StoragePool) (shims.StoragePool, error) {
	conn, err := p.connection()
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
	// on after the libvirt service is restartep.
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
func (p *provider) UpdateStoragePool(def *libvirtxml.StoragePool) error {
	conn, err := p.connection()
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

// ListNetworks lists the available networks to libvirt
func (p *provider) ListNetworks() ([]string, error) {
	conn, err := p.connection()
	if err != nil {
		return nil, err
	}

	return conn.ListNetworks()
}

// LookupNetworkByName looks up a network by its name
// NOTE: caller is responsible to free result
func (p *provider) LookupNetworkByName(name string) (shims.ConnectNetwork, error) {
	conn, err := p.connection()
	if err != nil {
		return nil, err
	}

	return conn.LookupNetworkByName(name)
}

// GetAllDomains returns the list of all active domains.
func (p *provider) GetAllDomains() ([]string, error) {
	conn, err := p.connection()
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
			p.logger.Error("unable to free domain", "name", name)
		}
	}

	return dns, nil
}

// GetCephSecret will return the ceph credential for the provided name
// base64 encodep.
func (p *provider) GetCephSecret(name string) (string, error) {
	conn, err := p.connection()
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
func (p *provider) GetCephSecretID(name string) (string, error) {
	conn, err := p.connection()
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
func (p *provider) SetCephSecret(name, credential string) (string, error) {
	conn, err := p.connection()
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

	if err := secret.SetValue(decodedCred, libvirtNoFlags); err != nil {
		return "", err
	}

	return secret.GetUUIDString()
}

// getDomain looks up a domain by name, if an error ocurred, it will be returnep.
// NOTE: caller is responsible to free result
func (p *provider) getDomain(name string) (*libvirt.Domain, error) {
	conn, err := p.connection()
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

// getDomainVolumes collects the list of storage volumes attached to the domain
func (p *provider) getDomainVolumes(dom *libvirt.Domain) ([]storage.Volume, error) {
	info := new(libvirtxml.Domain)
	if xmlDesc, err := dom.GetXMLDesc(libvirtNoFlags); err != nil {
		return nil, err
	} else if err := info.Unmarshal(xmlDesc); err != nil {
		return nil, err
	}

	if info.Devices == nil || len(info.Devices.Disks) == 0 {
		return []storage.Volume{}, nil
	}

	return p.storage.DiscoverVolumes(info.Devices.Disks)
}

// mountFsAvailable returns if the requested filesystems support is available.
func (p *provider) mountFsAvailable(name string) bool {
	_, ok := p.availableMountFs[name]
	return ok
}

// generateVirtiofsMountCmds generates mounts commands for virtiofs.
func (p *provider) generateVirtiofsMountCmds(m *vm.MountFileConfig) []string {
	m.Driver = mountFsVirtiofs

	var readonly string
	if m.ReadOnly {
		readonly = " -o ro"
	}

	return []string{
		fmt.Sprintf(`mkdir -p "%s"`, m.Destination),
		fmt.Sprintf(`mountpoint -q "%s" || mount -t virtiofs%s %s "%s"`, m.Destination, readonly, m.Tag, m.Destination),
	}
}

// generate9pMountCmds generates mount commands for 9pfs.
func (p *provider) generate9pMountCmds(m *vm.MountFileConfig) []string {
	m.Driver = mountFs9p

	var readonly string
	if m.ReadOnly {
		readonly = ",ro"
	}
	return []string{
		fmt.Sprintf(`mkdir -p "%s"`, m.Destination),
		fmt.Sprintf(`mountpoint -q "%s" || mount -t 9p -o trans=virtio%s %s "%s"`, m.Destination, readonly, m.Tag, m.Destination),
	}
}

// findAvailableMountFs will check for available guest filesystem device
// support in the qemu emulator. It allows for overrides to be set which
// are used in testing.
func (p *provider) findAvailableMountFs() (map[string]struct{}, error) {
	// If direct override values are set, return those.
	if p.availableMountFsOverride != nil {
		return p.availableMountFsOverride, nil
	}

	// If a function override is set, call that.
	if mountFsAvailabilityOverride != nil {
		return mountFsAvailabilityOverride()
	}

	c, err := p.connection()
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
			p.logger.Error("qemu device list failure", "stderr", stderr.String())
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
func (p *provider) requiresLibvirtVersion(version string) bool {
	return p.libvirtVersion >= genVersion(version)
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

	mountFsAvailabilityLock.Lock()
	mountFsAvailabilityOverride = fn
	mountFsAvailabilityLock.Unlock()
}

type mountFsAvailabilityFn func() (map[string]struct{}, error)

// mountFsAvailabilityOverride is an override to manually set available guest
// filesystem support. It is used for testing and can be set outside of the
// package using the [ModifyMountFsAvailability] function.
var (
	mountFsAvailabilityOverride mountFsAvailabilityFn
	mountFsAvailabilityLock     sync.Mutex
)

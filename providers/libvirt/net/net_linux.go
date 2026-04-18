// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package net

import (
	"errors"
	"fmt"
	stdnet "net"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coreos/go-iptables/iptables"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/go-set"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/shared/structs"
	lv "libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

const (
	// postroutingIPTablesChainName is the IPTables chain name used by the
	// driver for postrouting rules. This is currently used for entries within
	// the nat table specifically for handling the special case of loopback
	// addresses.
	postroutingIPTablesChainName = "NOMAD_VT_PST"

	// preroutingIPTablesChainName is the IPTables chain name used by the
	// driver for prerouting rules. This is currently used for entries within
	// the nat table.
	preroutingIPTablesChainName = "NOMAD_VT_PRT"

	// forwardIPTablesChainName is the IPTables chain name used by the driver
	// for forwarding rules. This is currently used for entries within the
	// filter table.
	forwardIPTablesChainName = "NOMAD_VT_FW"

	// outputIPTablesChainName is the IPTables chain name used by the driver
	// for output rules. This is currently used for entries within the nat
	// table specifically for handling the special case of loopback addresses.
	outputIPTablesChainName = "NOMAD_VT_OUT"

	// iptablesNATTableName is the name of the nat table within iptables.
	iptablesNATTableName = "nat"

	// iptablesFilterTableName is the name of the filter table within iptables.
	iptablesFilterTableName = "filter"

	// automaticParentIndex is the special libvirt value to automatically
	// determine placement when adding an element.
	automaticParentIndex = -1

	// dhcpServerPort is the port the DHCP server is listening on.
	dhcpServerPort = "67"

	// routeLocalnetPathTemplate is the template for generating the path to check for device specific routing support.
	routeLocalnetPathTemplate = "/proc/sys/net/ipv4/conf/%s/route_localnet"

	// routeLocalnetGlobalName is the name of the global kernel configuration for localnet routing.
	routeLocalnetGlobalName = "all"
)

// initLock is used to prevent multiple controller.Init's running at once.
var initLock sync.Mutex

// loopbackForwardsLock is used to prevent multiple loopback forwards enablements
// and loopback device enablements from running at once.
var loopbackForwardsLock sync.Mutex

// Copy returns a new copy of the controller.
func (c *Controller) Copy(conn shims.Connect) *Controller {
	return &Controller{
		dhcpLeaseDiscoveryInterval: c.dhcpLeaseDiscoveryInterval,
		dhcpLeaseDiscoveryTimeout:  c.dhcpLeaseDiscoveryTimeout,
		interfaceByIPGetter:        c.interfaceByIPGetter,
		ipByInterfaceGetter:        c.ipByInterfaceGetter,
		iptablesInterfaceGetter:    c.iptablesInterfaceGetter,
		logger:                     c.logger,
		netConn:                    conn,
		routingInterfaceByIPGetter: c.routingInterfaceByIPGetter,
		routeLocalnetTemplate:      c.routeLocalnetTemplate,
	}
}

func (c *Controller) Fingerprint(attr map[string]*structs.Attribute) {

	// List the network names. This is terminal to the fingerprint process, as
	// without this, we have nothing to query.
	networkNames, err := c.netConn.ListNetworks()
	if err != nil {
		c.logger.Error("failed to list networks", "error", err)
		return
	}

	// Iterate the list of network names getting a network handle, so we can
	// query whether it is active.
	for _, networkName := range networkNames {

		networkInfo, err := c.netConn.LookupNetworkByName(networkName)
		if err != nil {
			c.logger.Error("failed to lookup network",
				"network", networkName, "error", err)
			continue
		}
		defer networkInfo.Free()

		active, err := networkInfo.IsActive()
		if err != nil {
			c.logger.Error("failed to check network state",
				"network", networkName, "error", err)
			continue
		}

		// Populate the attributes mapping with our network state. Libvirt does
		// not allow two networks of the same name, so there should be no
		// concern about overwriting or duplicates.
		netStateKey := net.FingerprintAttributeKeyPrefix + networkName + ".state"
		attr[netStateKey] = structs.NewStringAttribute(net.IsActiveString(active))

		bridgeName, err := networkInfo.GetBridgeName()
		if err != nil {
			c.logger.Error("failed to get network bridge name",
				"network", networkName, "error", err)
			continue
		}

		// Populate the attributes mapping with our bridge name. Only one
		// bridge can be configured per network, so there should be no concern
		// about overwriting or duplicates.
		netBridgeNameKey := net.FingerprintAttributeKeyPrefix + networkName + ".bridge_name"
		attr[netBridgeNameKey] = structs.NewStringAttribute(bridgeName)
	}
}

func (c *Controller) Init() error {
	var err error
	initLock.Lock()
	defer initLock.Unlock()

	err = c.ensureIPTables()

	return err
}

// ensureIPTables is responsible for ensuring the local host machine iptables
// are configured with the chains and rules needed by the driver.
//
// On a new machine, this function creates the "NOMAD_VT_PRT" and "NOMAD_VT_FW"
// chains. The "NOMAD_VT_PRT" chain then has a jump rule added to the "nat"
// table; the "NOMAD_VT_FW" chain has a jump rule added to the "filter" table.
func (c *Controller) ensureIPTables() error {

	ipt, err := c.iptablesInterfaceGetter()
	if err != nil {
		return fmt.Errorf("failed to create iptables handle: %w", err)
	}

	// Ensure the NAT prerouting chain is available and create the jump rule if
	// needed.
	natCreated, err := ensureIPTablesChain(ipt, iptablesNATTableName, preroutingIPTablesChainName)
	if err != nil {
		return fmt.Errorf("failed to create iptables chain %q: %w",
			preroutingIPTablesChainName, err)
	}
	if natCreated {
		if err := ipt.Insert(iptablesNATTableName, "PREROUTING", 1, []string{"-j", preroutingIPTablesChainName}...); err != nil {
			return err
		}
		c.logger.Info("successfully created NAT prerouting iptables chain",
			"name", preroutingIPTablesChainName)
	}

	// Ensure the filter forward chain is available and create the jump rule if
	// needed.
	filterCreated, err := ensureIPTablesChain(ipt, iptablesFilterTableName, forwardIPTablesChainName)
	if err != nil {
		return fmt.Errorf("failed to create iptables chain %q: %w",
			forwardIPTablesChainName, err)
	}
	if filterCreated {
		if err := ipt.Insert(iptablesFilterTableName, "FORWARD", 1, []string{"-j", forwardIPTablesChainName}...); err != nil {
			return err
		}
		c.logger.Info("successfully created filter forward iptables chain",
			"name", forwardIPTablesChainName)
	}

	return nil
}

func ensureIPTablesChain(ipt IPTables, table, chain string) (bool, error) {

	// List and iterate the currently configured iptables chains, so we can
	// identify whether the chain already exist.
	chains, err := ipt.ListChains(table)
	if err != nil {
		return false, err
	}
	for _, ch := range chains {
		if ch == chain {
			return false, nil
		}
	}

	err = ipt.NewChain(table, chain)

	// The error returned needs to be carefully checked as an exit code of 1
	// indicates the chain exists. This might happen when another routine has
	// created it.
	var e *iptables.Error

	if errors.As(err, &e) && e.ExitStatus() == 1 {
		return false, nil
	}

	return true, err
}

func (c *Controller) VMStartedBuild(req *net.VMStartedBuildRequest) (*net.VMStartedBuildResponse, error) {
	if req == nil {
		return nil, errors.New("net controller: no request provided")
	}
	if req.NetConfig == nil || req.Resources == nil {
		return &net.VMStartedBuildResponse{}, nil
	}

	// Dereference the network config and pull out the interface detail. The
	// driver only supports a single interface currently, so this is safe to
	// do, but when multi-interface support is added, this will need to change.
	netConfig := req.NetConfig

	// Protect against VMs with no network interface. The log is useful for
	// debugging which certainly caught me(jrasell) a few times in development.
	if len(netConfig) == 0 {
		c.logger.Debug("no network interface configured", "domain", req.VMName)
		return &net.VMStartedBuildResponse{}, nil
	}
	netInterface := netConfig[0]

	networkName, err := c.networkNameFromBridgeName(netInterface.Bridge.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to discover network: %w", err)
	}

	network, err := c.netConn.LookupNetworkByName(networkName)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup network: %w", err)
	}
	defer network.Free()

	ipAddr, macAddr, err := c.discoverDHCPLeaseIP(network, req.Hostname, networkName, req.Hwaddrs)
	if err != nil {
		return nil, fmt.Errorf("failed to discover IP address: %w", err)
	}

	// Register the IP to the domain to ensure it does not change.
	dhcpEntry, err := c.reserveIP(network, ipAddr, req.Hostname, macAddr)
	if err != nil {
		c.logger.Warn("failed to reserve IP address", "network", network, "address", ipAddr, "hostname",
			req.Hostname, "mac", macAddr, "error", err)
	}

	teardownRules, err := c.configureIPTables(req.Resources, netInterface.Bridge, ipAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to configure port mapping: %w", err)
	}

	return &net.VMStartedBuildResponse{
		DriverNetwork: &drivers.DriverNetwork{
			IP: ipAddr,
		},
		TeardownSpec: &net.TeardownSpec{
			IPTablesRules:   teardownRules,
			DHCPReservation: dhcpEntry,
			Network:         networkName,
		},
	}, nil
}

// reserveIP reserves an IP address with the DHCP server for a specific domain
// based on MAC address and hostname.
func (c *Controller) reserveIP(network shims.ConnectNetwork, ipAddr, hostname, mac string) (string, error) {
	reservation := libvirtxml.NetworkDHCPHost{
		IP:   ipAddr,
		MAC:  mac,
		Name: hostname,
	}

	c.logger.Debug("adding dhcp reservation", "reservation", reservation)

	entry, err := reservation.Marshal()
	if err != nil {
		return "", err
	}

	err = network.Update(lv.NETWORK_UPDATE_COMMAND_ADD_LAST, lv.NETWORK_SECTION_IP_DHCP_HOST,
		automaticParentIndex, entry, lv.NETWORK_UPDATE_AFFECT_LIVE|lv.NETWORK_UPDATE_AFFECT_CONFIG)

	if err != nil {
		return "", fmt.Errorf("failed to update network: %w", err)
	}

	return entry, nil
}

// networkNameFromBridgeName translates the name of a bridge network interface
// to a libvirt network name. Operators only need to specify the interface name
// when creating VMs, but we need the network name.
func (c *Controller) networkNameFromBridgeName(name string) (string, error) {

	networkNames, err := c.netConn.ListNetworks()
	if err != nil {
		return "", err
	}

	for _, networkName := range networkNames {

		networkInfo, err := c.netConn.LookupNetworkByName(networkName)
		if err != nil {
			return "", err
		}
		defer networkInfo.Free()

		brdigeName, err := networkInfo.GetBridgeName()
		if err != nil {
			return "", err
		}

		if brdigeName == name {
			return networkName, nil
		}
	}

	return "", fmt.Errorf("failed to find network with bridge %q", name)
}

// discoverDHCPLeaseIP identifies the IP assigned to the named VM on the named
// network. The function includes a ticker in order to poll for the information
// as this can take several seconds to become available.
func (c *Controller) discoverDHCPLeaseIP(
	network shims.ConnectNetwork, hostname, netName string, hwaddrs []string) (ipAddr string, macAddr string, err error) {

	ticker := time.NewTicker(c.dhcpLeaseDiscoveryInterval)
	defer ticker.Stop()

	timeout := time.NewTimer(c.dhcpLeaseDiscoveryTimeout)
	defer timeout.Stop()

	macs := set.From(hwaddrs)
	for {
		select {
		case <-ticker.C:

			// If we do not log, the driver and Nomad seem to stall from the
			// user perspective, which might be off-putting. Providing some
			// debug entry while performing this "long-lived" process should
			// help operators understand what is happening.
			c.logger.Debug("attempting DHCP lease discovery",
				"hostname", hostname, "network_name", netName,
				"hwaddrs", hwaddrs)

			// Lookup the DHCP leases of the network. If we receive any error,
			// log and try again. If it is transient error, we will find the
			// information on the next try, otherwise the timeout acts as our
			// retry cutoff.
			dhcpLeases, err := network.GetDHCPLeases()
			if err != nil {
				c.logger.Warn("failed to lookup DHCP leases",
					"network_name", netName, "error", err)
				continue
			}

			matches := []lv.NetworkDHCPLease{}

			// Gather all matching leases
			for _, lease := range dhcpLeases {
				// Check if lease matches any available interfaces on the domain.
				if !macs.Contains(lease.Mac) {
					continue
				}

				// Check if the hostname is set, and matches
				if lease.Hostname != "" && lease.Hostname != hostname {
					continue
				}

				// Only want to add leases that are still valid.
				if lease.ExpiryTime.Before(time.Now()) {
					continue
				}

				c.logger.Debug("DHCP lease detected", "hostname", hostname, "network_name", netName,
					"hwaddrs", hwaddrs, "lease", lease)
				matches = append(matches, lease)
			}

			if len(matches) == 0 {
				continue
			}

			// If any matches were found, sort them in descending order
			// by the lease expiry date and return the address with the
			// expiry furthest in the future. This is done to handle situations
			// where an interface's MAC address is being set and the instance
			// has been destroyed and created again resulting in multiple
			// leases for the same MAC.
			slices.SortFunc(matches, func(a, b lv.NetworkDHCPLease) int {
				return b.ExpiryTime.Compare(a.ExpiryTime)
			})

			lease := matches[len(matches)-1]

			return lease.IPaddr, lease.Mac, nil

		case <-timeout.C:
			return "", "", fmt.Errorf("timeout reached discovering DHCP lease for %q", hostname)
		}
	}
}

// enableLoopbackPortForwards validates that the host system is configured for routing
// localnet packets and adds the required chains.
func (c *Controller) enableLoopbackPortForwards() error {
	loopbackForwardsLock.Lock()
	defer loopbackForwardsLock.Unlock()

	ipt, err := c.iptablesInterfaceGetter()
	if err != nil {
		return err
	}

	// Add the output chain if it does not exist.
	outputCreated, err := ensureIPTablesChain(ipt, iptablesNATTableName, outputIPTablesChainName)
	if err != nil {
		return err
	}
	if outputCreated {
		if err := ipt.Insert(iptablesNATTableName, "OUTPUT", 1, []string{"-j", outputIPTablesChainName}...); err != nil {
			return err
		}

		c.logger.Info("successfully created NAT output iptables chain", "name", outputIPTablesChainName)
	}

	// Add the postrouting chain if it does not exist.
	postroutingCreated, err := ensureIPTablesChain(ipt, iptablesNATTableName, postroutingIPTablesChainName)
	if err != nil {
		return err
	}
	if postroutingCreated {
		if err := ipt.Insert(iptablesNATTableName, "POSTROUTING", 1, []string{"-j", postroutingIPTablesChainName}...); err != nil {
			return err
		}

		c.logger.Info("successfully created NAT postrouting iptables chain", "name", postroutingIPTablesChainName)
	}

	// Log a warning that loopback port forwarding is being enabled.
	c.logger.Warn("port forwarding for the local loopback is enabled")
	return nil
}

// loopbackPortForwardsSupported returns if the host has been configured for routing localnet packets.
// NOTE: The global configuration overrides device specific configuration.
func (c *Controller) loopbackPortForwardsSupported(device string) bool {
	for _, configName := range []string{routeLocalnetGlobalName, device} {
		tmpl := c.routeLocalnetTemplate
		if tmpl == "" {
			tmpl = routeLocalnetPathTemplate
		}

		cfgPath := fmt.Sprintf(tmpl, configName)
		content, err := os.ReadFile(cfgPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			c.logger.Error("read failed for device loopback support check", "path", cfgPath, "error", err)
			return false
		}

		if strings.TrimSpace(string(content)) == "1" {
			return true
		}
	}

	return false
}

// loopbackPortForwardsEnabled checks if port forwards are enabled for the loopback device.
func (c *Controller) loopbackPortForwardsEnabled(ipt IPTables) bool {
	chains, err := ipt.ListChains(iptablesNATTableName)
	if err != nil {
		c.logger.Error("could not list table chains", "table", iptablesNATTableName,
			"chain", postroutingIPTablesChainName, "error", err)
		return false
	}

	return slices.Contains(chains, postroutingIPTablesChainName)
}

// enableLoopbackForDevice will add the required rule to the postrouting chain
// for the device to handle port forwards from the loopback device.
func (c *Controller) enableLoopbackForDevice(ipt IPTables, device string) error {
	loopbackForwardsLock.Lock()
	defer loopbackForwardsLock.Unlock()

	// Create the required rule.
	rule := []string{
		"-o", device,
		"-m", "addrtype",
		"--src-type", "LOCAL",
		"--dst-type", "UNICAST",
		"-j", "MASQUERADE",
	}

	// Check if the rule already exists.
	existingRules, err := ipt.List(iptablesNATTableName, postroutingIPTablesChainName)
	if err != nil {
		return fmt.Errorf("failed to list postrouting rules: %w", err)
	}

	for _, existingRule := range existingRules {
		// Match the suffix of the existing rule since chain information is not included
		// in the new rule.
		if strings.HasSuffix(existingRule, strings.Join(rule, " ")) {
			return nil
		}
	}

	// Still here, then add the rule.
	if err := ipt.Append(iptablesNATTableName, postroutingIPTablesChainName, rule...); err != nil {
		return fmt.Errorf("failed to add loopback rule for device: %w", err)
	}

	return nil
}

// configureIPTables is responsible for adding the iptables entries to enable
// port mapping. The function will perform this action for all configured ports
// within the network interface configuration.
//
// The returned array contains the added rules which hopes to make it easier to
// delete rules when a task is stopped, specifically by avoiding having to
// generate the information again.
//
// TODO (jrasell) it is possible an error occurs after we have configured
// a number of iptables entries. The function should have a rollback mechanism
// to clear up, so we do not leak iptables rules. This requires testing, which
// is tricky, thus is still a todo.
func (c *Controller) configureIPTables(
	res *drivers.Resources, cfg *net.NetworkInterfaceBridgeConfig, ip string) ([][]string, error) {

	var teardownRules [][]string

	ipt, err := c.iptablesInterfaceGetter()
	if err != nil {
		return nil, fmt.Errorf("failed to create iptables handle: %w", err)
	}

	// Create lookup mapping for ip:interface-name, so we can cache reads of
	// this and not have to perform the translation each time.
	interfaceMapping := make(map[string]string)

	// Iterate the ports configured within the network interface and pull these
	// from the task allocated ports.
	for _, port := range cfg.Ports {

		reservedPort, ok := res.Ports.Get(port)
		if !ok {
			c.logger.Error("failed to find reserved port", "port", port)
			continue
		}

		// Look into the mapping for the interface based on the host IP,
		// otherwise perform the more expensive actual lookup by querying the
		// host.
		iface, ok := interfaceMapping[reservedPort.HostIP]
		if !ok {
			iface, err = c.interfaceByIPGetter(stdnet.ParseIP(reservedPort.HostIP))
			if err != nil {
				return nil, fmt.Errorf("failed to identify IP interface: %w", err)
			}

			interfaceMapping[reservedPort.HostIP] = iface
		}

		// Generate our NAT preroute arguments to include the table and chain
		// information. This allows us to store all the detail within the
		// teardownRules easily.
		var preRouteArgs []string

		hostIP, err := netip.ParseAddr(reservedPort.HostIP)
		if err != nil {
			return nil, fmt.Errorf("failed to parse host IP address: %w", err)
		}

		// If the host IP provided is a loopback, it needs to be picked up on the
		// output chain and redirected to the VM. This is a special case and which
		// requires the host to be properly configured.
		if hostIP.IsLoopback() {
			// Find the interface that the destination address is attached.
			dstIface, ok := interfaceMapping[ip]
			if !ok {
				dstIface, err = c.routingInterfaceByIPGetter(ip)
				if err != nil {
					return nil, fmt.Errorf("failed to identify IP interface: %w", err)
				}

				interfaceMapping[ip] = dstIface
			}

			// Check if loopback port forwarding is even available before starting.
			if !c.loopbackPortForwardsSupported(dstIface) {
				c.logger.Error(fmt.Sprintf("loopback port forwarding requires kernel runtime configuration - net.ipv4.conf.%s.route_localnet=1", dstIface))
				return nil, fmt.Errorf("loopback port forwarding not enabled for device - %s", dstIface)
			}

			// Ensure loopback port forwarding is setup
			if err := c.enableLoopbackPortForwards(); err != nil {
				return nil, fmt.Errorf("failed to enable loopback port forwarding: %w", err)
			}

			// Enable the device to handle requests from the loopback device.
			if err := c.enableLoopbackForDevice(ipt, dstIface); err != nil {
				return nil, err
			}

			preRouteArgs = []string{
				iptablesNATTableName,
				outputIPTablesChainName,
				"-s", reservedPort.HostIP,
				"-o", iface,
				"-p", "tcp",
				"-m", "tcp",
				"--dport", strconv.Itoa(reservedPort.Value),
				"-j", "DNAT",
				"--to-destination", fmt.Sprintf("%s:%v", ip, reservedPort.To),
			}

			if err := ipt.Append(preRouteArgs[0], preRouteArgs[1], preRouteArgs[2:]...); err != nil {
				return nil, err
			}

			c.logger.Debug("configured nat output chain for localhost", "args", preRouteArgs)
			teardownRules = append(teardownRules, preRouteArgs)
		} else {
			preRouteArgs = []string{
				iptablesNATTableName,
				preroutingIPTablesChainName,
				"-d", reservedPort.HostIP,
				"-i", iface,
				"-p", "tcp",
				"-m", "tcp",
				"--dport", strconv.Itoa(reservedPort.Value),
				"-j", "DNAT",
				"--to-destination", fmt.Sprintf("%s:%v", ip, reservedPort.To),
			}

			if err := ipt.Append(preRouteArgs[0], preRouteArgs[1], preRouteArgs[2:]...); err != nil {
				return nil, err
			}

			c.logger.Debug("configured nat prerouting chain", "args", preRouteArgs)
			teardownRules = append(teardownRules, preRouteArgs)

			// Generate our filter forward arguments to include the table and chain
			// information. This allows us to store all the detail within the
			// teardownRules easily.
			filterArgs := []string{
				iptablesFilterTableName,
				forwardIPTablesChainName,
				"-d", ip,
				"-p", "tcp",
				"-m", "state",
				"--state", "NEW",
				"-m", "tcp",
				"--dport", strconv.Itoa(reservedPort.To),
				"-j", "ACCEPT",
			}

			if err := ipt.Append(filterArgs[0], filterArgs[1], filterArgs[2:]...); err != nil {
				return nil, err
			}

			c.logger.Debug("configured filter forward chain", "args", filterArgs)
			teardownRules = append(teardownRules, filterArgs)
		}
		// The process made a change to the system, so log the critical
		// information that might be useful to operators.
		c.logger.Info("successfully configured port forwarding rules",
			"src_ip", reservedPort.HostIP, "src_port", reservedPort.Value,
			"dst_ip", ip, "dst_port", reservedPort.To, "port_label", port)
	}

	return teardownRules, nil
}

// getRoutingInterfaceByIP returns the name of the interface that can be used
// to reach the provided address.
func getRoutingInterfaceByIP(ip string) (string, error) {
	interfaces, err := stdnet.Interfaces()
	if err != nil {
		return "", err
	}

	checkAddr, err := netip.ParseAddr(ip)
	if err != nil {
		return "", err
	}

	for _, iface := range interfaces {
		if addrs, err := iface.Addrs(); err == nil {
			for _, addr := range addrs {
				if prefix, err := netip.ParsePrefix(addr.String()); err == nil {
					if prefix.Contains(checkAddr) {
						return iface.Name, nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("failed to find interface for IP %q", ip)
}

// getInterfaceByIP is a helper function which identifies which host network
// interface the passed IP address is linked to.
func getInterfaceByIP(ip stdnet.IP) (string, error) {
	interfaces, err := stdnet.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range interfaces {
		if addrs, err := iface.Addrs(); err == nil {
			for _, addr := range addrs {
				if iip, _, err := stdnet.ParseCIDR(addr.String()); err == nil {
					if iip.Equal(ip) {
						return iface.Name, nil
					}
				} else {
					continue
				}
			}
		} else {
			continue
		}
	}
	return "", fmt.Errorf("failed to find interface for IP %q", ip.String())
}

// getIPByInterface is a helper function which returns the IP address
// assigned to the interface.
func getIPByInterface(name string) (stdnet.IP, error) {
	interfaces, err := stdnet.Interfaces()
	if err != nil {
		return stdnet.IP{}, err
	}

	for _, iface := range interfaces {
		if iface.Name != name {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return stdnet.IP{}, err
		}

		for _, addr := range addrs {
			res, _, err := stdnet.ParseCIDR(addr.String())
			if err == nil {
				return res, nil
			}
		}
	}

	return stdnet.IP{}, fmt.Errorf("could not find address for interface %s", name)
}

func (c *Controller) VMTerminatedTeardown(req *net.VMTerminatedTeardownRequest) (*net.VMTerminatedTeardownResponse, error) {

	// We can't be exactly sure what the caller will give us, so make sure we
	// don't panic the driver.
	if req == nil || req.TeardownSpec == nil {
		return &net.VMTerminatedTeardownResponse{}, nil
	}

	ipt, err := c.iptablesInterfaceGetter()
	if err != nil {
		return nil, fmt.Errorf("failed to create iptables handle: %w", err)
	}

	// Collect all the errors, so we provide the operator with enough
	// information to manually tidy if needed.
	var mErr multierror.Error

	// Iterate the teardown rules and delete them from iptables. Do not halt
	// the loop if we encounter an error, track it and plough forward, so we
	// attempt to clean up as much as possible.
	//
	// Using DeleteIfExists means we do not generate error if the rule does not
	// exist. This is important for partial failure scenarios where we delete
	// one or more rules and one or more fail. The client will retry the
	// stop/kill call until all work is completed successfully. If we return an
	// error if the rule is not found, we can never recover from partial
	// failures.
	for _, iptablesRule := range req.TeardownSpec.IPTablesRules {
		if err := ipt.DeleteIfExists(iptablesRule[0], iptablesRule[1], iptablesRule[2:]...); err != nil {
			mErr.Errors = append(
				mErr.Errors,
				fmt.Errorf("failed to delete iptables %q entry in %q chain: %w",
					iptablesRule[0], iptablesRule[1], err))
		}
	}

	// Remove the DHCP IP reservation.
	if err := c.removeIPReservation(req.TeardownSpec.Network, req.TeardownSpec.DHCPReservation); err != nil {
		mErr.Errors = append(
			mErr.Errors,
			fmt.Errorf("failed to update network for IP reservation removal: %w", err))
	}

	// Release the DHCP lease. This is best effort only, so any errors encountered
	// are simply logged.
	if err := c.releaseDHCPLease(req.TeardownSpec.Network, req.TeardownSpec.DHCPReservation); err != nil {
		c.logger.Error("failed to release DHCP lease", "error", err)
	}

	return &net.VMTerminatedTeardownResponse{}, mErr.ErrorOrNil()
}

// removeIPReservation removes the DHCP IP reservation if it exists.
func (c *Controller) removeIPReservation(networkName, reservation string) error {
	if reservation == "" {
		return nil
	}

	network, err := c.netConn.LookupNetworkByName(networkName)
	if err != nil {
		return fmt.Errorf("failed to find network %q: %w", networkName, err)
	}
	defer network.Free()

	exists, err := c.ipReservationExists(network, reservation)
	if err != nil {
		return err
	}

	if !exists {
		c.logger.Debug("ip reservation not found", "reservation", reservation)
		return nil
	}

	err = network.Update(lv.NETWORK_UPDATE_COMMAND_DELETE, lv.NETWORK_SECTION_IP_DHCP_HOST,
		automaticParentIndex, reservation, lv.NETWORK_UPDATE_AFFECT_LIVE|lv.NETWORK_UPDATE_AFFECT_CONFIG)

	return err
}

// ipReservationExists checks if the DHCP reservation currently exists.
func (c *Controller) ipReservationExists(network shims.ConnectNetwork, reservation string) (bool, error) {
	res := &libvirtxml.NetworkDHCPHost{}
	if err := res.Unmarshal(reservation); err != nil {
		return false, fmt.Errorf("could not parse IP reservation: %w", err)
	}

	networkCfg := &libvirtxml.Network{}
	networkDoc, err := network.GetXMLDesc(0)
	if err = networkCfg.Unmarshal(networkDoc); err != nil {
		return false, err
	}

	for _, ip := range networkCfg.IPs {
		for _, host := range ip.DHCP.Hosts {
			if host.IP == res.IP && host.MAC == res.MAC && host.Name == res.Name {
				return true, nil
			}
		}
	}

	return false, nil
}

// releaseDHCPLease releases the DHCP lease associated to the reservation
// if it exists.
//
// Implementation is inspired from:
// https://github.com/imp/dnsmasq/blob/770bce967cfc9967273d0acfb3ea018fb7b17522/contrib/lease-tools/dhcp_release.c
func (c *Controller) releaseDHCPLease(networkName, reservation string) error {
	res := &libvirtxml.NetworkDHCPHost{}
	if err := res.Unmarshal(reservation); err != nil {
		return fmt.Errorf("failed to parse DHCP reservation: %w", err)
	}

	mac, err := stdnet.ParseMAC(res.MAC)
	if err != nil {
		return fmt.Errorf("failed to parse lease MAC address: %w", err)
	}

	network, err := c.netConn.LookupNetworkByName(networkName)
	if err != nil {
		return fmt.Errorf("failed to lookup network %q: %w", networkName, err)
	}
	defer network.Free()

	bridge, err := network.GetBridgeName()
	if err != nil {
		return fmt.Errorf("failed to get bridge name for network %q: %w", networkName, err)
	}

	server, err := c.ipByInterfaceGetter(bridge)
	if err != nil {
		return fmt.Errorf("failed to get IP address for device %q: %w", bridge, err)
	}

	// Build a DHCP release packet to trigger releasing the lease.
	pkt := layers.DHCPv4{
		Operation:    layers.DHCPOpRequest,
		HardwareType: layers.LinkTypeEthernet,
		HardwareLen:  uint8(len(mac)),
		ClientHWAddr: mac,
		ClientIP:     stdnet.ParseIP(res.IP),
		Options: []layers.DHCPOption{
			{
				Type:   layers.DHCPOptMessageType,
				Length: 1,
				Data:   []byte{byte(layers.DHCPMsgTypeRelease)},
			},
			{
				Type:   layers.DHCPOptServerID,
				Length: uint8(len(server.To4())),
				Data:   server.To4(),
			},
		},
	}
	buf := gopacket.NewSerializeBuffer()
	if err := pkt.SerializeTo(buf, gopacket.SerializeOptions{}); err != nil {
		return fmt.Errorf("failed to serialize DHCP release packet: %w", err)
	}

	// Use a custom dialer so the underlying socket can be customized
	// and bound to the bridge used for the network.
	var controlErr error
	dialer := &stdnet.Dialer{
		Control: func(network, address string, raw syscall.RawConn) error {
			return raw.Control(func(fd uintptr) {
				controlErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, bridge)
			})
		},
	}
	if controlErr != nil {
		return fmt.Errorf("failed to configure socket: %w", controlErr)
	}

	// Send the packet to the server
	conn, err := dialer.Dial("udp", stdnet.JoinHostPort(server.To4().String(), dhcpServerPort))
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("failed to write DHCP release packet: %w", err)
	}

	return nil
}

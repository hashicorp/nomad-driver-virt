// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package chnet

import (
	"errors"
	"fmt"
	stdnet "net"
	"strconv"
	"time"

	"github.com/coreos/go-iptables/iptables"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	domain "github.com/ccheshirecat/nomad-driver-ch/internal/shared"
	"github.com/ccheshirecat/nomad-driver-ch/virt/net"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

const (
	// preroutingIPTablesChainName is the IPTables chain name used by the
	// driver for prerouting rules. This is currently used for entries within
	// the nat table.
	preroutingIPTablesChainName = "NOMAD_CH_PRT"

	// forwardIPTablesChainName is the IPTables chain name used by the driver
	// for forwarding rules. This is currently used for entries within the
	// filter table.
	forwardIPTablesChainName = "NOMAD_CH_FW"

	// iptablesNATTableName is the name of the nat table within iptables.
	iptablesNATTableName = "nat"

	// iptablesFilterTableName is the name of the filter table within iptables.
	iptablesFilterTableName = "filter"

	// NetworkStateActive is string representation to declare a network is in
	// active state.
	NetworkStateActive = "active"

	// NetworkStateInactive is string representation to declare a network is in
	// inactive state.
	NetworkStateInactive = "inactive"
)

// Controller implements the Net interface for Cloud Hypervisor networking
// without depending on libvirt. It manages static IP allocation and iptables
// rules for port forwarding.
type Controller struct {
	logger        hclog.Logger
	networkConfig *domain.Network

	// interfaceByIPGetter is the function that queries the host using the
	// passed IP address and identifies the interface it is assigned to. It is
	// a field within the controller to aid testing.
	interfaceByIPGetter
}

// interfaceByIPGetter is the function signature used to identify the host's
// interface using a passed IP address. This is primarily used for testing,
// where we don't know the host, and we want to ensure stability and
// consistency when this is called.
type interfaceByIPGetter func(ip stdnet.IP) (string, error)

// NewController returns a Controller which implements the net.Net interface
// for Cloud Hypervisor networking.
func NewController(logger hclog.Logger, networkConfig *domain.Network) *Controller {
	return &Controller{
		logger:              logger.Named("chnet"),
		networkConfig:       networkConfig,
		interfaceByIPGetter: getInterfaceByIP,
	}
}

// Fingerprint interrogates the host system and populates the attribute
// mapping with relevant network information. For CH, we check the configured
// bridge interface status.
func (c *Controller) Fingerprint(attr map[string]*structs.Attribute) {
	bridgeName := c.networkConfig.Bridge

	// Check if bridge exists and is active
	state := c.getBridgeState(bridgeName)

	// Populate the attributes mapping with our bridge state
	bridgeStateKey := net.FingerprintAttributeKeyPrefix + bridgeName + ".state"
	attr[bridgeStateKey] = structs.NewStringAttribute(state)

	// Add bridge name attribute
	bridgeNameKey := net.FingerprintAttributeKeyPrefix + bridgeName + ".bridge_name"
	attr[bridgeNameKey] = structs.NewStringAttribute(bridgeName)

	c.logger.Debug("network fingerprint complete", "bridge", bridgeName, "state", state)
}

// getBridgeState checks if the bridge interface exists and is up
func (c *Controller) getBridgeState(bridgeName string) string {
	interfaces, err := stdnet.Interfaces()
	if err != nil {
		c.logger.Error("failed to list network interfaces", "error", err)
		return NetworkStateInactive
	}

	for _, iface := range interfaces {
		if iface.Name == bridgeName {
			if iface.Flags&stdnet.FlagUp != 0 {
				return NetworkStateActive
			}
			return NetworkStateInactive
		}
	}

	c.logger.Warn("bridge not found", "bridge", bridgeName)
	return NetworkStateInactive
}

// Init performs any initialization work needed by the network sub-system
// prior to being used by the driver. This sets up the required iptables
// chains for port forwarding.
func (c *Controller) Init() error {
	return c.ensureIPTables()
}

// ensureIPTables is responsible for ensuring the local host machine iptables
// are configured with the chains and rules needed by the driver.
//
// On a new machine, this function creates the "NOMAD_CH_PRT" and "NOMAD_CH_FW"
// chains. The "NOMAD_CH_PRT" chain then has a jump rule added to the "nat"
// table; the "NOMAD_CH_FW" chain has a jump rule added to the "filter" table.
func (c *Controller) ensureIPTables() error {
	ipt, err := iptables.New()
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

// ensureIPTablesChain creates an iptables chain if it doesn't exist
func ensureIPTablesChain(ipt *iptables.IPTables, table, chain string) (bool, error) {
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

// VMStartedBuild performs network configuration once a VM has been started.
// For Cloud Hypervisor, this means setting up port forwarding rules since
// we use static IP allocation rather than DHCP discovery.
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
	netConfig := *req.NetConfig

	// Protect against VMs with no network interface. The log is useful for
	// debugging which certainly caught me a few times in development.
	if len(netConfig) == 0 {
		c.logger.Debug("no network interface configured", "domain", req.DomainName)
		return &net.VMStartedBuildResponse{}, nil
	}
	netInterface := netConfig[0]

	// For Cloud Hypervisor, we get the IP from the VM process directly
	// rather than discovering it via DHCP. The CH driver should provide
	// this in the Hwaddrs field or we extract it differently.

	// For now, we'll extract IP from the domain name or use a static approach
	// TODO: This should be provided by the CH driver through a better mechanism
	ipAddr, err := c.getVMStaticIP(req.DomainName, req.Hwaddrs)
	if err != nil {
		return nil, fmt.Errorf("failed to get VM IP: %w", err)
	}

	// Configure iptables rules for port forwarding
	teardownRules, err := c.configureIPTables(req.Resources, netInterface.Bridge, ipAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to configure port mapping: %w", err)
	}

	return &net.VMStartedBuildResponse{
		DriverNetwork: &drivers.DriverNetwork{
			IP: ipAddr,
		},
		TeardownSpec: &net.TeardownSpec{
			IPTablesRules: teardownRules,
			// No DHCP reservation for CH since we use static IPs
			DHCPReservation: "",
			Network:         c.networkConfig.Bridge,
		},
	}, nil
}

// getVMStaticIP extracts or determines the static IP for a VM
// For now, this is a placeholder - the CH driver should provide this info
func (c *Controller) getVMStaticIP(domainName string, hwaddrs []string) (string, error) {
	// TODO: This should be coordinated with the CH driver's IP allocation
	// For now, return a placeholder approach or extract from a known mechanism

	// Simple approach: extract from domain name if it contains IP info
	// or use a deterministic allocation based on domain name hash
	// This is temporary until proper integration with CH driver

	if len(hwaddrs) > 0 {
		// If hwaddrs contains IP info, extract it
		// This is a hack until proper integration
		for _, addr := range hwaddrs {
			if stdnet.ParseIP(addr) != nil {
				return addr, nil
			}
		}
	}

	// Fallback: deterministic IP based on domain name hash
	hash := 0
	for _, c := range domainName {
		hash = hash*31 + int(c)
	}

	// Generate IP in the configured pool range
	ipOffset := (hash % 100) + 100 // 100-199 range
	ip := fmt.Sprintf("194.31.143.%d", ipOffset)

	return ip, nil
}

// configureIPTables is responsible for adding the iptables entries to enable
// port mapping. The function will perform this action for all configured ports
// within the network interface configuration.
//
// The returned array contains the added rules which helps make it easier to
// delete rules when a task is stopped, specifically by avoiding having to
// generate the information again.
func (c *Controller) configureIPTables(
	res *drivers.Resources, cfg *net.NetworkInterfaceBridgeConfig, ip string) ([][]string, error) {

	var teardownRules [][]string

	ipt, err := iptables.New()
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
		preRouteArgs := []string{
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

		// The process made a change to the system, so log the critical
		// information that might be useful to operators.
		c.logger.Info("successfully configured port forwarding rules",
			"src_ip", reservedPort.HostIP, "src_port", reservedPort.Value,
			"dst_ip", ip, "dst_port", reservedPort.To, "port_label", port)
	}

	return teardownRules, nil
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

// VMTerminatedTeardown performs all the network teardown required to clean
// the host and any systems of configuration specific to the task.
func (c *Controller) VMTerminatedTeardown(req *net.VMTerminatedTeardownRequest) (*net.VMTerminatedTeardownResponse, error) {
	// We can't be exactly sure what the caller will give us, so make sure we
	// don't panic the driver.
	if req == nil || req.TeardownSpec == nil {
		return &net.VMTerminatedTeardownResponse{}, nil
	}

	ipt, err := iptables.New()
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

	// No DHCP reservation cleanup needed for Cloud Hypervisor since we use static IPs

	return &net.VMTerminatedTeardownResponse{}, mErr.ErrorOrNil()
}
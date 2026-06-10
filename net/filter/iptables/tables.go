// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	virtnet "github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/drivers"
)

var errLoopbackNotEnabled = errors.New("loopback port forwarding not enabled")

// virtTables implements the filter.Filter interface.
type virtTables struct {
	logger hclog.Logger
	ipt    IPTables
	names  *names
	m      sync.Mutex

	// Everything below is used for testing.

	// routeLocalnetTemplate is a template for creating the path to the kernel
	// runtime configuration for device localnet routing.
	routeLocalnetPathTemplate string

	// interfaceByIPGetter is the function that queries the host using the
	// passed IP address and identifies the interface it is assigned to. It is
	// a field within the controller to aid testing.
	interfaceByIPGetter

	// routingIngerfaceByIPGetter is the function that queries the host using
	// the passed IP address and identifies the interface used to reach it.
	routingInterfaceByIPGetter
}

// SetLogger sets the logger used.
func (n *virtTables) SetLogger(logger hclog.Logger) {
	n.logger = logger
}

// Configure configures iptables to enable port forwards based on the passed
// resources and returns a collection of rules that can be used to remove
// the configuration with Teardown function.
func (n *virtTables) Configure(res *drivers.Resources, cfg *virtnet.NetworkInterfaceBridgeConfig, ip string) (rules *virtnet.FilterRemoval, err error) {
	// Check that received values are suitable for configuration.
	if res == nil {
		return nil, errors.New("cannot configure iptables, resources not provided")
	}

	if cfg == nil {
		return nil, errors.New("cannot configure iptables, bridge config not provided")
	}

	// If the ports are nil, there's nothing to do.
	if res.Ports == nil {
		return &virtnet.FilterRemoval{Name: removalName}, nil
	}

	// Create lookup mapping for ip:interface-name, so we can cache reads of
	// this and not have to perform the translation each time.
	interfaceMapping := make(map[string]string)

	// Create a new request to build up the desired changes.
	req := newRequest()

	// Iterate the ports configured within the network interface and pull these
	// from the task allocated ports.
	for _, port := range cfg.Ports {
		reservedPort, ok := res.Ports.Get(port)
		if !ok {
			n.logger.Error("failed to find reserved port", "port", port)
			continue
		}

		// Look into the mapping for the interface based on the host IP,
		// otherwise perform the more expensive actual lookup by querying the
		// host.
		iface, ok := interfaceMapping[reservedPort.HostIP]
		if !ok {
			iface, err = n.interfaceByIPGetter(net.ParseIP(reservedPort.HostIP))
			if err != nil {
				return nil, fmt.Errorf("failed to identify IP interface: %w", err)
			}

			interfaceMapping[reservedPort.HostIP] = iface
		}

		// Parse the host IP so we can determine if it is a loopback address.
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
				dstIface, err = n.routingInterfaceByIPGetter(ip)
				if err != nil {
					return nil, fmt.Errorf("failed to identify IP interface: %w", err)
				}

				interfaceMapping[ip] = dstIface
			}

			// Check if loopback port forwarding is even available before starting.
			if !n.loopbackPortForwardsSupported(dstIface) {
				n.logger.Error(fmt.Sprintf("loopback port forwarding requires kernel runtime configuration - net.ipv4.conf.%s.route_localnet=1", dstIface))
				return nil, fmt.Errorf("%w for device - %s", errLoopbackNotEnabled, dstIface)
			}

			// Add chains required for loopback port forwarding.
			req.chains.InsertSlice([]*chain{
				{
					table: n.names.tables.NAT,
					chain: n.names.chains.Nomad.Output,
				},
				{
					table: n.names.tables.NAT,
					chain: n.names.chains.Nomad.Postrouting,
				},
			})

			// Add the rules.
			req.rules.InsertSlice([]*rule{
				// Jump rules for the new chains.
				{
					table:    n.names.tables.NAT,
					chain:    n.names.chains.Output,
					position: 1,
					spec:     []string{"-j", n.names.chains.Nomad.Output},
				},
				{
					table:    n.names.tables.NAT,
					chain:    n.names.chains.Postrouting,
					position: 1,
					spec:     []string{"-j", n.names.chains.Nomad.Postrouting},
				},
				// Allow forwarding from the loopback to the destination device.
				{
					table: n.names.tables.NAT,
					chain: n.names.chains.Nomad.Postrouting,
					spec: []string{"-o", dstIface, "-m", "addrtype", "--src-type", "LOCAL",
						"--dst-type", "UNICAST", "-j", "MASQUERADE"},
				},
				// Enable the actual forward.
				{
					table:     n.names.tables.NAT,
					chain:     n.names.chains.Nomad.Output,
					removable: true,
					spec: []string{"-s", reservedPort.HostIP, "-o", iface, "-p", "tcp", "-m", "tcp",
						"--dport", strconv.Itoa(reservedPort.Value), "-j", "DNAT", "--to-destination",
						fmt.Sprintf("%s:%d", ip, reservedPort.To)},
				},
			})
		} else {
			// Add prerouting and filtering rule to enable the forward.
			req.rules.InsertSlice([]*rule{
				{
					table:     n.names.tables.NAT,
					chain:     n.names.chains.Nomad.Prerouting,
					removable: true,
					spec: []string{"-d", reservedPort.HostIP, "-i", iface, "-p", "tcp", "-m", "tcp",
						"--dport", strconv.Itoa(reservedPort.Value), "-j", "DNAT", "--to-destination",
						fmt.Sprintf("%s:%d", ip, reservedPort.To)},
				},
				{
					table:     n.names.tables.Filter,
					chain:     n.names.chains.Nomad.Forward,
					removable: true,
					spec: []string{"-d", ip, "-p", "tcp", "-m", "state", "--state", "NEW", "-m", "tcp",
						"--dport", strconv.Itoa(reservedPort.To), "-j", "ACCEPT"},
				},
			})
		}
	}

	if err := n.add(req); err != nil {
		return nil, err
	}

	return &virtnet.FilterRemoval{
		Name: removalName,
		Data: req.removalInstructions(),
	}, nil
}

// Teardown removes rules from iptables.
func (n *virtTables) Teardown(removal *virtnet.FilterRemoval) error {
	// If there is no removal information then there
	// is nothing to do.
	if removal == nil || removal.Data == nil {
		return nil
	}

	rules, ok := removal.Data.(Rules)
	if !ok {
		n.logger.Error("invalid teardown data received", "name", removal.Name,
			"type", hclog.Fmt("%T", removal.Data))
		return fmt.Errorf("invalid teardown data, cannot remove iptables rules")
	}
	req := newRequest()
	req.rules.InsertSet(rules.rules())

	return n.remove(req)
}

// setup is responsible for ensuring the local host machine iptables
// are configured with the chains and rules needed by the driver.
//
// On a new machine, this function creates the "NOMAD_VT_PRT" and "NOMAD_VT_FW"
// chains. The "NOMAD_VT_PRT" chain then has a jump rule added to the "nat"
// table; the "NOMAD_VT_FW" chain has a jump rule added to the "filter" table.
func (n *virtTables) setup() error {
	req := newRequest()

	// Add the custom NAT and filter chains.
	req.chains.InsertSlice([]*chain{
		{
			table: n.names.tables.NAT,
			chain: n.names.chains.Nomad.Prerouting,
		},
		{
			table: n.names.tables.Filter,
			chain: n.names.chains.Nomad.Forward,
		},
	})
	// Add the jump rules to the custom chains.
	req.rules.InsertSlice([]*rule{
		{
			table:    n.names.tables.NAT,
			chain:    n.names.chains.Prerouting,
			position: 1,
			spec:     []string{"-j", n.names.chains.Nomad.Prerouting},
		},
		{
			table:    n.names.tables.Filter,
			chain:    n.names.chains.Forward,
			position: 1,
			spec:     []string{"-j", n.names.chains.Nomad.Forward},
		},
	})

	// Apply any updates that are required.
	if err := n.add(req); err != nil {
		return fmt.Errorf("setup failure: %w", err)
	}

	return nil
}

// add adds chains and rules to iptables.
func (n *virtTables) add(req *request) error {
	n.m.Lock()
	defer n.m.Unlock()

	// Start with creating any needed chains.
	for _, c := range req.chains.Slice() {
		exists, err := n.ipt.ChainExists(c.table, c.chain)
		if err != nil {
			return fmt.Errorf("failed to check chain existence: %w", err)
		}

		if !exists {
			if err := n.ipt.NewChain(c.table, c.chain); err != nil {
				return fmt.Errorf("failed to create new chain: %w", err)
			}
		}
	}

	// Now add any rules that remain. If a position is defined, then the rule
	// is inserted, otherwise it is appended.
	for _, r := range req.rules.Slice() {
		var err error
		if r.position > 0 {
			err = n.ipt.InsertUnique(r.table, r.chain, r.position, r.spec...)
		} else {
			err = n.ipt.AppendUnique(r.table, r.chain, r.spec...)
		}

		if err != nil {
			return fmt.Errorf("failed to add rule: %w", err)
		}
	}

	return nil
}

// remove removes chains and rules from iptables.
// NOTE: Removal errors are _not_ immediately fatal allowing as much to
// be removed as possible. The errors will be collected and returned as
// a multierror.
func (n *virtTables) remove(req *request) error {
	n.m.Lock()
	defer n.m.Unlock()

	// ClearAndDeleteChain below will delete all the rules on the
	// chain, so those rules don't need to be deleted individually.
	req.rules.RemoveFunc(func(r *rule) bool {
		return req.chains.Contains(r.mkchain())
	})

	var mErr *multierror.Error

	// Remove rules in the request.
	for _, r := range req.rules.Slice() {
		if err := n.ipt.DeleteIfExists(r.table, r.chain, r.spec...); err != nil {
			// NOTE: attempting to delete jump rules that don't exist will
			// cause a "does not exist" error. Check error and ignore.
			if !isNotExistErr(err) {
				n.logger.Error("failed to delete iptables rule", "error", err, "rule", *r)
				mErr = multierror.Append(mErr, err)
			}
		}
	}

	// Remove chains in the request.
	for _, c := range req.chains.Slice() {
		// Clear and delete the chain. This function will check that
		// the chain exists so we don't need to do that here.
		if err := n.ipt.ClearAndDeleteChain(c.table, c.chain); err != nil {
			n.logger.Error("failed to delete iptables chain", "error", err, "chain", *c)
			mErr = multierror.Append(mErr, err)
		}
	}

	return mErr.ErrorOrNil()
}

// loopbackPortForwardsSupported returns if the host has been configured for routing localnet packets.
// NOTE: The global configuration overrides device specific configuration.
func (n *virtTables) loopbackPortForwardsSupported(device string) bool {
	for _, configName := range []string{routeLocalnetGlobalName, device} {
		tmpl := n.routeLocalnetPathTemplate
		if tmpl == "" {
			tmpl = routeLocalnetPathTemplate
		}

		cfgPath := fmt.Sprintf(tmpl, configName)
		content, err := os.ReadFile(cfgPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			n.logger.Error("read failed for device loopback support check", "path", cfgPath, "error", err)
			return false
		}

		if strings.TrimSpace(string(content)) == "1" {
			return true
		}
	}

	return false
}

// getInterfaceByIP is a helper function which identifies which host network
// interface the passed IP address is linked to.
func getInterfaceByIP(ip net.IP) (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}

	for _, iface := range interfaces {
		if addrs, err := iface.Addrs(); err == nil {
			for _, addr := range addrs {
				if iip, _, err := net.ParseCIDR(addr.String()); err == nil {
					if iip.Equal(ip) {
						return iface.Name, nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("failed to find interface for IP %q", ip.String())
}

// getRoutingInterfaceByIP returns the name of the interface that can be used
// to reach the provided address.
func getRoutingInterfaceByIP(ip string) (string, error) {
	interfaces, err := net.Interfaces()
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

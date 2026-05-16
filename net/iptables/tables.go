// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/go-set/v3"
	virtnet "github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/drivers"
)

// virtTables implements the NomadTables interface.
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

// Configure configures iptables to enable port forwards based on the passed
// resources and returns a collection of rules that can be used to remove
// the configuration with Teardown function.
func (n *virtTables) Configure(res *drivers.Resources, cfg *virtnet.NetworkInterfaceBridgeConfig, ip string) (rules Rules, err error) {
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
				return nil, fmt.Errorf("loopback port forwarding not enabled for device - %s", dstIface)
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
					table:    n.names.tables.NAT,
					chain:    n.names.chains.Nomad.Output,
					teardown: true,
					spec: []string{"-s", reservedPort.HostIP, "-o", iface, "-p", "tcp", "-m", "tcp",
						"--dport", strconv.Itoa(reservedPort.Value), "-j", "DNAT", "--to-destination",
						fmt.Sprintf("%s:%v", ip, reservedPort.To)},
				},
			})
		} else {
			// Add prerouting and filtering rule to enable the forward.
			req.rules.InsertSlice([]*rule{
				{
					table:    n.names.tables.NAT,
					chain:    n.names.chains.Nomad.Prerouting,
					teardown: true,
					spec: []string{"-d", reservedPort.HostIP, "-i", iface, "-p", "tcp", "-m", "tcp",
						"--dport", strconv.Itoa(reservedPort.Value), "-j", "DNAT", "--to-destination",
						fmt.Sprintf("%s:%v", ip, reservedPort.To)},
				},
				{
					table:    n.names.tables.Filter,
					chain:    n.names.chains.Nomad.Forward,
					teardown: true,
					spec: []string{"-d", ip, "-p", "tcp", "-m", "state", "--state", "NEW", "-m", "tcp",
						"--dport", strconv.Itoa(reservedPort.To), "-j", "ACCEPT"},
				},
			})
		}
	}

	if err := n.add(req); err != nil {
		return nil, err
	}

	return req.teardown(), nil
}

// Teardown removes rules from iptables.
func (n *virtTables) Teardown(rules Rules) error {
	req := newRequest()
	req.addRules(rules.rules().Slice())

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
	req.addChains([]*chain{
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
	req.addRules([]*rule{
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

// add adds chains and rules to iptables. The request will be inspected
// and compared to existing chains and rules defined in iptables and only
// add what does not already exist.
func (n *virtTables) add(req *request) error {
	n.m.Lock()
	defer n.m.Unlock()

	// First collect the existing lists of chains and rules for
	// the entries in the request.
	chainLists, ruleLists, err := n.buildLists(req)
	if err != nil {
		return err
	}

	// Prune any chains that already exist.
	req.chains.RemoveFunc(func(c *chain) bool {
		return chainLists.Contains(c)
	})

	// Prune any rules that already exist.
	req.rules.RemoveFunc(func(r *rule) bool {
		return ruleLists.Contains(r)
	})

	// Start with creating any needed chains.
	for _, c := range req.sortedChains() {
		if err := n.ipt.NewChain(c.table, c.chain); err != nil {
			return fmt.Errorf("failed to create new chain: %w", err)
		}
	}

	// Now add any rules that remain. If a position is defined, then the rule
	// is inserted, otherwise it is appended.
	for _, r := range req.sortedRules() {
		var err error
		if r.position > 0 {
			err = n.ipt.Insert(r.table, r.chain, r.position, r.spec...)
		} else {
			err = n.ipt.Append(r.table, r.chain, r.spec...)
		}

		if err != nil {
			return fmt.Errorf("failed to add rule: %w", err)
		}
	}

	return nil
}

// remove removes chains and rules from iptables. The request will be
// inspected and compared to existing chains and rules defined in iptables
// and only remove what does not already exist.
// NOTE: Removal errors are _not_ immediately fatal allowing as much to
// be removed as possible. The errors will be collected and returned as
// a multierror.
func (n *virtTables) remove(req *request) error {
	n.m.Lock()
	defer n.m.Unlock()

	chainLists, ruleLists, err := n.buildLists(req)
	if err != nil {
		return err
	}

	// Prune any chains that do not exist.
	req.chains.RemoveFunc(func(c *chain) bool {
		return !chainLists.Contains(c)
	})

	// Prune any rules that do not exist.
	req.rules.RemoveFunc(func(r *rule) bool {
		return !ruleLists.Contains(r)
	})

	var mErr *multierror.Error

	// Start with removing rules.
	for _, r := range req.sortedRules() {
		if err := n.ipt.Delete(r.table, r.chain, r.spec...); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	// Now remove any chains.
	for _, c := range req.sortedChains() {
		// First clear the chain.
		if err := n.ipt.ClearChain(c.table, c.chain); err != nil {
			mErr = multierror.Append(mErr, err)
			continue
		}

		// Now delete the chain.
		if err := n.ipt.DeleteChain(c.table, c.chain); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return mErr.ErrorOrNil()
}

// buildLists gathers lists of existing chains on tables and existing rules on chains that
// are relevant to the request.
func (n *virtTables) buildLists(req *request) (chainLists set.Collection[*chain], ruleLists set.Collection[*rule], err error) {
	chainLists = set.NewHashSet[*chain](0)
	for _, table := range req.tableList() {
		list, err := n.ipt.ListChains(table)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list chains in table %q: %w", table, err)
		}
		for _, item := range list {
			chainLists.Insert(&chain{table: table, chain: item})
		}
	}

	ruleLists = set.NewHashSet[*rule](0)
	for _, c := range req.ruleChains() {
		// If the chain doesn't exist, don't attempt to list
		// it as it will just generate an error.
		if !chainLists.Contains(c) {
			continue
		}

		list, err := n.ipt.List(c.table, c.chain)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list rules in chain %q on table %q: %w", c.table, c.chain, err)
		}
		for _, item := range list {
			parts := strings.Split(item, " ")
			r := &rule{
				table: c.table,
				chain: c.chain,
				spec:  parts[slices.Index(parts, c.chain)+1:],
			}
			ruleLists.Insert(r)
		}
	}

	return chainLists, ruleLists, nil
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

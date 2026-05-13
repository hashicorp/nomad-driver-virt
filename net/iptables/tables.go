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

// Rules is a raw collection of rules.
type Rules [][]string

// rules converts the raw collection into a slice of rules.
func (r Rules) rules() []*rule {
	result := make([]*rule, len(r))
	for i, entry := range r {
		if len(entry) < 3 {
			// invalid rule so skip.
			continue
		}
		result[i] = &rule{
			table: entry[0],
			chain: entry[1],
			spec:  entry[2:],
		}
	}

	return result
}

// newRequest returns a new request instance.
func newRequest() *request {
	return &request{
		chains: new(set.HashSet[*chain, string]),
		rules:  new(set.HashSet[*rule, string]),
	}
}

// request is used to provide chains and rules that need to
// be created or removed.
type request struct {
	chains *set.HashSet[*chain, string]
	rules  *set.HashSet[*rule, string]
}

// chainTables returns the collection of table names defined
// in the request's chain collection.
func (r *request) chainTables() set.Collection[string] {
	s := set.New[string](0)
	for _, c := range r.chains.Slice() {
		s.Insert(c.table)
	}

	return s
}

// ruleChains returns the collection chains defined in the
// request's rule collection.
func (r *request) ruleChains() set.Collection[*chain] {
	s := new(set.HashSet[*chain, string])
	for _, rule := range r.rules.Slice() {
		s.Insert(&chain{table: rule.table, chain: rule.chain})
	}

	return s
}

// teardown returns the raw collection of rules from the
// request that have been flagged as teardown.
func (r *request) teardown() Rules {
	result := make([][]string, 0)
	for _, r := range r.rules.Slice() {
		if !r.teardown {
			continue
		}
		result = append(result, r.slice())
	}

	return result
}

// chain represents an iptables chain.
type chain struct {
	table string // table name
	chain string // chain name
}

// Hash returns a unique string for the chain.
func (c *chain) Hash() string {
	return c.table + c.chain
}

// rule represents an iptables rule.
type rule struct {
	table    string   // table name
	chain    string   // chain name
	position int      // position of the rule if it should be inserted
	spec     []string // rule specification
	teardown bool     // rule should be included in teardown list
}

// Hash returns a unique string for the rule.
func (r *rule) Hash() string {
	return fmt.Sprintf("%s%s%d%s", r.table, r.chain, r.position, strings.Join(r.spec, ""))
}

func (r *rule) String() string {
	return fmt.Sprintf("%s %s %s", r.table, r.chain, strings.Join(r.spec, " "))
}

// slice converts the rule into a string slice.
func (r *rule) slice() []string {
	return append([]string{r.table, r.chain}, r.spec...)
}

// nomadTables implements the NomadTables interface.
type nomadTables struct {
	logger hclog.Logger
	m      sync.Mutex
	ipt    IPTables

	// Below is used for testing.

	// routeLocalnetTemplate is a template for creating the path to the kernel
	// runtime configuration for device localnet routing.
	routeLocalnetTemplate string

	// interfaceByIPGetter is the function that queries the host using the
	// passed IP address and identifies the interface it is assigned to. It is
	// a field within the controller to aid testing.
	interfaceByIPGetter

	// iptablesInterfaceGetter is the function that returns an interface
	// for IPTables.
	iptablesInterfaceGetter

	// routingIngerfaceByIPGetter is the function that queries the host using
	// the passed IP address and identifies the interface used to reach it.
	routingInterfaceByIPGetter
}

func (n *nomadTables) Configure(res *drivers.Resources, cfg *virtnet.NetworkInterfaceBridgeConfig, ip string) (rules Rules, err error) {
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

			// Add chains and rules required for loopback port forwarding.
			req.chains.Insert(&chain{table: iptablesNATTableName, chain: outputIPTablesChainName})
			req.chains.Insert(&chain{table: iptablesNATTableName, chain: postroutingIPTablesChainName})
			req.rules.Insert(&rule{table: iptablesNATTableName, chain: "OUTPUT",
				position: 1, spec: []string{"-j", outputIPTablesChainName}})
			req.rules.Insert(&rule{table: iptablesNATTableName, chain: "POSTROUTING",
				position: 1, spec: []string{"-j", postroutingIPTablesChainName}})

			// Add rule to enable forwarding from the loopback to the destination.
			req.rules.Insert(&rule{table: iptablesNATTableName, chain: postroutingIPTablesChainName,
				spec: []string{"-o", dstIface, "-m", "addrtype", "--src-type", "LOCAL", "--dst-type", "UNICAST", "-j", "MASQUERADE"}})

			// Finally add the forward rule.
			req.rules.Insert(&rule{table: iptablesNATTableName, chain: outputIPTablesChainName, teardown: true,
				spec: []string{"-s", reservedPort.HostIP, "-o", iface, "-p", "tcp", "-m", "tcp", "--dport", strconv.Itoa(reservedPort.Value),
					"-j", "DNAT", "--to-destination", fmt.Sprintf("%s:%v", ip, reservedPort.To)}})
		} else {
			// Add the prerouting rule.
			req.rules.Insert(&rule{table: iptablesNATTableName, chain: preroutingIPTablesChainName, teardown: true,
				spec: []string{"-d", reservedPort.HostIP, "-i", iface, "-p", "tcp", "-m", "tcp", "--dport", strconv.Itoa(reservedPort.Value),
					"-j", "DNAT", "--to-destination", fmt.Sprintf("%s:%v", ip, reservedPort.To)}})

			// Add the filtering rule.
			req.rules.Insert(&rule{table: iptablesFilterTableName, chain: forwardIPTablesChainName, teardown: true,
				spec: []string{"-d", ip, "-p", "tcp", "-m", "state", "--state", "NEW", "-m", "tcp",
					"--dport", strconv.Itoa(reservedPort.To), "-j", "ACCEPT"}})
		}
	}

	if err := n.apply(req); err != nil {
		return nil, err
	}

	return req.teardown(), nil
}

// Teardown removes rules from iptables.
func (n *nomadTables) Teardown(rules Rules) error {
	n.m.Lock()
	defer n.m.Unlock()

	var mErr *multierror.Error

	for _, r := range rules.rules() {
		if err := n.ipt.DeleteIfExists(r.table, r.chain, r.spec...); err != nil {
			mErr = multierror.Append(mErr,
				fmt.Errorf("failed to delete %q entry in %q chain: %w", r.table, r.chain, err))
		}
	}

	return mErr.ErrorOrNil()
}

// setup is responsible for ensuring the local host machine iptables
// are configured with the chains and rules needed by the driver.
//
// On a new machine, this function creates the "NOMAD_VT_PRT" and "NOMAD_VT_FW"
// chains. The "NOMAD_VT_PRT" chain then has a jump rule added to the "nat"
// table; the "NOMAD_VT_FW" chain has a jump rule added to the "filter" table.
func (n *nomadTables) setup() error {
	req := newRequest()

	// Add the NAT and filter forward chains and rules if needed.
	req.chains.Insert(&chain{table: iptablesNATTableName, chain: preroutingIPTablesChainName})
	req.chains.Insert(&chain{table: iptablesFilterTableName, chain: forwardIPTablesChainName})
	req.rules.Insert(&rule{table: iptablesNATTableName, chain: "PREROUTING",
		position: 1, spec: []string{"-j", preroutingIPTablesChainName}})
	req.rules.Insert(&rule{table: iptablesFilterTableName, chain: "FORWARD",
		position: 1, spec: []string{"-j", forwardIPTablesChainName}})

	// Apply any updates that are required.
	if err := n.apply(req); err != nil {
		return fmt.Errorf("setup failure: %w", err)
	}

	return nil
}

// buildLists gathers lists of existing chains on tables and existing rules on chains that
// are relevant to the request.
func (n *nomadTables) buildLists(req *request) (chainLists map[string][]string, ruleLists map[string][]string, err error) {
	chainLists = make(map[string][]string)
	for _, table := range req.chainTables().Slice() {
		list, err := n.ipt.ListChains(table)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list chains in table %q: %w", table, err)
		}
		chainLists[table] = list
	}

	ruleLists = make(map[string][]string)
	for _, c := range req.ruleChains().Slice() {
		list, err := n.ipt.List(c.table, c.chain)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to list rules in chain %q on table %q: %w", c.table, c.chain, err)
		}
		ruleLists[c.Hash()] = list
	}

	return chainLists, ruleLists, nil
}

// apply inspects the request and removes any chain or rule entries from the request
// that already exist and then creates the remaining chains and rules.
func (n *nomadTables) apply(req *request) error {
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
		return slices.Contains(chainLists[c.table], c.chain)
	})

	// Now create the chains.
	for _, c := range req.chains.Slice() {
		if err := n.ipt.NewChain(c.table, c.chain); err != nil {
			return fmt.Errorf("failed to create new chain: %w", err)
		}
	}

	// Prune any rules that already exist.
	// TODO: This needs to be better than big loop like this.
	req.rules.RemoveFunc(func(r *rule) bool {
		for _, existing := range ruleLists[r.Hash()] {
			if strings.HasPrefix(existing, strings.Join(r.spec, " ")) {
				return true
			}
		}
		return false
	})

	// Now add any rules that remain. If a position is defined, then the rule
	// is inserted, otherwise it is appended.
	for _, r := range req.rules.Slice() {
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

// loopbackPortForwardsSupported returns if the host has been configured for routing localnet packets.
// NOTE: The global configuration overrides device specific configuration.
func (n *nomadTables) loopbackPortForwardsSupported(device string) bool {
	for _, configName := range []string{routeLocalnetGlobalName, device} {
		tmpl := n.routeLocalnetTemplate
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

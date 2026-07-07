// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package linux

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	"github.com/hashicorp/nomad-driver-virt/net/filter/linux/shared"
	virtnet "github.com/hashicorp/nomad-driver-virt/virt/net"
)

// interfaceByIPGetter is the function signature used to identify the host's
// interface using a passed IP address. This is primarily used for testing,
// where we don't know the host, and we want to ensure stability and
// consistency when this is called.
type interfaceByIPGetter func(ip net.IP) (string, error)

// routingInterfaceByIPGetter is the function signature used to identify
// the host interface used for an IP address.
type routingInterfaceByIPGetter func(ip string) (string, error)

// tables implements the filter.Filter interface.
type tables struct {
	logger  hclog.Logger
	backend Backend
	names   *names
	m       sync.Mutex

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
func (t *tables) SetLogger(logger hclog.Logger) {
	t.m.Lock()
	defer t.m.Unlock()

	t.logger = logger
}

// Configure configures the packet filter to enable port forwards based on
// the passed port mappings and network configuration. The identity value is
// used to mark generated rules for later removal. The result of this function
// can be used to remove the generated rules using the Teardown function.
func (t *tables) Configure(mappings virtnet.PortMappings, cfg *virtnet.NetworkInterfaceBridgeConfig, ip, identity string) (rules *virtnet.FilterRemoval, err error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w - missing bridge config", errs.ErrPacketFilterConfiguration)
	}

	if len(cfg.Ports) > 0 && len(mappings) == 0 {
		return nil, fmt.Errorf("%w - missing port mappings", errs.ErrPacketFilterConfiguration)
	}

	// Create lookup mapping for ip:interface-name, so we can cache reads of
	// this and not have to perform the translation each time.
	interfaceMapping := make(map[string]string)

	// Create a new request to build up the desired changes.
	req := shared.NewRequest(identity)

	// Iterate the ports configured within the network interface and pull these
	// from the task allocated ports.
	for _, port := range cfg.Ports {
		reservedPort, ok := mappings.Get(port)
		if !ok {
			t.logger.Error("failed to find reserved port", "port", port)
			return nil, fmt.Errorf("%w - failed to find reserved port", errs.ErrPacketFilterConfigure)
		}

		// Look into the mapping for the interface based on the host IP,
		// otherwise perform the more expensive actual lookup by querying the
		// host.
		iface, ok := interfaceMapping[reservedPort.HostIP]
		if !ok {
			iface, err = t.interfaceByIPGetter(net.ParseIP(reservedPort.HostIP))
			if err != nil {
				t.logger.Error("no interface found for address", "address", reservedPort.HostIP)
				return nil, fmt.Errorf("%w - failed to identify IP interface: %w",
					errs.ErrPacketFilterConfigure, err)
			}

			interfaceMapping[reservedPort.HostIP] = iface
		}

		// Parse the host IP so we can determine if it is a loopback address.
		hostIP, err := netip.ParseAddr(reservedPort.HostIP)
		if err != nil {
			t.logger.Error("could not parse address", "address", reservedPort.HostIP)
			return nil, fmt.Errorf("%w - failed to parse host IP address: %w",
				errs.ErrPacketFilterConfigure, err)
		}

		// If the host IP provided is a loopback, it needs to be picked up on the
		// output chain and redirected to the VM. This is a special case and which
		// requires the host to be properly configured.
		if hostIP.IsLoopback() {
			// Find the interface that the destination address is attached.
			dstIface, ok := interfaceMapping[ip]
			if !ok {
				dstIface, err = t.routingInterfaceByIPGetter(ip)
				if err != nil {
					t.logger.Error("no interface found for destination", "address", ip)
					return nil, fmt.Errorf("%w - failed to identify IP interface: %w",
						errs.ErrPacketFilterConfigure, err)
				}

				interfaceMapping[ip] = dstIface
			}

			// Check if loopback port forwarding is even available before starting.
			if !t.loopbackPortForwardsSupported(dstIface) {
				t.logger.Error(fmt.Sprintf("loopback port forwarding requires kernel runtime configuration - net.ipv4.conf.%s.route_localnet=1", dstIface))
				return nil, fmt.Errorf("%w for device - %s", errs.ErrLoopbackNotEnabled, dstIface)
			}

			// Add chains required for loopback port forwarding.
			req.Add(
				shared.Chain{
					Table: t.names.tables.NAT,
					Chain: t.names.chains.Nomad.Output,
				},
				shared.Chain{
					Table: t.names.tables.NAT,
					Chain: t.names.chains.Nomad.Postrouting,
				},
			)

			// Add the rules.
			req.Add(
				// Jump rules for the new chains.
				shared.Rule{
					Chain: shared.Chain{
						Table: t.names.tables.NAT,
						Chain: t.names.chains.Output,
					},
					Spec: shared.Spec{Jump: t.names.chains.Nomad.Output},
				},
				shared.Rule{
					Chain: shared.Chain{
						Table: t.names.tables.NAT,
						Chain: t.names.chains.Postrouting,
					},
					Spec: shared.Spec{Jump: t.names.chains.Nomad.Postrouting},
				},
				// Allow forwarding from the loopback to the destination device.
				shared.Rule{
					Chain: shared.Chain{
						Table: t.names.tables.NAT,
						Chain: t.names.chains.Nomad.Postrouting,
					},
					Spec: shared.Spec{
						OutInterface:    dstIface,
						SourceType:      "LOCAL",
						DestinationType: "UNICAST",
						Jump:            "MASQUERADE",
					},
				},
				// Enable the actual forward.
				shared.Rule{
					Chain: shared.Chain{
						Table: t.names.tables.NAT,
						Chain: t.names.chains.Nomad.Output,
					},
					Spec: shared.Spec{
						Source:          reservedPort.HostIP,
						OutInterface:    iface,
						Protocol:        "tcp",
						DestinationPort: reservedPort.Value,
						Jump:            "DNAT",
						ToDestination:   fmt.Sprintf("%s:%d", ip, reservedPort.To),
					},
					Removable: true,
				},
			)
		} else {
			// Add prerouting and filtering rule to enable the forward.
			req.Add(
				shared.Rule{
					Chain: shared.Chain{
						Table: t.names.tables.NAT,
						Chain: t.names.chains.Nomad.Prerouting,
					},
					Spec: shared.Spec{
						Destination:     reservedPort.HostIP,
						InInterface:     iface,
						Protocol:        "tcp",
						DestinationPort: reservedPort.Value,
						Jump:            "DNAT",
						ToDestination:   fmt.Sprintf("%s:%d", ip, reservedPort.To),
					},
					Removable: true,
				},
				shared.Rule{
					Chain: shared.Chain{
						Table: t.names.tables.Filter,
						Chain: t.names.chains.Nomad.Forward,
					},
					Spec: shared.Spec{
						Destination:     ip,
						Protocol:        "tcp",
						State:           "NEW",
						DestinationPort: reservedPort.To,
						Jump:            "ACCEPT",
					},
					Removable: true,
				},
			)
		}
	}

	if err := t.backend.Add(req); err != nil {
		return nil, err
	}

	return &virtnet.FilterRemoval{
		Name: t.removalName(),
		Data: req.Teardown(),
	}, nil
}

// Teardown removes rules from the packet filter.
func (t *tables) Teardown(removal *virtnet.FilterRemoval) error {
	// If there is no removal information then there
	// is nothing to do.
	if removal == nil || removal.Data == nil {
		return nil
	}

	teardown, ok := removal.Data.(shared.Teardown)
	if !ok {
		t.logger.Error("invalid teardown data received", "name", removal.Name,
			"type", hclog.Fmt("%T", removal.Data))
		return fmt.Errorf("%w - invalid teardown data", errs.ErrPacketFilterTeardown)
	}

	t.logger.Trace("processing teardown", "removal-name", removal.Name, "teardown", teardown)

	return t.backend.Remove(teardown)
}

// setup is responsible for ensuring the local host machine packet
// filter is properly configured for the driver.
func (t *tables) setup() error {
	req := shared.NewRequest("")

	// Add the custom NAT and filter chains.
	req.Add(
		shared.Chain{
			Table: t.names.tables.NAT,
			Chain: t.names.chains.Nomad.Prerouting,
		},
		shared.Chain{
			Table: t.names.tables.Filter,
			Chain: t.names.chains.Nomad.Forward,
		},
	)
	// Add the jump rules to the custom chains.
	req.Add(
		shared.Rule{
			Chain: shared.Chain{
				Table: t.names.tables.NAT,
				Chain: t.names.chains.Prerouting,
			},
			Spec: shared.Spec{
				Jump: t.names.chains.Nomad.Prerouting,
			},
			Position: 1,
		},
		shared.Rule{
			Chain: shared.Chain{
				Table: t.names.tables.Filter,
				Chain: t.names.chains.Forward,
			},
			Spec: shared.Spec{
				Jump: t.names.chains.Nomad.Forward,
			},
			Position: 1,
		},
	)

	// Apply any updates that are required.
	if err := t.backend.Add(req); err != nil {
		return fmt.Errorf("setup failure: %w", err)
	}

	return nil
}

// removalName returns the name used for the removal record.
func (t *tables) removalName() string {
	return removalPrefix + t.backend.Name()
}

// loopbackPortForwardsSupported returns if the host has been configured for routing localnet packets.
// NOTE: The global configuration overrides device specific configuration.
func (t *tables) loopbackPortForwardsSupported(device string) bool {
	for _, configName := range []string{routeLocalnetGlobalName, device} {
		tmpl := t.routeLocalnetPathTemplate
		if tmpl == "" {
			tmpl = routeLocalnetPathTemplate
		}

		cfgPath := fmt.Sprintf(tmpl, configName)
		content, err := os.ReadFile(cfgPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			t.logger.Error("read failed for device loopback support check", "path", cfgPath, "error", err)
			return false
		}

		if strings.TrimSpace(string(content)) == "1" {
			return true
		}
	}

	return false
}

// cleanup is used to cleanup the backend during testing.
func (t *tables) cleanup() {
	if t.backend == nil {
		return
	}

	// If no names are set, chains cannot be removed.
	if t.names == nil {
		return
	}

	// Create a teardown request with the nomad specific chains and
	// remove them.
	t.backend.Remove(
		shared.Teardown{
			Chains: []shared.Chain{
				{
					Table: "",
					Chain: t.names.chains.Nomad.Forward,
				},
				{
					Table: "",
					Chain: t.names.chains.Nomad.Postrouting,
				},
				{
					Table: "",
					Chain: t.names.chains.Nomad.Prerouting,
				},
				{
					Table: "",
					Chain: t.names.chains.Nomad.Output,
				},
			},
		},
	)
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

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package net

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

// MacvtapMode represents the operating mode of a macvtap interface.
type MacvtapMode string

const (
	// MacvtapModeBridge allows the VM to communicate with other VMs on the
	// same host and with the external network, but not with the host itself.
	MacvtapModeBridge MacvtapMode = "bridge"

	// MacvtapModePrivate isolates the VM so it can only communicate with the
	// external network. Communication with the host and other VMs on the same
	// lower device is blocked.
	MacvtapModePrivate MacvtapMode = "private"

	// MacvtapModeVEPA (Virtual Ethernet Port Aggregator) forwards all traffic
	// to an external switch that is responsible for hairpin routing between
	// VMs on the same host.
	MacvtapModeVEPA MacvtapMode = "vepa"

	// MacvtapModePassthrough passes a single lower device exclusively to the
	// VM, giving it direct access to the physical interface. The lower device
	// is set into promiscuous mode.
	MacvtapModePassthrough MacvtapMode = "passthrough"
)

// validMacvtapModes is the set of accepted MacvtapMode values.
var validMacvtapModes = []MacvtapMode{
	MacvtapModeBridge,
	MacvtapModePrivate,
	MacvtapModeVEPA,
	MacvtapModePassthrough,
}

// NetworkInterfacesConfig is the list of network interfaces that should be
// added to a VM. Currently, the driver only supports a single entry which is
// validated within the Validate function.
//
// Due to its type, callers will need to dereference the object before
// performing iteration.
type NetworkInterfacesConfig []*NetworkInterfaceConfig

// Equal returns if the given NetworkInterfacesConfig is equal.
func (n NetworkInterfacesConfig) Equal(rhs NetworkInterfacesConfig) bool {
	if len(n) != len(rhs) {
		return false
	}

	for i, lhs := range n {
		if !lhs.Equal(rhs[i]) {
			return false
		}
	}

	return true
}

// NetworkInterfaceConfig contains all the possible network interface options
// that a VM currently supports via the Nomad driver.
type NetworkInterfaceConfig struct {
	Bridge  *NetworkInterfaceBridgeConfig  `codec:"bridge"`
	Macvtap *NetworkInterfaceMacvtapConfig `codec:"macvtap"`
}

// Equal returns if the given NetworkInterfaceConfig is equal.
func (n *NetworkInterfaceConfig) Equal(rhs *NetworkInterfaceConfig) bool {
	if n == nil || rhs == nil {
		return false
	}

	if !n.Bridge.Equal(rhs.Bridge) {
		return false
	}

	if !n.Macvtap.Equal(rhs.Macvtap) {
		return false
	}

	return true
}

// NetworkInterfaceBridgeConfig is the network object when a VM is attached to
// a bridged network interface.
type NetworkInterfaceBridgeConfig struct {

	// Name is the name of the bridge interface to use. This relates to the
	// output seen from commands such as "ip addr show" or "virsh net-info".
	Name string `codec:"name"`

	// Ports contains a list of port labels which will be exposed on the host
	// via mapping to the network interface. These labels must exist within the
	// job specification network block.
	Ports []string `codec:"ports"`
}

// Equal returns if the given NetworkInterfaceBridgeConfig is equal.
func (n *NetworkInterfaceBridgeConfig) Equal(rhs *NetworkInterfaceBridgeConfig) bool {
	if n == nil && rhs == nil {
		return true
	}

	if n == nil || rhs == nil {
		return false
	}

	if n.Name != rhs.Name {
		return false
	}

	if slices.Compare(n.Ports, rhs.Ports) != 0 {
		return false
	}

	return true
}

// NetworkInterfaceMacvtapConfig is the network object when a VM is attached to
// a macvtap interface.
type NetworkInterfaceMacvtapConfig struct {

	// Device is the name of the lower (physical or virtual) network device
	// that the macvtap interface will be created on top of. This should match
	// an interface visible in "ip addr show" output (e.g. "eth0", "ens3").
	Device string `codec:"device"`

	// Mode controls the traffic isolation policy of the macvtap interface.
	// Accepted values are: "bridge", "private", "vepa", and "passthrough".
	// Defaults to "bridge" when not specified.
	Mode MacvtapMode `codec:"mode"`
}

// Equal returns if the given NetworkInterfaceMacvtapConfig is equal.
func (n *NetworkInterfaceMacvtapConfig) Equal(rhs *NetworkInterfaceMacvtapConfig) bool {
	if n == nil && rhs == nil {
		return true
	}

	if n == nil || rhs == nil {
		return false
	}

	if n.Device != rhs.Device {
		return false
	}

	if n.Mode != rhs.Mode {
		return false
	}

	return true
}

// Validate ensures the NetworkInterfaces is a valid object supported by the
// driver. Any error returned here should be considered terminal for a task
// and stop the process execution.
func (n *NetworkInterfacesConfig) Validate() error {
	if n == nil {
		return nil
	}

	var mErr *multierror.Error

	// The driver only currently supports a single network interface per VM due
	// to constraints on Nomad's network mapping handling and the driver
	// itself.
	if len(*n) > 1 {
		mErr = multierror.Append(mErr,
			fmt.Errorf("%w: only one network interface can be configured", errs.ErrInvalidConfiguration))
	}

	// Iterate the network interfaces and validate each object to be correct
	// according to their type.
	// NOTE: only a single interface is allowed currently, but assume that will change.
	for i, netInterface := range *n {
		errPrefix := fmt.Sprintf("network_interface[%d] -", i+1)
		if netInterface.Bridge != nil && netInterface.Macvtap != nil {
			mErr = multierror.Append(mErr,
				fmt.Errorf("%s %w: bridge and macvtap are mutually exclusive", errPrefix, errs.ErrInvalidConfiguration))
			continue
		}

		if netInterface.Bridge != nil {
			mErr = multierror.Append(mErr, errs.MissingAttribute("bridge.name",
				netInterface.Bridge.Name, errs.WithPrefix(errPrefix)))
		}

		if netInterface.Macvtap != nil {
			mErr = multierror.Append(mErr, errs.MissingAttribute("macvtap.device",
				netInterface.Macvtap.Device, errs.WithPrefix(errPrefix)))

			// Default the mode to bridge when unset, matching common macvtap
			// usage and libvirt's own default behaviour.
			if netInterface.Macvtap.Mode == "" {
				netInterface.Macvtap.Mode = MacvtapModeBridge
			}

			if !slices.Contains(validMacvtapModes, netInterface.Macvtap.Mode) {
				validModes := make([]string, len(validMacvtapModes))
				for i, v := range validMacvtapModes {
					validModes[i] = string(v)
				}
				mErr = multierror.Append(mErr,
					fmt.Errorf("%s %w: macvtap has invalid mode %q; must be one of: %s", errPrefix, errs.ErrInvalidConfiguration, netInterface.Macvtap.Mode,
						strings.Join(validModes, ", "),
					),
				)
			}
		}
	}

	return mErr.ErrorOrNil()
}

// NetworkInterfaceHCLSpec returns the HCL specification for a virtual machines
// network interface object.
func NetworkInterfaceHCLSpec() *hclspec.Spec {
	return hclspec.NewBlockList("network_interface", hclspec.NewObject(map[string]*hclspec.Spec{
		"bridge": hclspec.NewBlock("bridge", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"name":  hclspec.NewAttr("name", "string", true),
			"ports": hclspec.NewAttr("ports", "list(string)", false),
		})),
		"macvtap": hclspec.NewBlock("macvtap", false, hclspec.NewObject(map[string]*hclspec.Spec{
			"device": hclspec.NewAttr("device", "string", true),
			"mode": hclspec.NewDefault(
				hclspec.NewAttr("mode", "string", false),
				hclspec.NewLiteral(fmt.Sprintf("%q", MacvtapModeBridge)),
			),
		})),
	}))
}

// ConfigurableOnly returns a new NetworkInterfacesConfig containing only configurable
// interfaces. This is used to filter out interface types such as macvtap that manage
// their own network identity and have no interaction with Nomad's host-side port
// mapping machinery.
func (n NetworkInterfacesConfig) ConfigurableOnly() NetworkInterfacesConfig {
	var out NetworkInterfacesConfig
	for _, iface := range n {
		if iface.Bridge != nil {
			out = append(out, iface)
		}
	}
	return out
}

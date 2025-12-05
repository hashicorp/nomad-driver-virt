// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package net

import (
	"errors"
	"fmt"

	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
)

// NetworkInterfacesConfig is the list of network interfaces that should be
// added to a VM. Currently, the driver only supports a single entry which is
// validated within the Validate function.
//
// Due to its type, callers will need to dereference the object before
// performing iteration.
type NetworkInterfacesConfig []*NetworkInterfaceConfig

// NetworkInterfaceConfig contains all the possible network interface options
// that a VM currently supports via the Nomad driver.
type NetworkInterfaceConfig struct {
	Bridge *NetworkInterfaceBridgeConfig `codec:"bridge"`
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

// Validate ensures the NetworkInterfaces is a valid object supported by the
// driver. Any error returned here should be considered terminal for a task
// and stop the process execution.
func (n *NetworkInterfacesConfig) Validate() error {

	if n == nil {
		return nil
	}

	var mErr multierror.Error

	// The driver only currently supports a single network interface per VM due
	// to constraints on Nomad's network mapping handling and the driver
	// itself.
	if len(*n) > 1 {
		mErr.Errors = append(mErr.Errors,
			errors.New("only one network interface can be configured"))
	}

	// Iterate the network interfaces and validate each object to be correct
	// according to their type.
	for i, netInterface := range *n {
		if netInterface.Bridge != nil && netInterface.Bridge.Name == "" {
			mErr.Errors = append(mErr.Errors,
				fmt.Errorf("network interface bridge '%v' requires name parameter", i))
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
	}))
}

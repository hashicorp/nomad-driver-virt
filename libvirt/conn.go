// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"github.com/hashicorp/nomad-driver-virt/virt/shim"
	"libvirt.org/go/libvirt"
)

// Connect is the "real" shim to the libvirt backend that implements the
// Connect interface.
type Connect struct {
	conn *libvirt.Connect
}

func NewConnect(conn *libvirt.Connect) shim.Connect {
	return &Connect{conn: conn}
}

func (c *Connect) ListNetworks() ([]string, error) {
	return c.conn.ListNetworks()
}

func (c *Connect) LookupNetworkByName(name string) (shim.Network, error) {
	return c.conn.LookupNetworkByName(name)
}

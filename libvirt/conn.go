// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"libvirt.org/go/libvirt"
)

// Connect is the production code of the libvirt backend that implements the
// ConnectShim interface.
type Connect struct {
	conn *libvirt.Connect
}

func NewConnect(conn *libvirt.Connect) ConnectShim {
	return &Connect{conn: conn}
}

func (c *Connect) ListNetworks() ([]string, error) {
	return c.conn.ListNetworks()
}

func (c *Connect) LookupNetworkByName(name string) (ConnectNetworkShim, error) {
	return c.conn.LookupNetworkByName(name)
}

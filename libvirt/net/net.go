// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package net

import (
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/libvirt"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
)

// Controller implements to Net interface and is the main/only way in which the
// driver should interact with the network-subsystem.
type Controller struct {
	logger  hclog.Logger
	netConn libvirt.ConnectShim
}

// NewController returns a Controller which implements the net.Net interface
// and has a named logger, to ensure log messages can be easily tied to the
// network system.
func NewController(logger hclog.Logger, conn libvirt.ConnectShim) net.Net {
	return &Controller{
		logger:  logger.Named("net"),
		netConn: conn,
	}
}

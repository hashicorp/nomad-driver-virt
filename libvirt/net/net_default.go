// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !linux

package net

import (
	stdnet "net"

	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

func (c *Controller) Fingerprint(_ map[string]*structs.Attribute) {}

func (c *Controller) Init() error { return nil }

func (c *Controller) VMStartedBuild(_ *net.VMStartedBuildRequest) (*net.VMStartedBuildResponse, error) {
	return nil, nil
}

func (c *Controller) VMTerminatedTeardown(_ *net.VMTerminatedTeardownRequest) (*net.VMTerminatedTeardownResponse, error) {
	return nil, nil
}

func getInterfaceByIP(_ stdnet.IP) (string, error) { return "", nil }

// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !linux

package chnet

import (
	"fmt"

	"github.com/hashicorp/go-hclog"
	domain "github.com/ccheshirecat/nomad-driver-ch/internal/shared"
	"github.com/ccheshirecat/nomad-driver-ch/virt/net"
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

// Controller is a stub implementation for non-Linux platforms
type Controller struct {
	logger hclog.Logger
}

// NewController returns a stub controller for non-Linux platforms
func NewController(logger hclog.Logger, networkConfig *domain.Network) *Controller {
	return &Controller{
		logger: logger.Named("chnet-stub"),
	}
}

// Fingerprint is not supported on non-Linux platforms
func (c *Controller) Fingerprint(attr map[string]*structs.Attribute) {
	c.logger.Warn("network fingerprinting not supported on this platform")
}

// Init is not supported on non-Linux platforms
func (c *Controller) Init() error {
	return fmt.Errorf("Cloud Hypervisor networking is only supported on Linux")
}

// VMStartedBuild is not supported on non-Linux platforms
func (c *Controller) VMStartedBuild(req *net.VMStartedBuildRequest) (*net.VMStartedBuildResponse, error) {
	return nil, fmt.Errorf("Cloud Hypervisor networking is only supported on Linux")
}

// VMTerminatedTeardown is not supported on non-Linux platforms
func (c *Controller) VMTerminatedTeardown(req *net.VMTerminatedTeardownRequest) (*net.VMTerminatedTeardownResponse, error) {
	return nil, fmt.Errorf("Cloud Hypervisor networking is only supported on Linux")
}
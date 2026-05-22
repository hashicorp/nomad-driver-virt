// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

//go:build !linux

package arp

import (
	"context"
	"fmt"
	"net"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/internal/errs"
)

// IsAvailable returns if ARP functionality is available.
func IsAvailable() bool {
	return false
}

func New() *arper {
	return &arper{}
}

type arper struct{}

func (a *arper) Discover(context.Context, *net.Interface, net.HardwareAddr) (<-chan net.IP, error) {
	return nil, fmt.Errorf("ARP is %w on this platform", errs.ErrNotImplemented)
}

func (a *arper) SetLogger(hclog.Logger) {}

func (a *arper) SetContext(context.Context) {}

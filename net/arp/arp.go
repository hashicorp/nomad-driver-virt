// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package arp

import (
	"context"
	"net"

	"github.com/hashicorp/go-hclog"
)

type ARP interface {
	// Discover will poll known ARP records on an interface for a matching MAC
	// address and send any detected IP addresses to the channel for the life
	// of the context.
	Discover(context.Context, *net.Interface, net.HardwareAddr) (<-chan net.IP, error)

	// SetLogger sets a custom logger.
	SetLogger(hclog.Logger)

	// SetContext sets a custom context.
	SetContext(context.Context)
}

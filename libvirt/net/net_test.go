// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package net

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/shoenig/test/must"
)

func Test_NewController(t *testing.T) {
	c := NewController(hclog.NewNullLogger(), nil)
	must.Eq(t, c.dhcpLeaseDiscoveryInterval, defaultDHCPLeaseDiscoveryInterval)
	must.Eq(t, c.dhcpLeaseDiscoveryTimeout, defaultDHCPLeaseDiscoveryTimeout)
	must.NotNil(t, c.interfaceByIPGetter)
}

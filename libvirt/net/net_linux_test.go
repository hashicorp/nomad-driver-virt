// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package net

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/libvirt"
	"github.com/hashicorp/nomad/plugins/shared/structs"
	"github.com/shoenig/test/must"
)

func TestController_Fingerprint(t *testing.T) {

	// Use a populated mock shim to test that we query and correctly populate
	// the passed attributes.
	mockController := NewController(hclog.NewNullLogger(), &libvirt.ConnectMock{})

	mockControllerAttrs := map[string]*structs.Attribute{}
	mockController.Fingerprint(mockControllerAttrs)

	expectedOutput := map[string]*structs.Attribute{
		"driver.virt.network.default.state":       structs.NewStringAttribute("active"),
		"driver.virt.network.default.bridge_name": structs.NewStringAttribute("virbr0"),
		"driver.virt.network.routed.state":        structs.NewStringAttribute("inactive"),
		"driver.virt.network.routed.bridge_name":  structs.NewStringAttribute("br0"),
	}
	must.Eq(t, expectedOutput, mockControllerAttrs)

	// Set the shim to our empty mock, to ensure we do not panic or have any
	// other undesired outcome when the process does not find any networks
	// available on the host.
	mockEmptyController := NewController(hclog.NewNullLogger(), &libvirt.ConnectMockEmpty{})

	mockEmptyControllerAttrs := map[string]*structs.Attribute{}
	mockEmptyController.Fingerprint(mockEmptyControllerAttrs)
	must.Eq(t, map[string]*structs.Attribute{}, mockEmptyControllerAttrs)
}

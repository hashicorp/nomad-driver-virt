// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !linux

package net

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/libvirt"
	"github.com/hashicorp/nomad/plugins/shared/structs"
	"github.com/shoenig/test/must"
)

func TestController_Fingerprint(t *testing.T) {
	mockController := NewController(hclog.NewNullLogger(), &libvirt.ConnectMock{})
	mockControllerAttrs := map[string]*structs.Attribute{}
	mockController.Fingerprint(mockControllerAttrs)
	must.Eq(t, map[string]*structs.Attribute{}, mockControllerAttrs)
}

func TestController_Init(t *testing.T) {
	mockController := NewController(hclog.NewNullLogger(), &libvirt.ConnectMock{})
	must.NoError(t, mockController.Init())
}

func TestController_PostStart(t *testing.T) {
	mockController := NewController(hclog.NewNullLogger(), &libvirt.ConnectMock{})
	resp, err := mockController.VMStartedBuild(nil)
	must.NoError(t, err)
	must.Nil(t, resp)
}

func TestController_PostStop(t *testing.T) {
	mockController := NewController(hclog.NewNullLogger(), &libvirt.ConnectMock{})
	resp, err := mockController.VMTerminatedTeardown(nil)
	must.NoError(t, err)
	must.Nil(t, resp)
}

func Test_getInterfaceByIP(t *testing.T) {
	resp, err := getInterfaceByIP(nil)
	must.NoError(t, err)
	must.Eq(t, "", resp)
}

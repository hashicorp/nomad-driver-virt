// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package net

import (
	"maps"

	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

func NewStatic() *StaticNet {
	return &StaticNet{}
}

type StaticNet struct {
	FingerprintResult          map[string]*structs.Attribute
	VMStartedBuildResult       *net.VMStartedBuildResponse
	VMTerminatedTeardownResult *net.VMTerminatedTeardownResponse
}

func (s *StaticNet) Init() error {
	return nil
}

func (s *StaticNet) Fingerprint(attrs map[string]*structs.Attribute) {
	if s.FingerprintResult == nil {
		return
	}

	maps.Copy(attrs, s.FingerprintResult)
}

func (s *StaticNet) VMStartedBuild(*net.VMStartedBuildRequest) (*net.VMStartedBuildResponse, error) {
	if s.VMStartedBuildResult != nil {
		return s.VMStartedBuildResult, nil
	}

	return &net.VMStartedBuildResponse{}, nil
}

func (s *StaticNet) VMTerminatedTeardown(*net.VMTerminatedTeardownRequest) (*net.VMTerminatedTeardownResponse, error) {
	if s.VMTerminatedTeardownResult != nil {
		return s.VMTerminatedTeardownResult, nil
	}

	return &net.VMTerminatedTeardownResponse{}, nil
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	mock_net "github.com/hashicorp/nomad-driver-virt/testutil/mock/virt/net"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

func NewStatic() *StaticVirt {
	return &StaticVirt{}
}

type StaticVirt struct {
	GetInfoResult              vm.VirtualizerInfo
	GetVMResult                *vm.Info
	GetNetworkInterfacesResult []vm.NetworkInterface
	FingerprintResult          map[string]*structs.Attribute
	NetworkingResult           net.Net
	UseCloudInitResult         bool
}

func (s *StaticVirt) Init() error {
	return nil
}

func (s *StaticVirt) CreateVM(*vm.Config) error {
	return nil
}

func (s *StaticVirt) StopVM(string) error {
	return nil
}

func (s *StaticVirt) DestroyVM(string) error {
	return nil
}

func (s *StaticVirt) GetVM(string) (*vm.Info, error) {
	if s.GetVMResult != nil {
		return s.GetVMResult, nil
	}

	return nil, vm.ErrNotFound
}

func (s *StaticVirt) GetNetworkInterfaces(string) ([]vm.NetworkInterface, error) {
	if s.GetNetworkInterfacesResult != nil {
		return s.GetNetworkInterfacesResult, nil
	}

	return []vm.NetworkInterface{}, nil
}

func (s *StaticVirt) UseCloudInit() bool {
	return s.UseCloudInitResult
}

func (s *StaticVirt) Networking() (net.Net, error) {
	if s.NetworkingResult != nil {
		return s.NetworkingResult, nil
	}

	return mock_net.NewStatic(), nil
}

func (s *StaticVirt) Fingerprint() (map[string]*structs.Attribute, error) {
	if s.FingerprintResult != nil {
		return s.FingerprintResult, nil
	}

	return make(map[string]*structs.Attribute), nil
}

func (s *StaticVirt) GetInfo() (vm.VirtualizerInfo, error) {
	return s.GetInfoResult, nil
}

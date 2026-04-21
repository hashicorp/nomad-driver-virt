// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/storage"
	mock_storage "github.com/hashicorp/nomad-driver-virt/testutil/mock/storage"
	mock_net "github.com/hashicorp/nomad-driver-virt/testutil/mock/virt/net"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

func NewStatic() *StaticVirt {
	return &StaticVirt{}
}

type StaticVirt struct {
	GetInfoResult               vm.VirtualizerInfo
	GetVMResult                 *vm.Info
	GetNetworkInterfacesResult  []vm.NetworkInterface
	GenerateMountCommandsResult []string
	FingerprintResult           map[string]*structs.Attribute
	NetworkingResult            net.Net
	UseCloudInitResult          bool
	StorageResult               storage.Storage

	counts map[string]int
	m      sync.Mutex
	o      sync.Once
}

func (s *StaticVirt) incrCount() {
	s.o.Do(func() {
		s.counts = make(map[string]int)
	})

	ctr, _, _, ok := runtime.Caller(1)
	if !ok {
		panic("unable to get caller information")
	}
	info := runtime.FuncForPC(ctr)
	if info == nil {
		panic("unable to get function information")
	}

	name := info.Name()[strings.LastIndex(info.Name(), ".")+1:]
	s.counts[name]++
}

func (s *StaticVirt) CallCount(fnName string) int {
	s.m.Lock()
	defer s.m.Unlock()

	if s.counts == nil {
		return 0
	}

	return s.counts[fnName]
}

func (s *StaticVirt) Init() error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticVirt) CreateVM(*vm.Config) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticVirt) StopVM(string) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticVirt) DestroyVM(string) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticVirt) GetVM(string) (*vm.Info, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.GetVMResult != nil {
		return s.GetVMResult, nil
	}

	return nil, errs.ErrNotFound
}

func (s *StaticVirt) GetNetworkInterfaces(string) ([]vm.NetworkInterface, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.GetNetworkInterfacesResult != nil {
		return s.GetNetworkInterfacesResult, nil
	}

	return []vm.NetworkInterface{}, nil
}

func (s *StaticVirt) GenerateMountCommands([]*vm.MountFileConfig) ([]string, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.GenerateMountCommandsResult != nil {
		return s.GenerateMountCommandsResult, nil
	}

	return []string{}, nil
}

func (s *StaticVirt) UseCloudInit() bool {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.UseCloudInitResult
}

func (s *StaticVirt) Networking() (net.Net, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.NetworkingResult == nil {
		s.NetworkingResult = mock_net.NewStatic()
	}

	return s.NetworkingResult, nil
}

func (s *StaticVirt) Fingerprint() (map[string]*structs.Attribute, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.FingerprintResult != nil {
		return s.FingerprintResult, nil
	}

	return make(map[string]*structs.Attribute), nil
}

func (s *StaticVirt) GetInfo() (vm.VirtualizerInfo, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.GetInfoResult, nil
}

func (s *StaticVirt) SetupStorage(*storage.Config) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticVirt) Storage() storage.Storage {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.StorageResult == nil {
		s.StorageResult = mock_storage.NewStaticStorage()
	}

	return s.StorageResult
}

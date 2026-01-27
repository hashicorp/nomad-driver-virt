// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package providers

import (
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/virt"
	"github.com/hashicorp/nomad/plugins/drivers"
)

func NewStatic(v virt.Virtualizer) *StaticProviders {
	return &StaticProviders{virtualizer: v}
}

type StaticProviders struct {
	virtualizer       virt.Virtualizer
	GetVMResult       *vm.Info
	FingerprintResult *drivers.Fingerprint
}

func (s *StaticProviders) Setup(*virt.Config) error {
	return nil
}

func (s *StaticProviders) Get(string) (virt.Virtualizer, error) {
	return s.virtualizer, nil
}

func (s *StaticProviders) Default() (virt.Virtualizer, error) {
	return s.virtualizer, nil
}

func (s *StaticProviders) GetVM(name string) (*vm.Info, error) {
	if s.GetVMResult != nil {
		return s.GetVMResult, nil
	}

	if s.virtualizer != nil {
		if info, _ := s.virtualizer.GetVM(name); info != nil {
			return info, nil
		}
	}

	return &vm.Info{}, nil
}

func (s *StaticProviders) GetProviderForVM(string) (virt.Virtualizer, error) {
	return s.virtualizer, nil
}

func (s *StaticProviders) Fingerprint() (*drivers.Fingerprint, error) {
	if s.FingerprintResult != nil {
		return s.FingerprintResult, nil
	}

	return &drivers.Fingerprint{}, nil
}

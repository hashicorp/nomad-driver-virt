// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package providers

import (
	"context"
	"runtime"
	"strings"
	"sync"

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

	counts map[string]int
	m      sync.Mutex
	o      sync.Once
}

func (s *StaticProviders) incrCount() {
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

func (s *StaticProviders) CallCount(fnName string) int {
	s.m.Lock()
	defer s.m.Unlock()

	if s.counts == nil {
		return 0
	}

	return s.counts[fnName]
}

func (s *StaticProviders) Setup(c *virt.Config) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	// If a default virtualizer is set, pass the calls
	// through that would be done during normal setup
	if s.virtualizer != nil {
		s.virtualizer.Init()
		n, _ := s.virtualizer.Networking()
		n.Init()
		s.virtualizer.SetupStorage(c.StoragePools)
	}

	return nil
}

func (s *StaticProviders) Get(context.Context, string) (virt.Virtualizer, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.virtualizer, nil
}

func (s *StaticProviders) Default(context.Context) (virt.Virtualizer, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.virtualizer, nil
}

func (s *StaticProviders) GetVM(name string) (*vm.Info, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

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

func (s *StaticProviders) GetProviderForVM(context.Context, string) (virt.Virtualizer, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.virtualizer, nil
}

func (s *StaticProviders) Fingerprint() (*drivers.Fingerprint, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.FingerprintResult != nil {
		return s.FingerprintResult, nil
	}

	return &drivers.Fingerprint{}, nil
}

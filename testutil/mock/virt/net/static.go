// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package net

import (
	"maps"
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

func NewStatic() *StaticNet {
	return &StaticNet{}
}

type StaticNet struct {
	FingerprintResult          map[string]*structs.Attribute // This value will be copied into received attrs
	VMStartedBuildResult       *net.VMStartedBuildResponse
	VMTerminatedTeardownResult *net.VMTerminatedTeardownResponse

	counts map[string]int
	m      sync.Mutex
	o      sync.Once
}

func (s *StaticNet) incrCount() {
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

func (s *StaticNet) CallCount(fnName string) int {
	s.m.Lock()
	defer s.m.Unlock()

	if s.counts == nil {
		return 0
	}

	return s.counts[fnName]
}

func (s *StaticNet) Init() error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticNet) Fingerprint(attrs map[string]*structs.Attribute) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.FingerprintResult == nil {
		return
	}

	maps.Copy(attrs, s.FingerprintResult)
}

func (s *StaticNet) VMStartedBuild(*net.VMStartedBuildRequest) (*net.VMStartedBuildResponse, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.VMStartedBuildResult != nil {
		return s.VMStartedBuildResult, nil
	}

	return &net.VMStartedBuildResponse{}, nil
}

func (s *StaticNet) VMTerminatedTeardown(*net.VMTerminatedTeardownRequest) (*net.VMTerminatedTeardownResponse, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.VMTerminatedTeardownResult != nil {
		return s.VMTerminatedTeardownResult, nil
	}

	return &net.VMTerminatedTeardownResponse{}, nil
}

// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package cloudinit

import (
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/nomad-driver-virt/cloudinit"
	"github.com/shoenig/test/must"
)

func NewMockCloudInit(t must.T) *MockCloudInit {
	return &MockCloudInit{t: t}
}

func NewStaticCloudInit() *StaticCloudInit {
	return &StaticCloudInit{}
}

type StaticCloudInit struct {
	counts map[string]int
	m      sync.Mutex
	o      sync.Once
}

func (s *StaticCloudInit) incrCount() {
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

func (s *StaticCloudInit) CallCount(fnName string) int {
	s.m.Lock()
	defer s.m.Unlock()

	if s.counts == nil {
		return 0
	}

	return s.counts[fnName]
}

func (s *StaticCloudInit) Apply(*cloudinit.Config, string) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

type Apply struct {
	Config *cloudinit.Config
	Path   string
	Err    error
}

type MockCloudInit struct {
	t must.T
	m sync.Mutex

	apply []Apply
}

func (m *MockCloudInit) Expect(calls ...any) *MockCloudInit {
	for _, call := range calls {
		switch c := call.(type) {
		case Apply:
			m.ExpectApply(c)
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockCloudInit) ExpectApply(c Apply) *MockCloudInit {
	m.m.Lock()
	defer m.m.Unlock()

	m.apply = append(m.apply, c)
	return m
}

func (m *MockCloudInit) Apply(config *cloudinit.Config, path string) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.apply,
		must.Sprint("Unexpected call to Apply"))
	call := m.apply[0]
	m.apply = m.apply[1:]

	must.Eq(m.t, call, Apply{
		Config: config,
		Path:   path,
		Err:    call.Err,
	}, must.Sprint("Apply received incorrect arguments"))

	return call.Err
}

func (m *MockCloudInit) AssertExpectations() {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceEmpty(m.t, m.apply,
		must.Sprintf("Apply expecting %d more invocations", len(m.apply)))
}

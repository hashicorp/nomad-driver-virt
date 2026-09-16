// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package backend

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/nomad-driver-virt/net/filter/linux/shared"
	"github.com/shoenig/test/must"
)

type StaticBackend struct {
	NameResult string

	counts map[string]int
	m      sync.Mutex
	o      sync.Once
}

func NewStatic() *StaticBackend {
	return &StaticBackend{}
}

func (s *StaticBackend) incrCount() {
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

func (s *StaticBackend) CallCount(fnName string) int {
	s.m.Lock()
	defer s.m.Unlock()

	if s.counts == nil {
		return 0
	}

	return s.counts[fnName]
}

func (s *StaticBackend) Name() string {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.NameResult
}

func (s *StaticBackend) Add(*shared.Request) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticBackend) Remove(shared.Teardown) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

type MockBackend struct {
	adds    []Add
	names   []Name
	removes []Remove
	t       must.T
	m       sync.Mutex
}

type Add struct {
	Chains []shared.Chain
	Rules  []shared.Rule
	Err    error
}

type Name struct {
	Name string
}

type Remove struct {
	Teardown shared.Teardown
	Err      error
}

func NewMock(t must.T) *MockBackend {
	return &MockBackend{t: t}
}

func (m *MockBackend) Expect(calls ...any) *MockBackend {
	for _, call := range calls {
		switch c := call.(type) {
		case Add:
			m.ExpectAdd(c)
		case Name:
			m.ExpectName(c)
		case Remove:
			m.ExpectRemove(c)
		default:
			panic(fmt.Sprintf("unsupported type for mock expectation: %T", c))
		}
	}

	return m
}

func (m *MockBackend) ExpectAdd(c Add) *MockBackend {
	m.m.Lock()
	defer m.m.Unlock()

	m.adds = append(m.adds, c)
	return m
}

func (m *MockBackend) ExpectName(c Name) *MockBackend {
	m.m.Lock()
	defer m.m.Unlock()

	m.names = append(m.names, c)
	return m
}

func (m *MockBackend) ExpectRemove(c Remove) *MockBackend {
	m.m.Lock()
	defer m.m.Unlock()

	m.removes = append(m.removes, c)
	return m
}

func (m *MockBackend) Add(req *shared.Request) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.adds,
		must.Sprintf("Unexpected call to Add - Add(%#v)", *req))
	call := m.adds[0]
	m.adds = m.adds[1:]

	must.Eq(m.t, call, Add{Chains: req.Chains(), Rules: req.Rules()},
		must.Sprint("Add received incorrect arguments"))

	return call.Err
}

func (m *MockBackend) Name() string {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.names,
		must.Sprintf("Unexpected call to Name"))
	call := m.names[0]
	m.names = m.names[1:]

	return call.Name
}

func (m *MockBackend) Remove(req shared.Teardown) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.removes,
		must.Sprintf("Unexpected call to Remove - Remove(%#v)", req))
	call := m.removes[0]
	m.removes = m.removes[1:]
	must.Eq(m.t, call.Teardown, req,
		must.Sprint("Remove received incorrect arguments"))

	return call.Err
}

func (m *MockBackend) AssertExpectations() {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceEmpty(m.t, m.adds,
		must.Sprintf("Add expecting %d more invocations", len(m.adds)))
	must.SliceEmpty(m.t, m.names,
		must.Sprintf("Name expecting %d more invocations", len(m.names)))
	must.SliceEmpty(m.t, m.removes,
		must.Sprintf("Remove expecting %d more invocations", len(m.removes)))
}

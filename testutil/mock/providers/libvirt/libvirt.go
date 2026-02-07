// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
	mock_storage "github.com/hashicorp/nomad-driver-virt/testutil/mock/providers/libvirt/storage"
	"github.com/shoenig/test/must"
	"libvirt.org/go/libvirtxml"
)

func NewMockLibvirt(t must.T) *MockLibvirt {
	return &MockLibvirt{t: t}
}

func NewStaticLibvirt() *StaticLibvirt {
	return &StaticLibvirt{}
}

type StaticLibvirt struct {
	CreateStoragePoolResult shims.StoragePool
	FindStoragePoolResult   shims.StoragePool
	NewStreamResult         shims.Stream

	counts map[string]int
	m      sync.Mutex
	o      sync.Once
}

func (s *StaticLibvirt) incrCount() {
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

func (s *StaticLibvirt) CallCount(fnName string) int {
	s.m.Lock()
	defer s.m.Unlock()

	if s.counts == nil {
		return 0
	}

	return s.counts[fnName]
}

func (s *StaticLibvirt) CreateStoragePool(libvirtxml.StoragePool) (shims.StoragePool, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.CreateStoragePoolResult == nil {
		s.CreateStoragePoolResult = mock_storage.NewStaticStoragePool()
	}

	return s.CreateStoragePoolResult, nil
}

func (s *StaticLibvirt) FindStoragePool(string) (shims.StoragePool, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.FindStoragePoolResult == nil {
		s.FindStoragePoolResult = mock_storage.NewStaticStoragePool()
	}

	return s.FindStoragePoolResult, nil
}

func (s *StaticLibvirt) NewStream() (shims.Stream, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.NewStreamResult == nil {
		s.NewStreamResult = NewStaticStream()
	}

	return s.NewStreamResult, nil
}

type CreateStoragePool struct {
	Desc   libvirtxml.StoragePool
	Result shims.StoragePool
	Err    error
}

type FindStoragePool struct {
	Name   string
	Result shims.StoragePool
	Err    error
}

type NewStream struct {
	Result shims.Stream
	Err    error
}

type MockLibvirt struct {
	t must.T

	createStoragePool []CreateStoragePool
	findStoragePool   []FindStoragePool
	newStream         []NewStream
	m                 sync.Mutex
}

func (m *MockLibvirt) Expect(calls ...any) *MockLibvirt {
	for _, call := range calls {
		switch c := call.(type) {
		case CreateStoragePool:
			m.ExpectCreateStoragePool(c)
		case FindStoragePool:
			m.ExpectFindStoragePool(c)
		case NewStream:
			m.ExpectNewStream(c)
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockLibvirt) ExpectCreateStoragePool(c CreateStoragePool) *MockLibvirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.createStoragePool = append(m.createStoragePool, c)
	return m
}

func (m *MockLibvirt) ExpectFindStoragePool(c FindStoragePool) *MockLibvirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.findStoragePool = append(m.findStoragePool, c)
	return m
}

func (m *MockLibvirt) ExpectNewStream(c NewStream) *MockLibvirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.newStream = append(m.newStream, c)
	return m
}

func (m *MockLibvirt) CreateStoragePool(desc libvirtxml.StoragePool) (shims.StoragePool, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.createStoragePool,
		must.Sprint("Unexpected call to CreateStoragePool"))
	call := m.createStoragePool[0]
	m.createStoragePool = m.createStoragePool[1:]

	must.Eq(m.t, call.Desc, desc,
		must.Sprint("CreateStoragePool received incorrect argument"))

	return call.Result, call.Err
}

func (m *MockLibvirt) FindStoragePool(name string) (shims.StoragePool, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.findStoragePool,
		must.Sprint("Unexpected call to FindStoragePool"))
	call := m.findStoragePool[0]
	m.findStoragePool = m.findStoragePool[1:]

	must.Eq(m.t, struct{ Name string }{call.Name}, struct{ Name string }{name},
		must.Sprint("FindStoragePool received incorrect argument"))

	return call.Result, call.Err
}

func (m *MockLibvirt) NewStream() (shims.Stream, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.newStream,
		must.Sprint("Unexpected call to NewStream"))
	call := m.newStream[0]
	m.newStream = m.newStream[1:]

	return call.Result, call.Err
}

func (m *MockLibvirt) AssertExpectations() {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceEmpty(m.t, m.createStoragePool,
		must.Sprintf("CreateStoragePool expecting %d more invocations", len(m.createStoragePool)))
	must.SliceEmpty(m.t, m.findStoragePool,
		must.Sprintf("FindStoragePool expecting %d more invocations", len(m.findStoragePool)))
	must.SliceEmpty(m.t, m.newStream,
		must.Sprintf("NewStream expecting %d more invocations", len(m.newStream)))
}

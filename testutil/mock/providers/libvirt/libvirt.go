// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"runtime"
	"slices"
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
	GetCephSecretResult     string
	GetCephSecretIDResult   string
	FindStoragePoolResult   shims.StoragePool
	NewStreamResult         shims.Stream
	SetCephSecretResult     string

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

func (s *StaticLibvirt) CreateStoragePool(*libvirtxml.StoragePool) (shims.StoragePool, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.CreateStoragePoolResult == nil {
		s.CreateStoragePoolResult = mock_storage.NewStaticStoragePool()
	}

	return s.CreateStoragePoolResult, nil
}

func (s *StaticLibvirt) GetCephSecret(string) (string, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.GetCephSecretResult, nil
}

func (s *StaticLibvirt) GetCephSecretID(string) (string, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.GetCephSecretIDResult, nil
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

func (s *StaticLibvirt) SetCephSecret(string, string) (string, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.SetCephSecretResult, nil
}

func (s *StaticLibvirt) UpdateStoragePool(*libvirtxml.StoragePool) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

type CreateStoragePool struct {
	Desc   *libvirtxml.StoragePool
	Result shims.StoragePool
	Err    error
}

type GetCephSecret struct {
	Name   string
	Result string
	Err    error
}

type GetCephSecretID struct {
	Name   string
	Result string
	Err    error
}

type SetCephSecret struct {
	Name       string
	Result     string
	Credential string
	Err        error
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

type UpdateStoragePool struct {
	Desc *libvirtxml.StoragePool
	Err  error
}

type MockLibvirt struct {
	t must.T

	getCephSecret     []GetCephSecret
	getCephSecretId   []GetCephSecretID
	createStoragePool []CreateStoragePool
	findStoragePool   []FindStoragePool
	newStream         []NewStream
	setCephSecret     []SetCephSecret
	updateStoragePool []UpdateStoragePool

	m sync.Mutex
}

func (m *MockLibvirt) Expect(calls ...any) *MockLibvirt {
	for _, call := range calls {
		switch c := call.(type) {
		case CreateStoragePool:
			m.ExpectCreateStoragePool(c)
		case GetCephSecret:
			m.ExpectGetCephSecret(c)
		case GetCephSecretID:
			m.ExpectGetCephSecretID(c)
		case FindStoragePool:
			m.ExpectFindStoragePool(c)
		case NewStream:
			m.ExpectNewStream(c)
		case SetCephSecret:
			m.ExpectSetCephSecret(c)
		case UpdateStoragePool:
			m.ExpectUpdateStoragePool(c)
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

func (m *MockLibvirt) ExpectGetCephSecret(c GetCephSecret) *MockLibvirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.getCephSecret = append(m.getCephSecret, c)
	return m
}

func (m *MockLibvirt) ExpectGetCephSecretID(c GetCephSecretID) *MockLibvirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.getCephSecretId = append(m.getCephSecretId, c)
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

func (m *MockLibvirt) ExpectSetCephSecret(c SetCephSecret) *MockLibvirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.setCephSecret = append(m.setCephSecret, c)
	return m
}

func (m *MockLibvirt) ExpectUpdateStoragePool(c UpdateStoragePool) *MockLibvirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.updateStoragePool = append(m.updateStoragePool, c)
	return m
}

func (m *MockLibvirt) CreateStoragePool(desc *libvirtxml.StoragePool) (shims.StoragePool, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.createStoragePool,
		must.Sprint("Unexpected call to CreateStoragePool"))
	idx := max(slices.IndexFunc(m.createStoragePool, func(c CreateStoragePool) bool {
		return c.Desc.Name == desc.Name
	}), 0)

	call := m.createStoragePool[idx]
	m.createStoragePool = append(m.createStoragePool[:idx], m.createStoragePool[idx+1:]...)

	must.Eq(m.t, call.Desc, desc,
		must.Sprint("CreateStoragePool received incorrect argument"))

	return call.Result, call.Err
}

func (m *MockLibvirt) GetCephSecret(name string) (string, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getCephSecret,
		must.Sprint("Unexpected call to GetCephSecret"))
	call := m.getCephSecret[0]
	m.getCephSecret = m.getCephSecret[1:]

	must.Eq(m.t, call, GetCephSecret{Name: name, Result: call.Result, Err: call.Err},
		must.Sprint("GetCephSecret received incorrect argument"))

	return call.Result, call.Err
}

func (m *MockLibvirt) GetCephSecretID(name string) (string, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getCephSecretId,
		must.Sprint("Unexpected call to GetCephSecretID"))
	call := m.getCephSecretId[0]
	m.getCephSecretId = m.getCephSecretId[1:]

	must.Eq(m.t, call, GetCephSecretID{Name: name, Result: call.Result, Err: call.Err},
		must.Sprint("GetCephSecretID received incorrect argument"))

	return call.Result, call.Err
}

func (m *MockLibvirt) FindStoragePool(name string) (shims.StoragePool, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.findStoragePool,
		must.Sprint("Unexpected call to FindStoragePool"))

	idx := max(slices.IndexFunc(m.findStoragePool, func(c FindStoragePool) bool {
		return c.Name == name
	}), 0)

	call := m.findStoragePool[idx]
	m.findStoragePool = append(m.findStoragePool[:idx], m.findStoragePool[idx+1:]...)

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

func (m *MockLibvirt) SetCephSecret(name, credential string) (string, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.setCephSecret,
		must.Sprint("Unexpected call to SetCephSecret"))
	call := m.setCephSecret[0]
	m.setCephSecret = m.setCephSecret[1:]

	must.Eq(m.t, call, SetCephSecret{Name: name, Credential: credential, Result: call.Result, Err: call.Err},
		must.Sprint("SetCephSecret received incorrect argument"))

	return call.Result, call.Err
}

func (m *MockLibvirt) UpdateStoragePool(desc *libvirtxml.StoragePool) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.updateStoragePool,
		must.Sprint("Unexpected call to UpdateStoragePool"))
	call := m.updateStoragePool[0]
	m.updateStoragePool = m.updateStoragePool[1:]

	must.Eq(m.t, call, UpdateStoragePool{Desc: desc, Err: call.Err},
		must.Sprint("UpdateStoragePool received incorrect argument"))

	return call.Err
}

func (m *MockLibvirt) AssertExpectations() {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceEmpty(m.t, m.createStoragePool,
		must.Sprintf("CreateStoragePool expecting %d more invocations", len(m.createStoragePool)))
	must.SliceEmpty(m.t, m.getCephSecret,
		must.Sprintf("GetCephSecret expecting %d more invocations", len(m.getCephSecret)))
	must.SliceEmpty(m.t, m.getCephSecretId,
		must.Sprintf("GetCephSecretID expecting %d more invocations", len(m.getCephSecretId)))
	must.SliceEmpty(m.t, m.findStoragePool,
		must.Sprintf("FindStoragePool expecting %d more invocations", len(m.findStoragePool)))
	must.SliceEmpty(m.t, m.newStream,
		must.Sprintf("NewStream expecting %d more invocations", len(m.newStream)))
	must.SliceEmpty(m.t, m.setCephSecret,
		must.Sprintf("SetCephSecret expecting %d more invocations", len(m.setCephSecret)))
	must.SliceEmpty(m.t, m.updateStoragePool,
		must.Sprintf("UpdateStoragePool expecting %d more invocations", len(m.updateStoragePool)))
}

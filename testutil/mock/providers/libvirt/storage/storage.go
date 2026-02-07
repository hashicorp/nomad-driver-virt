// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
	"github.com/shoenig/test/must"
	"libvirt.org/go/libvirt"
)

func NewStaticStoragePool() *StaticStoragePool {
	return &StaticStoragePool{}
}

func NewMockStoragePool(t must.T) *MockStoragePool {
	return &MockStoragePool{t: t}
}

type StaticStoragePool struct {
	GetNameResult                string
	LookupStorageVolByNameResult shims.StorageVol
	StorageVolCreateByXMLResult  shims.StorageVol

	counts map[string]int
	m      sync.Mutex
	o      sync.Once
}

func (s *StaticStoragePool) incrCount() {
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

func (s *StaticStoragePool) CallCount(fnName string) int {
	s.m.Lock()
	defer s.m.Unlock()

	if s.counts == nil {
		return 0
	}

	return s.counts[fnName]
}

func (s *StaticStoragePool) Free() error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticStoragePool) GetName() (string, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.GetNameResult, nil
}

func (s *StaticStoragePool) LookupStorageVolByName(string) (shims.StorageVol, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.LookupStorageVolByNameResult == nil {
		s.LookupStorageVolByNameResult = &StaticStorageVol{}
	}

	return s.LookupStorageVolByNameResult, nil
}

func (s *StaticStoragePool) StorageVolCreateXML(string, libvirt.StorageVolCreateFlags) (shims.StorageVol, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.StorageVolCreateByXMLResult == nil {
		s.StorageVolCreateByXMLResult = &StaticStorageVol{}
	}

	return s.StorageVolCreateByXMLResult, nil
}

type Free struct {
	Err error
}

type GetName struct {
	Result string
	Err    error
}

type LookupStorageVolByName struct {
	Name   string
	Result shims.StorageVol
	Err    error
}

type StorageVolCreateXML struct {
	Desc   string
	Flags  libvirt.StorageVolCreateFlags
	Result shims.StorageVol
	Err    error
}

type MockStoragePool struct {
	t must.T

	free                   []Free
	getName                []GetName
	lookupStorageVolByName []LookupStorageVolByName
	storageVolCreateXml    []StorageVolCreateXML
	m                      sync.Mutex
}

func (m *MockStoragePool) Expect(calls ...any) *MockStoragePool {
	for _, call := range calls {
		switch c := call.(type) {
		case Free:
			m.ExpectFree(c)
		case GetName:
			m.ExpectGetName(c)
		case LookupStorageVolByName:
			m.ExpectLookupStorageVolByName(c)
		case StorageVolCreateXML:
			m.ExpectStorageVolCreateXML(c)
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockStoragePool) ExpectFree(c Free) *MockStoragePool {
	m.m.Lock()
	defer m.m.Unlock()

	m.free = append(m.free, c)
	return m
}

func (m *MockStoragePool) ExpectGetName(c GetName) *MockStoragePool {
	m.m.Lock()
	defer m.m.Unlock()

	m.getName = append(m.getName, c)
	return m
}

func (m *MockStoragePool) ExpectLookupStorageVolByName(c LookupStorageVolByName) *MockStoragePool {
	m.m.Lock()
	defer m.m.Unlock()

	m.lookupStorageVolByName = append(m.lookupStorageVolByName, c)
	return m
}

func (m *MockStoragePool) ExpectStorageVolCreateXML(c StorageVolCreateXML) *MockStoragePool {
	m.m.Lock()
	defer m.m.Unlock()

	m.storageVolCreateXml = append(m.storageVolCreateXml, c)
	return m
}

func (m *MockStoragePool) Free() error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.free,
		must.Sprint("unexpected call to Free"))
	call := m.free[0]
	m.free = m.free[1:]

	return call.Err
}

func (m *MockStoragePool) GetName() (string, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getName,
		must.Sprint("unexpected call to GetName"))
	call := m.getName[0]
	m.getName = m.getName[1:]

	return call.Result, call.Err
}

func (m *MockStoragePool) LookupStorageVolByName(name string) (shims.StorageVol, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.lookupStorageVolByName,
		must.Sprint("unexpected call to LookupStorageVolByName"))
	call := m.lookupStorageVolByName[0]
	m.lookupStorageVolByName = m.lookupStorageVolByName[1:]

	must.Eq(m.t, struct{ Name string }{call.Name}, struct{ Name string }{name},
		must.Sprint("LookupStorageVolByName received incorrect argument"))

	return call.Result, call.Err
}

func (m *MockStoragePool) StorageVolCreateXML(desc string, flags libvirt.StorageVolCreateFlags) (shims.StorageVol, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.storageVolCreateXml,
		must.Sprint("unexpected call to StorageVolCreateXML"))
	call := m.storageVolCreateXml[0]
	m.storageVolCreateXml = m.storageVolCreateXml[1:]

	must.Eq(m.t, call, StorageVolCreateXML{Desc: desc, Flags: flags, Result: call.Result, Err: call.Err},
		must.Sprint("StorageVolCreateXML received incorrect argument"))

	return call.Result, call.Err
}

func (m *MockStoragePool) AssertExpectations() {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceEmpty(m.t, m.free,
		must.Sprintf("Free expecting %d more invocations", len(m.free)))
	must.SliceEmpty(m.t, m.getName,
		must.Sprintf("GetName expecting %d more invocations", len(m.getName)))
	must.SliceEmpty(m.t, m.lookupStorageVolByName,
		must.Sprintf("LookupStorageVolByName expecting %d more invocations", len(m.lookupStorageVolByName)))
	must.SliceEmpty(m.t, m.storageVolCreateXml,
		must.Sprintf("StorageVolCreateXML expecting %d more invocations", len(m.storageVolCreateXml)))
}

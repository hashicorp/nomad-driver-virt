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
	GetNameResult                 string
	GetXMLDescResult              string
	IsActiveResult                bool
	LookupStorageVolByNameResult  shims.StorageVol
	StorageVolCreateXMLResult     shims.StorageVol
	StorageVolCreateXMLFromResult shims.StorageVol

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

func (s *StaticStoragePool) Create(libvirt.StoragePoolCreateFlags) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
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

func (s *StaticStoragePool) GetXMLDesc(libvirt.StorageXMLFlags) (string, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.GetXMLDescResult, nil
}

func (s *StaticStoragePool) IsActive() (bool, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.IsActiveResult, nil
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

func (s *StaticStoragePool) Refresh(uint32) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticStoragePool) SetAutostart(bool) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticStoragePool) StorageVolCreateXML(string, libvirt.StorageVolCreateFlags) (shims.StorageVol, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.StorageVolCreateXMLResult == nil {
		s.StorageVolCreateXMLResult = &StaticStorageVol{}
	}

	return s.StorageVolCreateXMLResult, nil
}

func (s *StaticStoragePool) StorageVolCreateXMLFrom(string, shims.StorageVol, libvirt.StorageVolCreateFlags) (shims.StorageVol, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.StorageVolCreateXMLFromResult == nil {
		s.StorageVolCreateXMLFromResult = &StaticStorageVol{}
	}

	return s.StorageVolCreateXMLFromResult, nil
}

type Create struct {
	Flags libvirt.StoragePoolCreateFlags
	Err   error
}

type Free struct {
	Err error
}

type GetName struct {
	Result string
	Err    error
}

type GetXMLDesc struct {
	Flags  libvirt.StorageXMLFlags
	Result string
	Err    error
}

type IsActive struct {
	Result bool
	Err    error
}

type Refresh struct {
	Flags uint32
	Err   error
}

type SetAutostart struct {
	Enable bool
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

type StorageVolCreateXMLFrom struct {
	Desc     string
	CloneVol shims.StorageVol
	Flags    libvirt.StorageVolCreateFlags
	Result   shims.StorageVol
	Err      error
}

type MockStoragePool struct {
	t must.T

	create                  []Create
	free                    []Free
	getName                 []GetName
	getXmlDesc              []GetXMLDesc
	isActive                []IsActive
	refresh                 []Refresh
	setAutostart            []SetAutostart
	lookupStorageVolByName  []LookupStorageVolByName
	storageVolCreateXml     []StorageVolCreateXML
	storageVolCreateXmlFrom []StorageVolCreateXMLFrom

	m sync.Mutex
}

func (m *MockStoragePool) Expect(calls ...any) *MockStoragePool {
	for _, call := range calls {
		switch c := call.(type) {
		case Create:
			m.ExpectCreate(c)
		case Free:
			m.ExpectFree(c)
		case GetXMLDesc:
			m.ExpectGetXMLDesc(c)
		case GetName:
			m.ExpectGetName(c)
		case IsActive:
			m.ExpectIsActive(c)
		case LookupStorageVolByName:
			m.ExpectLookupStorageVolByName(c)
		case Refresh:
			m.ExpectRefresh(c)
		case SetAutostart:
			m.ExpectSetAutostart(c)
		case StorageVolCreateXML:
			m.ExpectStorageVolCreateXML(c)
		case StorageVolCreateXMLFrom:
			m.ExpectStorageVolCreateXMLFrom(c)
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockStoragePool) ExpectCreate(c Create) *MockStoragePool {
	m.m.Lock()
	defer m.m.Unlock()

	m.create = append(m.create, c)
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

func (m *MockStoragePool) ExpectGetXMLDesc(c GetXMLDesc) *MockStoragePool {
	m.m.Lock()
	defer m.m.Unlock()

	m.getXmlDesc = append(m.getXmlDesc, c)
	return m
}

func (m *MockStoragePool) ExpectIsActive(c IsActive) *MockStoragePool {
	m.m.Lock()
	defer m.m.Unlock()

	m.isActive = append(m.isActive, c)
	return m
}

func (m *MockStoragePool) ExpectLookupStorageVolByName(c LookupStorageVolByName) *MockStoragePool {
	m.m.Lock()
	defer m.m.Unlock()

	m.lookupStorageVolByName = append(m.lookupStorageVolByName, c)
	return m
}

func (m *MockStoragePool) ExpectRefresh(c Refresh) *MockStoragePool {
	m.m.Lock()
	defer m.m.Unlock()

	m.refresh = append(m.refresh, c)
	return m
}

func (m *MockStoragePool) ExpectSetAutostart(c SetAutostart) *MockStoragePool {
	m.m.Lock()
	defer m.m.Unlock()

	m.setAutostart = append(m.setAutostart, c)
	return m
}

func (m *MockStoragePool) ExpectStorageVolCreateXML(c StorageVolCreateXML) *MockStoragePool {
	m.m.Lock()
	defer m.m.Unlock()

	m.storageVolCreateXml = append(m.storageVolCreateXml, c)
	return m
}

func (m *MockStoragePool) ExpectStorageVolCreateXMLFrom(c StorageVolCreateXMLFrom) *MockStoragePool {
	m.m.Lock()
	defer m.m.Unlock()

	m.storageVolCreateXmlFrom = append(m.storageVolCreateXmlFrom, c)
	return m
}

func (m *MockStoragePool) Create(flags libvirt.StoragePoolCreateFlags) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.create,
		must.Sprint("unexpected call to Create"))
	call := m.create[0]
	m.create = m.create[1:]

	must.Eq(m.t, call, Create{Flags: flags, Err: call.Err},
		must.Sprint("Create received incorrect argument"))

	return call.Err
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

func (m *MockStoragePool) GetXMLDesc(flags libvirt.StorageXMLFlags) (string, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getXmlDesc,
		must.Sprint("unexpected call to GetXMLDesc"))
	call := m.getXmlDesc[0]
	m.getXmlDesc = m.getXmlDesc[1:]

	must.Eq(m.t, call, GetXMLDesc{Flags: flags, Result: call.Result, Err: call.Err},
		must.Sprint("GetXMLDesc received incorrect argument"))

	return call.Result, call.Err
}

func (m *MockStoragePool) IsActive() (bool, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.isActive,
		must.Sprint("unexpected call to IsActive"))
	call := m.isActive[0]
	m.isActive = m.isActive[1:]

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

func (m *MockStoragePool) Refresh(uint32) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.refresh,
		must.Sprint("unexpected call to Refresh"))
	call := m.refresh[0]
	m.refresh = m.refresh[1:]

	return call.Err
}

func (m *MockStoragePool) SetAutostart(enable bool) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.setAutostart,
		must.Sprint("unexpected call to SetAutostart"))
	call := m.setAutostart[0]
	m.setAutostart = m.setAutostart[1:]

	must.Eq(m.t, call, SetAutostart{Enable: enable, Err: call.Err},
		must.Sprint("SetAutostart received incorrect argument"))

	return call.Err
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

func (m *MockStoragePool) StorageVolCreateXMLFrom(desc string, cloneVol shims.StorageVol, flags libvirt.StorageVolCreateFlags) (shims.StorageVol, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.storageVolCreateXmlFrom,
		must.Sprint("unexpected call to StorageVolCreateXMLFrom"))
	call := m.storageVolCreateXmlFrom[0]
	m.storageVolCreateXmlFrom = m.storageVolCreateXmlFrom[1:]

	must.Eq(m.t, call, StorageVolCreateXMLFrom{Desc: desc, CloneVol: cloneVol, Flags: flags, Result: call.Result, Err: call.Err},
		must.Sprint("StorageVolCreateXMLFrom received incorrect argument"))

	return call.Result, call.Err
}

func (m *MockStoragePool) AssertExpectations() {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceEmpty(m.t, m.create,
		must.Sprintf("Create expecting %d more invocations", len(m.create)))
	must.SliceEmpty(m.t, m.free,
		must.Sprintf("Free expecting %d more invocations", len(m.free)))
	must.SliceEmpty(m.t, m.getName,
		must.Sprintf("GetName expecting %d more invocations", len(m.getName)))
	must.SliceEmpty(m.t, m.getXmlDesc,
		must.Sprintf("GetXMLDesc expecting %d more invocations", len(m.getXmlDesc)))
	must.SliceEmpty(m.t, m.isActive,
		must.Sprintf("IsActive expecting %d more invocations", len(m.isActive)))
	must.SliceEmpty(m.t, m.refresh,
		must.Sprintf("Refresh expecting %d more invocations", len(m.refresh)))
	must.SliceEmpty(m.t, m.setAutostart,
		must.Sprintf("SetAutostart expecting %d more invocations", len(m.setAutostart)))
	must.SliceEmpty(m.t, m.lookupStorageVolByName,
		must.Sprintf("LookupStorageVolByName expecting %d more invocations", len(m.lookupStorageVolByName)))
	must.SliceEmpty(m.t, m.storageVolCreateXml,
		must.Sprintf("StorageVolCreateXML expecting %d more invocations", len(m.storageVolCreateXml)))
	must.SliceEmpty(m.t, m.storageVolCreateXmlFrom,
		must.Sprintf("StorageVolCreateXMLFrom expecting %d more invocations", len(m.storageVolCreateXmlFrom)))
}

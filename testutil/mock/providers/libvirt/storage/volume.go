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
	"libvirt.org/go/libvirtxml"
)

func NewStaticStorageVol() *StaticStorageVol {
	return &StaticStorageVol{}
}

func NewMockStorageVol(t must.T) *MockStorageVol {
	return &MockStorageVol{t: t}
}

type StaticStorageVol struct {
	GetInfoResult            *libvirt.StorageVolInfo
	GetPathResult            string
	GetXMLDescResult         string
	LookupPoolByVolumeResult shims.StoragePool

	counts map[string]int
	m      sync.Mutex
	o      sync.Once
}

func (s *StaticStorageVol) incrCount() {
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

func (s *StaticStorageVol) CallCount(fnName string) int {
	s.m.Lock()
	defer s.m.Unlock()

	if s.counts == nil {
		return 0
	}

	return s.counts[fnName]
}

func (s *StaticStorageVol) LookupPoolByVolume() (shims.StoragePool, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.LookupPoolByVolumeResult != nil {
		return s.LookupPoolByVolumeResult, nil
	}

	s.LookupPoolByVolumeResult = &StaticStoragePool{}
	return s.LookupPoolByVolumeResult, nil
}

func (s *StaticStorageVol) Delete(libvirt.StorageVolDeleteFlags) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticStorageVol) Free() error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticStorageVol) GetInfo() (*libvirt.StorageVolInfo, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.GetInfoResult != nil {
		return s.GetInfoResult, nil
	}

	return &libvirt.StorageVolInfo{}, nil
}

func (s *StaticStorageVol) GetPath() (string, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.GetPathResult, nil
}

func (s *StaticStorageVol) GetXMLDesc(uint32) (string, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.GetXMLDescResult == "" {
		desc := &libvirtxml.StorageVolume{}
		return desc.Marshal()
	}
	return s.GetXMLDescResult, nil
}

func (s *StaticStorageVol) Resize(uint64, libvirt.StorageVolResizeFlags) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticStorageVol) Upload(shims.Stream, uint64, uint64, libvirt.StorageVolUploadFlags) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

type Delete struct {
	Flags libvirt.StorageVolDeleteFlags
	Err   error
}

type GetInfo struct {
	Result *libvirt.StorageVolInfo
	Err    error
}

type GetPath struct {
	Result string
	Err    error
}

type LookupPoolByVolume struct {
	Result shims.StoragePool
	Err    error
}

type Resize struct {
	Size  uint64
	Flags libvirt.StorageVolResizeFlags
	Err   error
}

type Upload struct {
	Stream shims.Stream
	Offset uint64
	Size   uint64
	Flags  libvirt.StorageVolUploadFlags
	Err    error
}

type MockStorageVol struct {
	t must.T

	delete             []Delete
	free               []Free
	getInfo            []GetInfo
	getPath            []GetPath
	getXmlDesc         []GetXMLDesc
	lookupPoolByVolume []LookupPoolByVolume
	resize             []Resize
	upload             []Upload
	m                  sync.Mutex
}

func (m *MockStorageVol) Expect(calls ...any) *MockStorageVol {
	for _, call := range calls {
		switch c := call.(type) {
		case Delete:
			m.ExpectDelete(c)
		case Free:
			m.ExpectFree(c)
		case GetInfo:
			m.ExpectGetInfo(c)
		case GetPath:
			m.ExpectGetPath(c)
		case GetXMLDesc:
			m.ExpectGetXMLDesc(c)
		case LookupPoolByVolume:
			m.ExpectLookupPoolByVolume(c)
		case Resize:
			m.ExpectResize(c)
		case Upload:
			m.ExpectUpload(c)
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockStorageVol) ExpectDelete(c Delete) *MockStorageVol {
	m.m.Lock()
	defer m.m.Unlock()

	m.delete = append(m.delete, c)
	return m
}

func (m *MockStorageVol) ExpectFree(c Free) *MockStorageVol {
	m.m.Lock()
	defer m.m.Unlock()

	m.free = append(m.free, c)
	return m
}

func (m *MockStorageVol) ExpectGetInfo(c GetInfo) *MockStorageVol {
	m.m.Lock()
	defer m.m.Unlock()

	m.getInfo = append(m.getInfo, c)
	return m
}

func (m *MockStorageVol) ExpectGetPath(c GetPath) *MockStorageVol {
	m.m.Lock()
	defer m.m.Unlock()

	m.getPath = append(m.getPath, c)
	return m
}

func (m *MockStorageVol) ExpectGetXMLDesc(c GetXMLDesc) *MockStorageVol {
	m.m.Lock()
	defer m.m.Unlock()

	m.getXmlDesc = append(m.getXmlDesc, c)
	return m
}

func (m *MockStorageVol) ExpectLookupPoolByVolume(c LookupPoolByVolume) *MockStorageVol {
	m.m.Lock()
	defer m.m.Unlock()

	m.lookupPoolByVolume = append(m.lookupPoolByVolume, c)
	return m
}

func (m *MockStorageVol) ExpectResize(c Resize) *MockStorageVol {
	m.m.Lock()
	defer m.m.Unlock()

	m.resize = append(m.resize, c)
	return m
}

func (m *MockStorageVol) ExpectUpload(c Upload) *MockStorageVol {
	m.m.Lock()
	defer m.m.Unlock()

	m.upload = append(m.upload, c)
	return m
}

func (m *MockStorageVol) Delete(flags libvirt.StorageVolDeleteFlags) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.delete,
		must.Sprint("unexpected call to Delete"))
	call := m.delete[0]
	m.delete = m.delete[1:]

	must.Eq(m.t, call.Flags, flags,
		must.Sprint("Delete received incorrect argument"))

	return call.Err
}

func (m *MockStorageVol) Free() error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.free,
		must.Sprint("unexpected call to Free"))
	call := m.free[0]
	m.free = m.free[1:]

	return call.Err
}

func (m *MockStorageVol) GetInfo() (*libvirt.StorageVolInfo, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getInfo,
		must.Sprint("unexpected call to GetInfo"))
	call := m.getInfo[0]
	m.getInfo = m.getInfo[1:]

	return call.Result, call.Err
}

func (m *MockStorageVol) GetPath() (string, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getPath,
		must.Sprint("unexpected call to GetPath"))
	call := m.getPath[0]
	m.getPath = m.getPath[1:]

	return call.Result, call.Err
}

func (m *MockStorageVol) GetXMLDesc(_ uint32) (string, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getXmlDesc,
		must.Sprint("unexpected call to GetXMLDesc"))
	call := m.getXmlDesc[0]
	m.getXmlDesc = m.getXmlDesc[1:]

	return call.Result, call.Err
}

func (m *MockStorageVol) LookupPoolByVolume() (shims.StoragePool, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.lookupPoolByVolume,
		must.Sprint("unexpected call to LookupPoolByVolume"))
	call := m.lookupPoolByVolume[0]
	m.lookupPoolByVolume = m.lookupPoolByVolume[1:]

	return call.Result, call.Err
}

func (m *MockStorageVol) Resize(size uint64, flags libvirt.StorageVolResizeFlags) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.resize,
		must.Sprint("unexpected call to Resize"))
	call := m.resize[0]
	m.resize = m.resize[1:]

	must.Eq(m.t, call, Resize{Size: size, Flags: flags, Err: call.Err},
		must.Sprint("Resize received incorrect argument"))

	return call.Err
}

func (m *MockStorageVol) Upload(stream shims.Stream, offset uint64, size uint64, flags libvirt.StorageVolUploadFlags) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.upload,
		must.Sprint("unexpected call to Upload"))
	call := m.upload[0]
	m.upload = m.upload[1:]

	must.Eq(m.t, call, Upload{Stream: stream, Offset: offset, Size: size, Flags: flags, Err: call.Err},
		must.Sprint("Upload received incorrect argument"))

	return call.Err
}

func (m *MockStorageVol) AssertExpectations() {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceEmpty(m.t, m.delete,
		must.Sprintf("Delete expecting %d more invocations", len(m.delete)))
	must.SliceEmpty(m.t, m.free,
		must.Sprintf("Free expecting %d more invocations", len(m.free)))
	must.SliceEmpty(m.t, m.getInfo,
		must.Sprintf("GetInfo expecting %d more invocations", len(m.getInfo)))
	must.SliceEmpty(m.t, m.getPath,
		must.Sprintf("GetPath expecting %d more invocations", len(m.getPath)))
	must.SliceEmpty(m.t, m.getXmlDesc,
		must.Sprintf("GetXMLDesc expecting %d more invocations", len(m.getXmlDesc)))
	must.SliceEmpty(m.t, m.lookupPoolByVolume,
		must.Sprintf("LookupPoolByVolume expecting %d more invocations", len(m.lookupPoolByVolume)))
	must.SliceEmpty(m.t, m.resize,
		must.Sprintf("Resize expecting %d more invocations", len(m.resize)))
	must.SliceEmpty(m.t, m.upload,
		must.Sprintf("Upload expecting %d more invocations", len(m.upload)))
}

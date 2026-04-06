// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/nomad-driver-virt/storage"
	"github.com/shoenig/test/must"
)

func NewMockPool(t must.T) *MockPool {
	return &MockPool{t: t}
}

func NewStaticPool() *StaticPool {
	return &StaticPool{}
}

type StaticPool struct {
	AddVolumeResult          *storage.Volume
	GetVolumeResult          *storage.Volume
	NameResult               string
	TypeResult               string
	DefaultImageFormatResult string
	ListVolumesResult        []string

	counts map[string]int
	m      sync.Mutex
	o      sync.Once
}

func (s *StaticPool) incrCount() {
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

func (s *StaticPool) CallCount(fnName string) int {
	s.m.Lock()
	defer s.m.Unlock()

	if s.counts == nil {
		return 0
	}

	return s.counts[fnName]
}

func (s *StaticPool) ListVolumes() ([]string, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.ListVolumesResult != nil {
		return s.ListVolumesResult, nil
	}

	return make([]string, 0), nil
}

func (s *StaticPool) AddVolume(string, storage.Options) (*storage.Volume, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.AddVolumeResult == nil {
		s.AddVolumeResult = &storage.Volume{}
	}

	return s.AddVolumeResult, nil
}

func (s *StaticPool) GetVolume(string) (*storage.Volume, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.GetVolumeResult == nil {
		s.GetVolumeResult = &storage.Volume{}
	}

	return s.GetVolumeResult, nil
}

func (s *StaticPool) DeleteVolume(string) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticPool) Name() string {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.NameResult
}

func (s *StaticPool) Type() string {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.TypeResult
}

func (s *StaticPool) DefaultImageFormat() string {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.DefaultImageFormatResult != "" {
		return s.DefaultImageFormatResult
	}

	return "testing-image-format"
}

type AddVolume struct {
	Name   string
	Opts   storage.Options
	Result *storage.Volume
	Err    error
}

type DefaultImageFormat struct {
	Result string
	Err    error
}

type GetVolume struct {
	Name   string
	Result *storage.Volume
	Err    error
}

type DeleteVolume struct {
	Name string
	Err  error
}

type Name struct {
	Result string
}

type Type struct {
	Result string
}

type ListVolumes struct {
	Result []string
	Err    error
}

type MockPool struct {
	t must.T

	addVolume          []AddVolume
	getVolume          []GetVolume
	defaultImageFormat []DefaultImageFormat
	deleteVolume       []DeleteVolume
	name               []Name
	types              []Type
	listVolumes        []ListVolumes
	m                  sync.Mutex
}

func (m *MockPool) Expect(calls ...any) *MockPool {
	for _, call := range calls {
		switch c := call.(type) {
		case AddVolume:
			m.ExpectAddVolume(c)
		case DefaultImageFormat:
			m.ExpectDefaultImageFormat(c)
		case GetVolume:
			m.ExpectGetVolume(c)
		case DeleteVolume:
			m.ExpectDeleteVolume(c)
		case Name:
			m.ExpectName(c)
		case Type:
			m.ExpectType(c)
		case ListVolumes:
			m.ExpectListVolumes(c)
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockPool) ExpectListVolumes(c ListVolumes) *MockPool {
	m.m.Lock()
	defer m.m.Unlock()

	m.listVolumes = append(m.listVolumes, c)
	return m
}

func (m *MockPool) ExpectType(c Type) *MockPool {
	m.m.Lock()
	defer m.m.Unlock()

	m.types = append(m.types, c)
	return m
}

func (m *MockPool) ExpectAddVolume(c AddVolume) *MockPool {
	m.m.Lock()
	defer m.m.Unlock()

	m.addVolume = append(m.addVolume, c)
	return m
}

func (m *MockPool) ExpectDefaultImageFormat(c DefaultImageFormat) *MockPool {
	m.m.Lock()
	defer m.m.Unlock()

	m.defaultImageFormat = append(m.defaultImageFormat, c)
	return m
}

func (m *MockPool) ExpectGetVolume(c GetVolume) *MockPool {
	m.m.Lock()
	defer m.m.Unlock()

	m.getVolume = append(m.getVolume, c)
	return m
}

func (m *MockPool) ExpectDeleteVolume(c DeleteVolume) *MockPool {
	m.m.Lock()
	defer m.m.Unlock()

	m.deleteVolume = append(m.deleteVolume, c)
	return m
}

func (m *MockPool) ExpectName(c Name) *MockPool {
	m.m.Lock()
	defer m.m.Unlock()

	m.name = append(m.name, c)
	return m
}

func (m *MockPool) ListVolumes() ([]string, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.listVolumes,
		must.Sprint("Unexpected call to ListVolumes"))
	call := m.listVolumes[0]
	m.listVolumes = m.listVolumes[1:]

	return call.Result, call.Err
}

func (m *MockPool) AddVolume(name string, opts storage.Options) (*storage.Volume, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.addVolume,
		must.Sprint("Unexpected call to AddVolume"))
	call := m.addVolume[0]
	m.addVolume = m.addVolume[1:]

	must.Eq(m.t,
		struct {
			Name string
			Opts storage.Options
		}{call.Name, call.Opts},
		struct {
			Name string
			Opts storage.Options
		}{name, opts},
		must.Sprint("AddVolume received incorrect arguments"),
	)

	return call.Result, call.Err
}

func (m *MockPool) GetVolume(name string) (*storage.Volume, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getVolume,
		must.Sprint("Unexpected call to GetVolume"))
	call := m.getVolume[0]
	m.getVolume = m.getVolume[1:]

	must.Eq(m.t,
		struct{ Name string }{call.Name},
		struct{ Name string }{name},
		must.Sprint("GetVolume received incorrect arguments"),
	)

	return call.Result, call.Err
}

func (m *MockPool) DeleteVolume(name string) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.deleteVolume,
		must.Sprint("Unexpected call to DeleteVolume"))
	call := m.deleteVolume[0]
	m.deleteVolume = m.deleteVolume[1:]

	must.Eq(m.t,
		struct{ Name string }{call.Name},
		struct{ Name string }{name},
		must.Sprint("DeleteVolume received incorrect arguments"),
	)

	return call.Err
}

func (m *MockPool) Name() string {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.name,
		must.Sprint("Unexpected call to Name"))
	call := m.name[0]
	m.name = m.name[1:]

	return call.Result
}

func (m *MockPool) DefaultImageFormat() string {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.defaultImageFormat,
		must.Sprint("Unexpected call to DefaultImageFormat"))
	call := m.defaultImageFormat[0]
	m.defaultImageFormat = m.defaultImageFormat[1:]

	return call.Result
}

func (m *MockPool) Type() string {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.types,
		must.Sprint("Unexpected call to Type"))
	call := m.types[0]
	m.types = m.types[1:]

	return call.Result
}

// AssertExpectations verifies that all expected invocations
// have been called.
func (m *MockPool) AssertExpectations() {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceEmpty(m.t, m.addVolume,
		must.Sprintf("AddVolume expecting %d more invocations", len(m.addVolume)))
	must.SliceEmpty(m.t, m.getVolume,
		must.Sprintf("GetVolume expecting %d more invocations", len(m.getVolume)))
	must.SliceEmpty(m.t, m.deleteVolume,
		must.Sprintf("DeleteVolume expecting %d more invocations", len(m.deleteVolume)))
	must.SliceEmpty(m.t, m.name,
		must.Sprintf("Name expecting %d more invocations", len(m.name)))
	must.SliceEmpty(m.t, m.defaultImageFormat,
		must.Sprintf("DefaultImageFormat expecting %d more invocations", len(m.defaultImageFormat)))
	must.SliceEmpty(m.t, m.types,
		must.Sprintf("Type expecting %d more invocations", len(m.types)))
	must.SliceEmpty(m.t, m.listVolumes,
		must.Sprintf("ListVolumes expecting %d more invocations", len(m.listVolumes)))
}

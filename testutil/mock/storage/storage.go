// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/nomad-driver-virt/storage"
	"github.com/hashicorp/nomad-driver-virt/storage/image_tools"
	mock_image_tools "github.com/hashicorp/nomad-driver-virt/testutil/mock/storage/image_tools"
	"github.com/shoenig/test/must"
)

func NewStaticStorage() *StaticStorage {
	return &StaticStorage{}
}

func NewMockStorage(t must.T) *MockStorage {
	return &MockStorage{t: t}
}

type StaticStorage struct {
	DefaultPoolResult        storage.Pool
	GetPoolResult            storage.Pool
	ImageHandlerResult       image_tools.ImageHandler
	DefaultDiskDriverResult  string
	GenerateDeviceNameResult string
	counts                   map[string]int
	m                        sync.Mutex
	o                        sync.Once
}

func (s *StaticStorage) incrCount() {
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

func (s *StaticStorage) CallCount(fnName string) int {
	s.m.Lock()
	defer s.m.Unlock()

	if s.counts == nil {
		return 0
	}

	return s.counts[fnName]
}

func (s *StaticStorage) DefaultPool() (storage.Pool, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.DefaultPoolResult == nil {
		s.DefaultPoolResult = NewStaticPool()
	}

	return s.DefaultPoolResult, nil
}

func (s *StaticStorage) GetPool(string) (storage.Pool, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.GetPoolResult == nil {
		s.GetPoolResult = NewStaticPool()
	}

	return s.GetPoolResult, nil
}

func (s *StaticStorage) ImageHandler() image_tools.ImageHandler {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.ImageHandlerResult == nil {
		s.ImageHandlerResult = mock_image_tools.NewStaticImageHandler()
	}

	return s.ImageHandlerResult
}

func (s *StaticStorage) DefaultDiskDriver() string {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.DefaultDiskDriverResult
}

func (s *StaticStorage) GenerateDeviceName(string, []string) string {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.GenerateDeviceNameResult
}

type DefaultPool struct {
	Result storage.Pool
	Err    error
}

type GetPool struct {
	Name   string
	Result storage.Pool
	Err    error
}

type ImageHandler struct {
	Result image_tools.ImageHandler
}

type DefaultDiskDriver struct {
	Result string
}

type GenerateDeviceName struct {
	DiskType        string
	ExistingDevices []string
	Result          string
}

type MockStorage struct {
	t must.T

	defaultPool        []DefaultPool
	getPool            []GetPool
	imageHandler       []ImageHandler
	defaultDiskDriver  []DefaultDiskDriver
	generateDeviceName []GenerateDeviceName
	m                  sync.Mutex
}

func (m *MockStorage) Expect(calls ...any) *MockStorage {
	for _, call := range calls {
		switch c := call.(type) {
		case GetPool:
			m.ExpectGetPool(c)
		case ImageHandler:
			m.ExpectImageHandler(c)
		case DefaultDiskDriver:
			m.ExpectDefaultDiskDriver(c)
		case GenerateDeviceName:
			m.ExpectGenerateDeviceName(c)
		case DefaultPool:
			m.ExpectDefaultPool(c)
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockStorage) ExpectDefaultPool(c DefaultPool) *MockStorage {
	m.m.Lock()
	defer m.m.Unlock()

	m.defaultPool = append(m.defaultPool, c)
	return m
}

func (m *MockStorage) ExpectGetPool(c GetPool) *MockStorage {
	m.m.Lock()
	defer m.m.Unlock()

	m.getPool = append(m.getPool, c)
	return m
}

func (m *MockStorage) ExpectImageHandler(c ImageHandler) *MockStorage {
	m.m.Lock()
	defer m.m.Unlock()

	m.imageHandler = append(m.imageHandler, c)
	return m
}

func (m *MockStorage) ExpectDefaultDiskDriver(c DefaultDiskDriver) *MockStorage {
	m.m.Lock()
	defer m.m.Unlock()

	m.defaultDiskDriver = append(m.defaultDiskDriver, c)
	return m
}

func (m *MockStorage) ExpectGenerateDeviceName(c GenerateDeviceName) *MockStorage {
	m.m.Lock()
	defer m.m.Unlock()

	m.generateDeviceName = append(m.generateDeviceName, c)
	return m
}

func (m *MockStorage) DefaultPool() (storage.Pool, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.defaultPool,
		must.Sprint("Unexpected call to DefaultPool"))
	call := m.defaultPool[0]
	m.defaultPool = m.defaultPool[1:]

	return call.Result, nil
}

func (m *MockStorage) GetPool(name string) (storage.Pool, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getPool,
		must.Sprint("Unexpected call to GetPool"))
	call := m.getPool[0]
	m.getPool = m.getPool[1:]

	must.Eq(m.t,
		struct{ Name string }{call.Name},
		struct{ Name string }{name},
		must.Sprint("GetPool received incorrect arguments"),
	)

	return call.Result, call.Err
}

func (m *MockStorage) ImageHandler() image_tools.ImageHandler {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.imageHandler,
		must.Sprint("Unexpected call to ImageHandler"))
	call := m.imageHandler[0]
	m.imageHandler = m.imageHandler[1:]

	return call.Result
}

func (m *MockStorage) DefaultDiskDriver() string {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.defaultDiskDriver,
		must.Sprint("Unexpected call to DefaultDiskDriver"))
	call := m.defaultDiskDriver[0]
	m.defaultDiskDriver = m.defaultDiskDriver[1:]

	return call.Result
}

func (m *MockStorage) GenerateDeviceName(diskType string, existingDevices []string) string {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.generateDeviceName,
		must.Sprint("Unexpected call to GenerateDeviceName"))
	call := m.generateDeviceName[0]
	m.generateDeviceName = m.generateDeviceName[1:]

	must.Eq(m.t,
		struct {
			DiskType        string
			ExistingDevices []string
		}{call.DiskType, call.ExistingDevices},
		struct {
			DiskType        string
			ExistingDevices []string
		}{diskType, existingDevices},
		must.Sprint("GeneratedDeviceName received incorrect arguments"),
	)

	return call.Result
}

// AssertExpectations verifies that all expected invocations
// have been called.
func (m *MockStorage) AssertExpectations() {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceEmpty(m.t, m.defaultPool,
		must.Sprintf("DefaultPool expecting %d more invocations", len(m.defaultPool)))
	must.SliceEmpty(m.t, m.getPool,
		must.Sprintf("GetPool expecting %d more invocations", len(m.getPool)))
	must.SliceEmpty(m.t, m.imageHandler,
		must.Sprintf("ImageHandler expecting %d more invocations", len(m.imageHandler)))
	must.SliceEmpty(m.t, m.defaultDiskDriver,
		must.Sprintf("DefaultDiskDriver expecting %d more invocations", len(m.defaultDiskDriver)))
	must.SliceEmpty(m.t, m.generateDeviceName,
		must.Sprintf("GenerateDeviceName expecting %d more invocations", len(m.generateDeviceName)))
}

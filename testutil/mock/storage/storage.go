// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"maps"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/hashicorp/nomad-driver-virt/storage"
	"github.com/hashicorp/nomad-driver-virt/storage/image_tools"
	mock_image_tools "github.com/hashicorp/nomad-driver-virt/testutil/mock/storage/image_tools"
	"github.com/hashicorp/nomad/plugins/shared/structs"
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
	FingerprintResult        map[string]*structs.Attribute
	ListPoolsResult          []string
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

func (s *StaticStorage) ListPools() []string {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.ListPoolsResult != nil {
		return s.ListPoolsResult
	}

	return make([]string, 0)
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

func (s *StaticStorage) Fingerprint(attrs map[string]*structs.Attribute) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.FingerprintResult == nil {
		return
	}

	maps.Copy(attrs, s.FingerprintResult)
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

type ListPools struct {
	Result []string
}

type ImageHandler struct {
	Result image_tools.ImageHandler
}

type DefaultDiskDriver struct {
	Result string
}

type GenerateDeviceName struct {
	BusType         string
	ExistingDevices []string
	Result          string
}

type Fingerprint struct {
	Attrs   map[string]*structs.Attribute       // Expected attribute to receive (nil value prevents check)
	AttrsFn func(map[string]*structs.Attribute) // Allows for modification
}

type MockStorage struct {
	t must.T

	defaultPool        []DefaultPool
	getPool            []GetPool
	imageHandler       []ImageHandler
	defaultDiskDriver  []DefaultDiskDriver
	generateDeviceName []GenerateDeviceName
	fingerprint        []Fingerprint
	listPools          []ListPools
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
		case Fingerprint:
			m.ExpectFingerprint(c)
		case ListPools:
			m.ExpectListPools(c)
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockStorage) ExpectFingerprint(c Fingerprint) *MockStorage {
	m.m.Lock()
	defer m.m.Unlock()

	m.fingerprint = append(m.fingerprint, c)
	return m
}

func (m *MockStorage) ExpectListPools(c ListPools) *MockStorage {
	m.m.Lock()
	defer m.m.Unlock()

	m.listPools = append(m.listPools, c)
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

func (m *MockStorage) Fingerprint(attrs map[string]*structs.Attribute) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.fingerprint,
		must.Sprint("Unexpected call to Fingerprint"))
	call := m.fingerprint[0]
	m.fingerprint = m.fingerprint[1:]

	expectedKeys := slices.Sorted(maps.Keys(call.Attrs))
	actualKeys := slices.Sorted(maps.Keys(attrs))

	must.SliceContainsAll(m.t, expectedKeys, actualKeys,
		must.Sprint("Fingerprint received incorrect argument (map keys do not match)"))

	if call.Attrs != nil {
		for expectedKey, expectedValue := range call.Attrs {
			val, ok := attrs[expectedKey]
			if !ok {
				m.t.Fatalf("Fingerprint unexpected comparision error (missing value for key %s)", expectedKey)
			}

			if _, ok = expectedValue.Compare(val); !ok {
				m.t.Fatalf("Fingerprint received incorrect argument - key: %s - %v != %v", expectedKey, expectedValue, val)
			}
		}
	}

	if call.AttrsFn != nil {
		call.AttrsFn(attrs)
	}
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

func (m *MockStorage) ListPools() []string {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.listPools,
		must.Sprint("Unexpected call to ListPools"))
	call := m.listPools[0]
	m.listPools = m.listPools[1:]

	return call.Result
}

func (m *MockStorage) GenerateDeviceName(busType string, existingDevices []string) string {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.generateDeviceName,
		must.Sprint("Unexpected call to GenerateDeviceName"))
	call := m.generateDeviceName[0]
	m.generateDeviceName = m.generateDeviceName[1:]

	must.Eq(m.t,
		struct {
			BusType         string
			ExistingDevices []string
		}{call.BusType, call.ExistingDevices},
		struct {
			BusType         string
			ExistingDevices []string
		}{busType, existingDevices},
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
	must.SliceEmpty(m.t, m.fingerprint,
		must.Sprintf("Fingerprint expecting %d more invocations", len(m.fingerprint)))
	must.SliceEmpty(m.t, m.listPools,
		must.Sprintf("ListPools expecting %d more invocations", len(m.listPools)))
}

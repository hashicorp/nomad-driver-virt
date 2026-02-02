// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"github.com/hashicorp/nomad-driver-virt/storage"
	"github.com/hashicorp/nomad-driver-virt/storage/image_tools"
	mock_image_tools "github.com/hashicorp/nomad-driver-virt/testutil/mock/storage/image_tools"
	"github.com/shoenig/test/must"
)

type StaticStorage struct {
	GetPoolResult            storage.Pool
	ImageHandlerResult       image_tools.ImageHandler
	DefaultDiskDriverResult  string
	GenerateDeviceNameResult string
}

func (s *StaticStorage) GetPool(string) (storage.Pool, error) {
	if s.GetPoolResult != nil {
		return s.GetPoolResult, nil
	}

	return NewStaticPool(), nil
}

func (s *StaticStorage) ImageHandler() image_tools.ImageHandler {
	if s.ImageHandlerResult != nil {
		return s.ImageHandlerResult
	}

	return mock_image_tools.NewStaticImageHandler()
}

func (s *StaticStorage) DefaultDiskDriver() string {
	return s.DefaultDiskDriverResult
}

func (s *StaticStorage) GenerateDeviceName(string, []string) string {
	return s.GenerateDeviceNameResult
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

	getPool            []GetPool
	imageHandler       []ImageHandler
	defaultDiskDriver  []DefaultDiskDriver
	generateDeviceName []GenerateDeviceName
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
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockStorage) ExpectGetPool(c GetPool) *MockStorage {
	m.getPool = append(m.getPool, c)
	return m
}

func (m *MockStorage) ExpectImageHandler(c ImageHandler) *MockStorage {
	m.imageHandler = append(m.imageHandler, c)
	return m
}

func (m *MockStorage) ExpectDefaultDiskDriver(c DefaultDiskDriver) *MockStorage {
	m.defaultDiskDriver = append(m.defaultDiskDriver, c)
	return m
}

func (m *MockStorage) ExpectGenerateDeviceName(c GenerateDeviceName) *MockStorage {
	m.generateDeviceName = append(m.generateDeviceName, c)
	return m
}

func (m *MockStorage) GetPool(name string) (storage.Pool, error) {
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
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.imageHandler,
		must.Sprint("Unexpected call to ImageHandler"))
	call := m.imageHandler[0]
	m.imageHandler = m.imageHandler[1:]

	return call.Result
}

func (m *MockStorage) DefaultDiskDriver() string {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.defaultDiskDriver,
		must.Sprint("Unexpected call to DefaultDiskDriver"))
	call := m.defaultDiskDriver[0]
	m.defaultDiskDriver = m.defaultDiskDriver[1:]

	return call.Result
}

func (m *MockStorage) GenerateDeviceName(diskType string, existingDevices []string) string {
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
	m.t.Helper()

	must.SliceEmpty(m.t, m.getPool,
		must.Sprintf("GetPool expecting %d more invocations", len(m.getPool)))
	must.SliceEmpty(m.t, m.imageHandler,
		must.Sprintf("ImageHandler expecting %d more invocations", len(m.imageHandler)))
	must.SliceEmpty(m.t, m.defaultDiskDriver,
		must.Sprintf("DefaultDiskDriver expecting %d more invocations", len(m.defaultDiskDriver)))
	must.SliceEmpty(m.t, m.generateDeviceName,
		must.Sprintf("GenerateDeviceName expecting %d more invocations", len(m.generateDeviceName)))
}

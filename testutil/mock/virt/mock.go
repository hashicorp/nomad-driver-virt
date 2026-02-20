// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"sync"

	"github.com/google/go-cmp/cmp/cmpopts"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/shared/structs"
	"github.com/shoenig/test/must"
)

type Init struct {
	Err error
}

type CreateVM struct {
	Config *vm.Config
	Err    error
}

type StopVM struct {
	Name string
	Err  error
}

type DestroyVM struct {
	Name string
	Err  error
}

type GetVM struct {
	Name   string
	Result *vm.Info
	Err    error
}

type GetInfo struct {
	Result vm.VirtualizerInfo
	Err    error
}

type GetNetworkInterfaces struct {
	Name   string
	Result []vm.NetworkInterface
	Err    error
}

type UseCloudInit struct {
	Result bool
}

type Networking struct {
	Result net.Net
	Err    error
}

type Fingerprint struct {
	Result map[string]*structs.Attribute
	Err    error
}

type SetupStorage struct {
	Config *storage.Config
	Err    error
}

type Storage struct {
	Result storage.Storage
}

func NewMock(t must.T) *MockVirt {
	return &MockVirt{t: t}
}

type MockVirt struct {
	t must.T

	init                 []Init
	createVm             []CreateVM
	stopVm               []StopVM
	destroyVm            []DestroyVM
	getVm                []GetVM
	getInfo              []GetInfo
	getNetworkInterfaces []GetNetworkInterfaces
	useCloudInit         []UseCloudInit
	networking           []Networking
	fingerprint          []Fingerprint
	setupStorage         []SetupStorage
	storage              []Storage
	m                    sync.Mutex
}

func (m *MockVirt) Expect(calls ...any) *MockVirt {
	for _, call := range calls {
		switch c := call.(type) {
		case Init:
			m.ExpectInit(c)
		case CreateVM:
			m.ExpectCreateVM(c)
		case StopVM:
			m.ExpectStopVM(c)
		case DestroyVM:
			m.ExpectDestroyVM(c)
		case GetVM:
			m.ExpectGetVM(c)
		case GetInfo:
			m.ExpectGetInfo(c)
		case GetNetworkInterfaces:
			m.ExpectGetNetworkInterfaces(c)
		case UseCloudInit:
			m.ExpectUseCloudInit(c)
		case Networking:
			m.ExpectNetworking(c)
		case Fingerprint:
			m.ExpectFingerprint(c)
		case SetupStorage:
			m.ExpectSetupStorage(c)
		case Storage:
			m.ExpectStorage(c)
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockVirt) ExpectInit(c Init) *MockVirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.init = append(m.init, c)
	return m
}

func (m *MockVirt) ExpectCreateVM(c CreateVM) *MockVirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.createVm = append(m.createVm, c)
	return m
}

func (m *MockVirt) ExpectStopVM(c StopVM) *MockVirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.stopVm = append(m.stopVm, c)
	return m
}

func (m *MockVirt) ExpectDestroyVM(c DestroyVM) *MockVirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.destroyVm = append(m.destroyVm, c)
	return m
}

func (m *MockVirt) ExpectGetVM(c GetVM) *MockVirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.getVm = append(m.getVm, c)
	return m
}

func (m *MockVirt) ExpectGetInfo(c GetInfo) *MockVirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.getInfo = append(m.getInfo, c)
	return m
}

func (m *MockVirt) ExpectGetNetworkInterfaces(c GetNetworkInterfaces) *MockVirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.getNetworkInterfaces = append(m.getNetworkInterfaces, c)
	return m
}

func (m *MockVirt) ExpectUseCloudInit(c UseCloudInit) *MockVirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.useCloudInit = append(m.useCloudInit, c)
	return m
}

func (m *MockVirt) ExpectNetworking(c Networking) *MockVirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.networking = append(m.networking, c)
	return m
}

func (m *MockVirt) ExpectFingerprint(c Fingerprint) *MockVirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.fingerprint = append(m.fingerprint, c)
	return m
}

func (m *MockVirt) ExpectSetupStorage(c SetupStorage) *MockVirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.setupStorage = append(m.setupStorage, c)
	return m
}

func (m *MockVirt) ExpectStorage(c Storage) *MockVirt {
	m.m.Lock()
	defer m.m.Unlock()

	m.storage = append(m.storage, c)
	return m
}

func (m *MockVirt) Init() error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.init,
		must.Sprint("Unexpected call to Init"))

	call := m.init[0]
	m.init = m.init[1:]

	return call.Err
}

func (m *MockVirt) CreateVM(config *vm.Config) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.createVm,
		must.Sprint("Unexpected call to CreateVM"))
	call := m.createVm[0]
	m.createVm = m.createVm[1:]

	// NOTE: ignore the content field for now until dynamic content
	// can be handled better.
	must.Eq(m.t, call.Config, config,
		must.Sprint("CreateVM received incorrect argument"),
		must.Cmp(cmpopts.IgnoreFields(vm.File{}, "Content")))

	return call.Err
}

func (m *MockVirt) StopVM(name string) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.stopVm,
		must.Sprint("Unexpected call to StopVM"))
	call := m.stopVm[0]
	m.stopVm = m.stopVm[1:]

	must.Eq(m.t, struct{ Name string }{call.Name}, struct{ Name string }{name},
		must.Sprint("StopVM received incorrect argument"))

	return call.Err
}

func (m *MockVirt) DestroyVM(name string) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.destroyVm,
		must.Sprint("Unexpected call to DestroyVM"))
	call := m.destroyVm[0]
	m.destroyVm = m.destroyVm[1:]

	must.Eq(m.t, struct{ Name string }{call.Name}, struct{ Name string }{name},
		must.Sprint("DestroyVM received incorrect argument"))

	return call.Err
}

func (m *MockVirt) GetVM(name string) (*vm.Info, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getVm,
		must.Sprint("Unexpected call to GetVM"))
	call := m.getVm[0]
	m.getVm = m.getVm[1:]

	must.Eq(m.t, struct{ Name string }{call.Name}, struct{ Name string }{name},
		must.Sprint("GetVM received incorrect argument"))

	return call.Result, call.Err
}

func (m *MockVirt) GetInfo() (vm.VirtualizerInfo, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getInfo,
		must.Sprint("Unexpected call to GetInfo"))
	call := m.getInfo[0]
	m.getInfo = m.getInfo[1:]

	return call.Result, call.Err
}

func (m *MockVirt) GetNetworkInterfaces(name string) ([]vm.NetworkInterface, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getNetworkInterfaces,
		must.Sprint("Unexpected call to GetNetworkInterfaces"))
	call := m.getNetworkInterfaces[0]
	m.getNetworkInterfaces = m.getNetworkInterfaces[1:]

	must.Eq(m.t, struct{ Name string }{call.Name}, struct{ Name string }{name},
		must.Sprint("GetNetworkInterfaces received incorrect argument"))

	return call.Result, call.Err
}

func (m *MockVirt) UseCloudInit() bool {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.useCloudInit,
		must.Sprint("Unexpected call to UseCloudInit"))
	call := m.useCloudInit[0]
	m.useCloudInit = m.useCloudInit[1:]

	return call.Result
}

func (m *MockVirt) Networking() (net.Net, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.networking,
		must.Sprint("Unexpected call to Networking"))
	call := m.networking[0]
	m.networking = m.networking[1:]

	return call.Result, call.Err
}

func (m *MockVirt) Fingerprint() (map[string]*structs.Attribute, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.fingerprint,
		must.Sprint("Unexpected call to Fingerprint"))
	call := m.fingerprint[0]
	m.fingerprint = m.fingerprint[1:]

	return call.Result, call.Err
}

func (m *MockVirt) SetupStorage(config *storage.Config) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.setupStorage,
		must.Sprint("Unexpected call to SetupStorage"))
	call := m.setupStorage[0]
	m.setupStorage = m.setupStorage[1:]

	must.Eq(m.t, call.Config, config,
		must.Sprint("SetupStorage received incorrect argument"))

	return call.Err
}

func (m *MockVirt) Storage() storage.Storage {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.storage,
		must.Sprint("Unexpected call to Storage"))
	call := m.storage[0]
	m.storage = m.storage[1:]

	return call.Result
}

// AssertExpectations verifies that all expected invocations
// have been called.
func (m *MockVirt) AssertExpectations() {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceEmpty(m.t, m.init,
		must.Sprintf("Init expecting %d more invocations", len(m.init)))
	must.SliceEmpty(m.t, m.createVm,
		must.Sprintf("CreateVM expecting %d more invocations", len(m.createVm)))
	must.SliceEmpty(m.t, m.stopVm,
		must.Sprintf("StopVM expecting %d more invocations", len(m.stopVm)))
	must.SliceEmpty(m.t, m.destroyVm,
		must.Sprintf("DestroyVM expecting %d more invocations", len(m.destroyVm)))
	must.SliceEmpty(m.t, m.getVm,
		must.Sprintf("GetVM expecting %d more invocations", len(m.getVm)))
	must.SliceEmpty(m.t, m.getInfo,
		must.Sprintf("GetInfo expecting %d more invocations", len(m.getInfo)))
	must.SliceEmpty(m.t, m.getNetworkInterfaces,
		must.Sprintf("GetNetworkInterfaces expecting %d more invocations", len(m.getNetworkInterfaces)))
	must.SliceEmpty(m.t, m.useCloudInit,
		must.Sprintf("UseCloudInit expecting %d more invocations", len(m.useCloudInit)))
	must.SliceEmpty(m.t, m.networking,
		must.Sprintf("Networking expecting %d more invocations", len(m.networking)))
	must.SliceEmpty(m.t, m.fingerprint,
		must.Sprintf("Fingerprint expecting %d more invocations", len(m.fingerprint)))
	must.SliceEmpty(m.t, m.setupStorage,
		must.Sprintf("SetupStorage expecting %d more invocations", len(m.setupStorage)))
	must.SliceEmpty(m.t, m.storage,
		must.Sprintf("Storage expecting %d more invocations", len(m.storage)))
}

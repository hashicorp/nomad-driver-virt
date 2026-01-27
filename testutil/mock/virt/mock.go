// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
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
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockVirt) ExpectInit(c Init) *MockVirt {
	m.init = append(m.init, c)
	return m
}

func (m *MockVirt) ExpectCreateVM(c CreateVM) *MockVirt {
	m.createVm = append(m.createVm, c)
	return m
}

func (m *MockVirt) ExpectStopVM(c StopVM) *MockVirt {
	m.stopVm = append(m.stopVm, c)
	return m
}

func (m *MockVirt) ExpectDestroyVM(c DestroyVM) *MockVirt {
	m.destroyVm = append(m.destroyVm, c)
	return m
}

func (m *MockVirt) ExpectGetVM(c GetVM) *MockVirt {
	m.getVm = append(m.getVm, c)
	return m
}

func (m *MockVirt) ExpectGetInfo(c GetInfo) *MockVirt {
	m.getInfo = append(m.getInfo, c)
	return m
}

func (m *MockVirt) ExpectGetNetworkInterfaces(c GetNetworkInterfaces) *MockVirt {
	m.getNetworkInterfaces = append(m.getNetworkInterfaces, c)
	return m
}

func (m *MockVirt) ExpectUseCloudInit(c UseCloudInit) *MockVirt {
	m.useCloudInit = append(m.useCloudInit, c)
	return m
}

func (m *MockVirt) ExpectNetworking(c Networking) *MockVirt {
	m.networking = append(m.networking, c)
	return m
}

func (m *MockVirt) ExpectFingerprint(c Fingerprint) *MockVirt {
	m.fingerprint = append(m.fingerprint, c)
	return m
}

func (m *MockVirt) Init() error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.init,
		must.Sprint("Unexpected call to Init"))
	call := m.init[0]
	m.init = m.init[1:]

	return call.Err
}

func (m *MockVirt) CreateVM(c *vm.Config) error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.createVm,
		must.Sprint("Unexpected call to CreateVM"))
	call := m.createVm[0]
	m.createVm = m.createVm[1:]

	must.NotNil(m.t, c, must.Sprint("CreateVM received incorrect argument"))

	if call.Config != nil {
		expectedC := *call.Config
		actualC := *c
		must.Eq(m.t, expectedC, actualC, must.Sprint("CreateVM received incorrect argument"))
	}

	return call.Err
}

func (m *MockVirt) StopVM(n string) error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.stopVm,
		must.Sprint("Unexpected call to StopVM"))
	call := m.stopVm[0]
	m.stopVm = m.stopVm[1:]

	must.Eq(m.t, call.Name, n, must.Sprint("StopVM received incorrect argument"))

	return call.Err
}

func (m *MockVirt) DestroyVM(n string) error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.destroyVm,
		must.Sprint("Unexpected call to DestroyVM"))
	call := m.destroyVm[0]
	m.destroyVm = m.destroyVm[1:]

	must.Eq(m.t, call.Name, n, must.Sprint("DestroyVM received incorrect argument"))

	return call.Err
}

func (m *MockVirt) GetVM(n string) (*vm.Info, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getVm,
		must.Sprint("Unexpected call to GetVM"))
	call := m.getVm[0]
	m.getVm = m.getVm[1:]

	must.Eq(m.t, call.Name, n, must.Sprint("GetVM received incorrect argument"))

	return call.Result, call.Err
}

func (m *MockVirt) GetInfo() (vm.VirtualizerInfo, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getInfo,
		must.Sprint("Unexpected call to GetInfo"))
	call := m.getInfo[0]
	m.getInfo = m.getInfo[1:]

	return call.Result, call.Err
}

func (m *MockVirt) GetNetworkInterfaces(n string) ([]vm.NetworkInterface, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getNetworkInterfaces,
		must.Sprint("Unexpected call to GetNetworkInterfaces"))
	call := m.getNetworkInterfaces[0]
	m.getNetworkInterfaces = m.getNetworkInterfaces[1:]

	must.Eq(m.t, call.Name, n, must.Sprint("GetNetworkInterfaces received incorrect argument"))

	return call.Result, call.Err
}

func (m *MockVirt) UseCloudInit() bool {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.useCloudInit,
		must.Sprint("Unexpected call to UseCloudInit"))
	call := m.useCloudInit[0]
	m.useCloudInit = m.useCloudInit[1:]

	return call.Result
}

func (m *MockVirt) Networking() (net.Net, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.networking,
		must.Sprint("Unexpected call to Networking"))
	call := m.networking[0]
	m.networking = m.networking[1:]

	return call.Result, call.Err
}

func (m *MockVirt) Fingerprint() (map[string]*structs.Attribute, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.fingerprint,
		must.Sprint("Unexpected call to Fingerprint"))
	call := m.fingerprint[0]
	m.fingerprint = m.fingerprint[1:]

	return call.Result, call.Err
}

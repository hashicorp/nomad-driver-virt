// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package providers

import (
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/virt"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/shoenig/test/must"
)

type Setup struct {
	Config *virt.Config
	Err    error
}

type Get struct {
	Name   string
	Result virt.Virtualizer
	Err    error
}

type Default struct {
	Result virt.Virtualizer
	Err    error
}

type GetVM struct {
	Name   string
	Result *vm.Info
	Err    error
}

type GetProviderForVM struct {
	Name   string
	Result virt.Virtualizer
	Err    error
}

type Fingerprint struct {
	Result *drivers.Fingerprint
	Err    error
}

func NewMock(t must.T) *MockProviders {
	return &MockProviders{t: t}
}

type MockProviders struct {
	t must.T

	setup            []Setup
	get              []Get
	defaults         []Default
	getVm            []GetVM
	getProviderForVm []GetProviderForVM
	fingerprint      []Fingerprint
}

func (m *MockProviders) Expect(calls ...any) *MockProviders {
	for _, call := range calls {
		switch c := call.(type) {
		case Setup:
			m.ExpectSetup(c)
		case Get:
			m.ExpectGet(c)
		case Default:
			m.ExpectDefault(c)
		case GetVM:
			m.ExpectGetVM(c)
		case GetProviderForVM:
			m.ExpectGetProviderForVM(c)
		case Fingerprint:
			m.ExpectFingerprint(c)
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockProviders) ExpectSetup(s Setup) *MockProviders {
	m.setup = append(m.setup, s)
	return m
}

func (m *MockProviders) ExpectGet(g Get) *MockProviders {
	m.get = append(m.get, g)
	return m
}

func (m *MockProviders) ExpectDefault(d Default) *MockProviders {
	m.defaults = append(m.defaults, d)
	return m
}

func (m *MockProviders) ExpectGetVM(v GetVM) *MockProviders {
	m.getVm = append(m.getVm, v)
	return m
}

func (m *MockProviders) ExpectGetProviderForVM(g GetProviderForVM) *MockProviders {
	m.getProviderForVm = append(m.getProviderForVm, g)
	return m
}

func (m *MockProviders) ExpectFingerprint(f Fingerprint) *MockProviders {
	m.fingerprint = append(m.fingerprint, f)
	return m
}

func (m *MockProviders) Setup(c *virt.Config) error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.setup,
		must.Sprint("Unexpected call to Setup"))
	call := m.setup[0]
	m.setup = m.setup[1:]

	if call.Config == nil {
		must.Nil(m.t, c,
			must.Sprint("Setup received incorrect argument (expected nil)"))
	} else {
		must.Eq(m.t, *call.Config, *c,
			must.Sprint("Setup received incorrect argument"))
	}

	return call.Err
}

func (m *MockProviders) Get(name string) (virt.Virtualizer, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.get,
		must.Sprint("Unexpected call to Get"))
	call := m.get[0]
	m.get = m.get[1:]

	must.Eq(m.t, call.Name, name,
		must.Sprint("Get received incorrect argument"))
	return call.Result, call.Err
}

func (m *MockProviders) Default() (virt.Virtualizer, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.defaults,
		must.Sprint("Unexpected call to Default"))
	call := m.defaults[0]
	m.defaults = m.defaults[1:]

	return call.Result, call.Err
}

func (m *MockProviders) GetVM(name string) (*vm.Info, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getVm,
		must.Sprint("Unexpected call to GetVM"))
	call := m.getVm[0]
	m.getVm = m.getVm[1:]

	must.Eq(m.t, call.Name, name,
		must.Sprint("GetVM received incorrect argument"))
	return call.Result, call.Err
}

func (m *MockProviders) GetProviderForVM(name string) (virt.Virtualizer, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getProviderForVm,
		must.Sprint("Unexpected call to GetProviderForVM"))
	call := m.getProviderForVm[0]
	m.getProviderForVm = m.getProviderForVm[1:]

	must.Eq(m.t, call.Name, name,
		must.Sprint("GetProviderForVM received incorrect argument"))
	return call.Result, call.Err
}

func (m *MockProviders) Fingerprint() (*drivers.Fingerprint, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.fingerprint,
		must.Sprint("Unexpected call to Fingerprint"))
	call := m.fingerprint[0]
	m.fingerprint = m.fingerprint[1:]

	return call.Result, call.Err
}

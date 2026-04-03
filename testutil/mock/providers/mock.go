// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package providers

import (
	"context"
	"sync"

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
	m                sync.Mutex
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
	m.m.Lock()
	defer m.m.Unlock()

	m.setup = append(m.setup, s)
	return m
}

func (m *MockProviders) ExpectGet(g Get) *MockProviders {
	m.m.Lock()
	defer m.m.Unlock()

	m.get = append(m.get, g)
	return m
}

func (m *MockProviders) ExpectDefault(d Default) *MockProviders {
	m.m.Lock()
	defer m.m.Unlock()

	m.defaults = append(m.defaults, d)
	return m
}

func (m *MockProviders) ExpectGetVM(v GetVM) *MockProviders {
	m.m.Lock()
	defer m.m.Unlock()

	m.getVm = append(m.getVm, v)
	return m
}

func (m *MockProviders) ExpectGetProviderForVM(g GetProviderForVM) *MockProviders {
	m.m.Lock()
	defer m.m.Unlock()

	m.getProviderForVm = append(m.getProviderForVm, g)
	return m
}

func (m *MockProviders) ExpectFingerprint(f Fingerprint) *MockProviders {
	m.m.Lock()
	defer m.m.Unlock()

	m.fingerprint = append(m.fingerprint, f)
	return m
}

func (m *MockProviders) Setup(c *virt.Config) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.setup,
		must.Sprint("Unexpected call to Setup"))
	call := m.setup[0]
	m.setup = m.setup[1:]

	if call.Config == nil && c != nil {
		must.Nil(m.t, c,
			must.Sprint("Setup received incorrect argument (expected nil)"))
	} else {
		must.Eq(m.t, *call.Config, *c,
			must.Sprint("Setup received incorrect argument"))
	}

	return call.Err
}

func (m *MockProviders) Get(_ context.Context, name string) (virt.Virtualizer, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.get,
		must.Sprint("Unexpected call to Get"))
	call := m.get[0]
	m.get = m.get[1:]

	must.Eq(m.t, struct{ Name string }{call.Name}, struct{ Name string }{name},
		must.Sprint("Get received incorrect argument"))
	return call.Result, call.Err
}

func (m *MockProviders) Default(context.Context) (virt.Virtualizer, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.defaults,
		must.Sprint("Unexpected call to Default"))
	call := m.defaults[0]
	m.defaults = m.defaults[1:]

	return call.Result, call.Err
}

func (m *MockProviders) GetVM(name string) (*vm.Info, error) {
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

func (m *MockProviders) GetProviderForVM(_ context.Context, name string) (virt.Virtualizer, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getProviderForVm,
		must.Sprint("Unexpected call to GetProviderForVM"))
	call := m.getProviderForVm[0]
	m.getProviderForVm = m.getProviderForVm[1:]

	must.Eq(m.t, struct{ Name string }{call.Name}, struct{ Name string }{name},
		must.Sprint("GetProviderForVM received incorrect argument"))
	return call.Result, call.Err
}

func (m *MockProviders) Fingerprint() (*drivers.Fingerprint, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.fingerprint,
		must.Sprint("Unexpected call to Fingerprint"))
	call := m.fingerprint[0]
	m.fingerprint = m.fingerprint[1:]

	return call.Result, call.Err
}

func (m *MockProviders) AssertExpectations() {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceEmpty(m.t, m.setup,
		must.Sprintf("Setup expecting %d more invocations", len(m.setup)))
	must.SliceEmpty(m.t, m.get,
		must.Sprintf("Get expecting %d more invocations", len(m.get)))
	must.SliceEmpty(m.t, m.defaults,
		must.Sprintf("Defaults expecting %d more invocations", len(m.defaults)))
	must.SliceEmpty(m.t, m.getVm,
		must.Sprintf("GetVM expecting %d more invocations", len(m.getVm)))
	must.SliceEmpty(m.t, m.getProviderForVm,
		must.Sprintf("GetProviderForVM expecting %d more invocations", len(m.getProviderForVm)))
	must.SliceEmpty(m.t, m.fingerprint,
		must.Sprintf("Fingerprint expecting %d more invocations", len(m.fingerprint)))

}

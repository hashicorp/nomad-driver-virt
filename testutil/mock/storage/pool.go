// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
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
	AddVolumeResult *storage.Volume
}

func (s *StaticPool) AddVolume(string, storage.Options) (*storage.Volume, error) {
	if s.AddVolumeResult != nil {
		return s.AddVolumeResult, nil
	}

	return &storage.Volume{}, nil
}

func (s *StaticPool) DeleteVolume(string) error {
	return nil
}

type AddVolume struct {
	Name   string
	Opts   storage.Options
	Result *storage.Volume
	Err    error
}

type DeleteVolume struct {
	Name string
	Err  error
}

type MockPool struct {
	t must.T

	addVolume    []AddVolume
	deleteVolume []DeleteVolume
}

func (m *MockPool) Expect(calls ...any) *MockPool {
	for _, call := range calls {
		switch c := call.(type) {
		case AddVolume:
			m.ExpectAddVolume(c)
		case DeleteVolume:
			m.ExpectDeleteVolume(c)
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockPool) ExpectAddVolume(c AddVolume) *MockPool {
	m.addVolume = append(m.addVolume, c)
	return m
}

func (m *MockPool) ExpectDeleteVolume(c DeleteVolume) *MockPool {
	m.deleteVolume = append(m.deleteVolume, c)
	return m
}

func (m *MockPool) AddVolume(name string, opts storage.Options) (*storage.Volume, error) {
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

func (m *MockPool) DeleteVolume(name string) error {
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

// AssertExpectations verifies that all expected invocations
// have been called.
func (m *MockPool) AssertExpectations() {
	m.t.Helper()

	must.SliceEmpty(m.t, m.addVolume,
		must.Sprintf("AddVolume expecting %d more invocations", len(m.addVolume)))
	must.SliceEmpty(m.t, m.deleteVolume,
		must.Sprintf("DeleteVolume expecting %d more invocations", len(m.deleteVolume)))
}

var (
	_ storage.Pool = (*StaticPool)(nil)
	_ storage.Pool = (*MockPool)(nil)
)

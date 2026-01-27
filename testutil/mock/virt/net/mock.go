// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package net

import (
	"maps"
	"slices"

	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/shared/structs"
	"github.com/shoenig/test/must"
)

func NewMock(t must.T) *MockNet {
	return &MockNet{t: t}
}

type Init struct {
	Err error
}

type Fingerprint struct {
	Attrs   map[string]*structs.Attribute       // Expected attributes to receive (nil value prevents check)
	AttrsFn func(map[string]*structs.Attribute) // Allows for modifications
}

type VMStartedBuild struct {
	Request *net.VMStartedBuildRequest
	Result  *net.VMStartedBuildResponse
	Err     error
}

type VMTerminatedTeardown struct {
	Request *net.VMTerminatedTeardownRequest
	Result  *net.VMTerminatedTeardownResponse
	Err     error
}

type MockNet struct {
	t                    must.T
	init                 []Init
	fingerprint          []Fingerprint
	vmStartedBuild       []VMStartedBuild
	vmTerminatedTeardown []VMTerminatedTeardown
}

func (m *MockNet) Expect(calls ...any) *MockNet {
	for _, call := range calls {
		switch c := call.(type) {
		case Init:
			m.ExpectInit(c)
		case Fingerprint:
			m.ExpectFingerprint(c)
		case VMStartedBuild:
			m.ExpectVMStartedBuild(c)
		case VMTerminatedTeardown:
			m.ExpectVMTerminatedTeardown(c)
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockNet) ExpectInit(c Init) *MockNet {
	m.init = append(m.init, c)
	return m
}

func (m *MockNet) ExpectFingerprint(c Fingerprint) *MockNet {
	m.fingerprint = append(m.fingerprint, c)
	return m
}

func (m *MockNet) ExpectVMStartedBuild(c VMStartedBuild) *MockNet {
	m.vmStartedBuild = append(m.vmStartedBuild, c)
	return m
}

func (m *MockNet) ExpectVMTerminatedTeardown(c VMTerminatedTeardown) *MockNet {
	m.vmTerminatedTeardown = append(m.vmTerminatedTeardown, c)
	return m
}

func (m *MockNet) Init() error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.init,
		must.Sprint("Unexpected call to Init"))
	call := m.init[0]
	m.init = m.init[1:]

	return call.Err
}

func (m *MockNet) Fingerprint(attrs map[string]*structs.Attribute) {
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

func (m *MockNet) VMStartedBuild(request *net.VMStartedBuildRequest) (*net.VMStartedBuildResponse, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.vmStartedBuild,
		must.Sprint("Unexpected call to VMStartedBuild"))
	call := m.vmStartedBuild[0]
	m.vmStartedBuild = m.vmStartedBuild[1:]

	must.NotNil(m.t, request, must.Sprint("VMStartedBuild received incorrect argument"))
	if call.Request != nil {
		must.True(m.t, call.Request.IsEqual(request),
			must.Sprint("VMStartedBuild request does not match expected"))
	}

	return call.Result, call.Err
}

func (m *MockNet) VMTerminatedTeardown(request *net.VMTerminatedTeardownRequest) (*net.VMTerminatedTeardownResponse, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.vmTerminatedTeardown,
		must.Sprint("Unexpected call to VMTerminatedTeardown"))
	call := m.vmTerminatedTeardown[0]
	m.vmTerminatedTeardown = m.vmTerminatedTeardown[1:]

	must.NotNil(m.t, request, must.Sprint("VMTerminatedTeardown received incorrect argument"))
	if call.Request != nil {
		must.True(m.t, call.Request.IsEqual(request),
			must.Sprint("VMTerminatedTeardown request does not match expected"))
	}

	return call.Result, call.Err
}

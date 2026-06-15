// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package filter

import (
	"fmt"
	"runtime"
	"strings"
	"sync"

	"github.com/hashicorp/go-hclog"
	virtnet "github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/shoenig/test/must"
)

func NewStatic() *StaticFilter {
	return &StaticFilter{}
}

type StaticFilter struct {
	ConfigureResult *virtnet.FilterRemoval

	counts map[string]int
	m      sync.Mutex
	o      sync.Once
}

func (s *StaticFilter) incrCount() {
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

func (s *StaticFilter) CallCount(fnName string) int {
	s.m.Lock()
	defer s.m.Unlock()

	if s.counts == nil {
		return 0
	}

	return s.counts[fnName]
}

func (s *StaticFilter) Configure(*drivers.Resources, *virtnet.NetworkInterfaceBridgeConfig, string) (*virtnet.FilterRemoval, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.ConfigureResult, nil
}

func (s *StaticFilter) Teardown(*virtnet.FilterRemoval) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticFilter) SetLogger(hclog.Logger) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()
}

func NewMock(t must.T) *MockFilter {
	return &MockFilter{t: t}
}

type Configure struct {
	Resources     *drivers.Resources
	NetworkConfig *virtnet.NetworkInterfaceBridgeConfig
	IP            string
	Result        *virtnet.FilterRemoval
	Err           error
}

type Teardown struct {
	Removal *virtnet.FilterRemoval
	Err     error
}

type MockFilter struct {
	configures []Configure
	teardowns  []Teardown
	setLoggers []SetLogger
	t          must.T
	m          sync.Mutex
}

type SetLogger struct{}

func (m *MockFilter) Expect(calls ...any) *MockFilter {
	for _, call := range calls {
		switch c := call.(type) {
		case Configure:
			m.ExpectConfigure(c)
		case Teardown:
			m.ExpectTeardown(c)
		case SetLogger:
			m.ExpectSetLogger(c)
		default:
			panic(fmt.Sprintf("unsupported type for mock expectation: %T", c))
		}
	}

	return m
}

func (m *MockFilter) ExpectConfigure(c Configure) *MockFilter {
	m.m.Lock()
	defer m.m.Unlock()

	m.configures = append(m.configures, c)
	return m
}

func (m *MockFilter) ExpectTeardown(c Teardown) *MockFilter {
	m.m.Lock()
	defer m.m.Unlock()

	m.teardowns = append(m.teardowns, c)
	return m
}

func (m *MockFilter) ExpectSetLogger(c SetLogger) *MockFilter {
	m.m.Lock()
	defer m.m.Unlock()

	m.setLoggers = append(m.setLoggers, c)
	return m
}

func (m *MockFilter) Configure(resources *drivers.Resources, config *virtnet.NetworkInterfaceBridgeConfig, ip string) (*virtnet.FilterRemoval, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.configures,
		must.Sprintf("Unexpected call to Configure - Configure(%v, %q, %q)", resources, config, ip))
	call := m.configures[0]
	m.configures = m.configures[1:]

	// We only care about the ports value within the resources
	// so break down expectations.
	if call.Resources.Ports == nil || resources.Ports == nil {
		must.Eq(m.t, call.Resources.Ports, resources.Ports,
			must.Sprint("Configured received incorrect arguments for resources Ports"))
	} else {
		must.Equal(m.t, *call.Resources.Ports, *resources.Ports,
			must.Sprint("Configured received incorrect arguments for resources Ports"))
	}

	received := Configure{
		NetworkConfig: config,
		IP:            ip,
		Resources:     call.Resources,
		Result:        call.Result,
		Err:           call.Err,
	}
	must.Eq(m.t, call, received,
		must.Sprint("Configure received incorrect arguments"))

	return call.Result, call.Err
}

func (m *MockFilter) Teardown(removal *virtnet.FilterRemoval) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.teardowns,
		must.Sprintf("Unexpected call to Teardown - Teardown(%q)", removal))
	call := m.teardowns[0]
	m.teardowns = m.teardowns[1:]
	must.Eq(m.t, call.Removal, removal,
		must.Sprint("Teardown received incorrect arguments"))

	return call.Err
}

func (m *MockFilter) SetLogger(hclog.Logger) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.setLoggers,
		must.Sprintf("Unexpected call to SetLogger"))
	m.setLoggers = m.setLoggers[1:]
}

func (m *MockFilter) AssertExpectations() {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceEmpty(m.t, m.configures,
		must.Sprintf("Configure expecting %d more invocations", len(m.configures)))
	must.SliceEmpty(m.t, m.teardowns,
		must.Sprintf("Teardown expecting %d more invocations", len(m.teardowns)))
	must.SliceEmpty(m.t, m.setLoggers,
		must.Sprintf("SetLogger expecting %d more invocations", len(m.setLoggers)))
}

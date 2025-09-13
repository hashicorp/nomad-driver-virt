package libvirt

import (
	"fmt"

	iface "github.com/hashicorp/nomad-driver-virt/libvirt"
	"github.com/shoenig/test/must"
)

type ListNetworks struct {
	Result []string
	Err    error
}

type LookupNetworkByName struct {
	Name   string
	Result iface.ConnectNetworkShim
	Err    error
}

type MockConnect struct {
	listNetworks         []ListNetworks
	lookupNetworkByNames []LookupNetworkByName
	t                    must.T
}

// NewConnect returns a new mock compatible with libvirt.ConnectShim
func NewConnect(t must.T) *MockConnect {
	return &MockConnect{t: t}
}

// Expect adds a list of expected calls.
func (m *MockConnect) Expect(calls ...any) *MockConnect {
	for _, call := range calls {
		switch c := call.(type) {
		case ListNetworks:
			m.ExpectListNetworks(c)
		case LookupNetworkByName:
			m.ExpectLookupNetworkByName(c)
		default:
			panic(fmt.Sprintf("unsupported type for mock expectation: %T", c))
		}
	}

	return m
}

// ExpectListNetworks adds an expected ListNetworks call.
func (m *MockConnect) ExpectListNetworks(list ListNetworks) *MockConnect {
	m.listNetworks = append(m.listNetworks, list)
	return m
}

// ExpectLookupNetworkByName adds an expected ExpectLookupNetworkByName call.
func (m *MockConnect) ExpectLookupNetworkByName(lookup LookupNetworkByName) *MockConnect {
	m.lookupNetworkByNames = append(m.lookupNetworkByNames, lookup)
	return m
}

func (m *MockConnect) ListNetworks() ([]string, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.listNetworks,
		must.Sprint("Unexpected call to ListNetworks"))
	call := m.listNetworks[0]
	m.listNetworks = m.listNetworks[1:]

	return call.Result, call.Err
}

func (m *MockConnect) LookupNetworkByName(name string) (iface.ConnectNetworkShim, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.lookupNetworkByNames,
		must.Sprintf("Unexpected call to LookupNetworkByName - LookupNetworkByName(%q)", name))
	call := m.lookupNetworkByNames[0]
	m.lookupNetworkByNames = m.lookupNetworkByNames[1:]

	must.Eq(m.t, call.Name, name,
		must.Sprint("LookupNetworkByName received incorrect arguments"))

	return call.Result, call.Err
}

// AssertExpectations verifies that all expected invocations
// have been called.
func (m *MockConnect) AssertExpectations() {
	m.t.Helper()

	must.SliceEmpty(m.t, m.listNetworks,
		must.Sprintf("ListNetworks expecting %d more invocations", len(m.listNetworks)))
	must.SliceEmpty(m.t, m.lookupNetworkByNames,
		must.Sprintf("LookupNetworkByName expecting %d more invocations", len(m.lookupNetworkByNames)))
}

var _ iface.ConnectShim = (*MockConnect)(nil)

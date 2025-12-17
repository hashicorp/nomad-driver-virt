// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"crypto/rand"
	"fmt"
	"net/netip"
	"time"

	iface "github.com/hashicorp/nomad-driver-virt/libvirt"
	"github.com/shoenig/test/must"
	"libvirt.org/go/libvirt"
)

type IsActive struct {
	Result bool
	Err    error
}

type GetBridgeName struct {
	Result string
	Err    error
}

type GetDHCPLeases struct {
	Result []libvirt.NetworkDHCPLease
	Err    error
}

type Update struct {
	Cmd         libvirt.NetworkUpdateCommand
	Section     libvirt.NetworkUpdateSection
	ParentIndex int
	Xml         string
	Flags       libvirt.NetworkUpdateFlags
	Err         error
}

type GetXMLDesc struct {
	Flags  libvirt.NetworkXMLFlags
	Result string
	Err    error
}

type MockNetwork struct {
	isActives      []IsActive
	getBridgeNames []GetBridgeName
	getDHCPLeases  []GetDHCPLeases
	updates        []Update
	getXMLDescs    []GetXMLDesc
	t              must.T
}

// NewNetwork returns a new mock compatible with libvirt.ConnectNetworkShim
func NewNetwork(t must.T) *MockNetwork {
	return &MockNetwork{t: t}
}

// Expect adds a list of expected calls.
func (m *MockNetwork) Expect(calls ...any) *MockNetwork {
	for _, call := range calls {
		switch c := call.(type) {
		case IsActive:
			m.ExpectIsActive(c)
		case GetBridgeName:
			m.ExpectGetBridgeName(c)
		case GetDHCPLeases:
			m.ExpectGetDHCPLeases(c)
		case Update:
			m.ExpectUpdate(c)
		case GetXMLDesc:
			m.ExpectGetXMLDesc(c)
		default:
			panic(fmt.Sprintf("unsupported type for mock expectation: %T", c))
		}
	}

	return m
}

// ExpactIsActive adds an expected IsActive call.
func (m *MockNetwork) ExpectIsActive(act IsActive) *MockNetwork {
	m.isActives = append(m.isActives, act)
	return m
}

// ExpectGetBridgeName adds an expected ExpectGetBridgeName call.
func (m *MockNetwork) ExpectGetBridgeName(get GetBridgeName) *MockNetwork {
	m.getBridgeNames = append(m.getBridgeNames, get)
	return m
}

// ExpectGetDHCPLeases adds an expected ExpectGetDHCPLeases call.
func (m *MockNetwork) ExpectGetDHCPLeases(get GetDHCPLeases) *MockNetwork {
	m.getDHCPLeases = append(m.getDHCPLeases, get)
	return m
}

// ExpectUpdate adds an expected Update call.
func (m *MockNetwork) ExpectUpdate(up Update) *MockNetwork {
	m.updates = append(m.updates, up)
	return m
}

// ExpectGetXMLDesc adds an expected GetXMLDesc call.
func (m *MockNetwork) ExpectGetXMLDesc(get GetXMLDesc) *MockNetwork {
	m.getXMLDescs = append(m.getXMLDescs, get)
	return m
}

func (m *MockNetwork) IsActive() (bool, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.isActives,
		must.Sprint("Unexpected call to IsActive"))
	call := m.isActives[0]
	m.isActives = m.isActives[1:]

	return call.Result, call.Err
}

func (m *MockNetwork) GetBridgeName() (string, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getBridgeNames,
		must.Sprint("Unexpected call to GetBridgeName"))
	call := m.getBridgeNames[0]
	m.getBridgeNames = m.getBridgeNames[1:]

	return call.Result, call.Err
}

func (m *MockNetwork) GetDHCPLeases() ([]libvirt.NetworkDHCPLease, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getDHCPLeases,
		must.Sprint("Unexpected call to GetDHCPLeases"))
	call := m.getDHCPLeases[0]
	m.getDHCPLeases = m.getDHCPLeases[1:]

	return call.Result, call.Err
}

func (m *MockNetwork) Update(cmd libvirt.NetworkUpdateCommand, section libvirt.NetworkUpdateSection, parentIndex int, xml string, flags libvirt.NetworkUpdateFlags) error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.updates,
		must.Sprintf("Unexpected call to Update - Update(%q, %q, %q, %q, %q)", cmd, section, parentIndex, xml, flags))
	call := m.updates[0]
	m.updates = m.updates[1:]
	received := Update{
		Cmd:         cmd,
		Section:     section,
		ParentIndex: parentIndex,
		Xml:         xml,
		Flags:       flags,
		Err:         call.Err,
	}
	must.Eq(m.t, call, received,
		must.Sprint("Update received incorrect arguments"))

	return call.Err
}

func (m *MockNetwork) GetXMLDesc(flags libvirt.NetworkXMLFlags) (string, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getXMLDescs,
		must.Sprintf("Unexpected call to GetXMLDesc - GetXMLDesc(%q)", flags))
	call := m.getXMLDescs[0]
	m.getXMLDescs = m.getXMLDescs[1:]
	must.Eq(m.t, call.Flags, flags,
		must.Sprint("GetXMLDesc received incorrect arguments"))

	return call.Result, call.Err
}

// AssertExpectations verifies that all expected invocations
// have been called.
func (m *MockNetwork) AssertExpectations() {
	m.t.Helper()

	must.SliceEmpty(m.t, m.isActives,
		must.Sprintf("IsActive expecting %d more invocations", len(m.isActives)))
	must.SliceEmpty(m.t, m.getBridgeNames,
		must.Sprintf("GetBridgeNames expecting %d more invocations", len(m.getBridgeNames)))
	must.SliceEmpty(m.t, m.getDHCPLeases,
		must.Sprintf("GetDHCPLeases expecting %d more invocations", len(m.getDHCPLeases)))
	must.SliceEmpty(m.t, m.updates,
		must.Sprintf("Update expecting %d more invocations", len(m.updates)))
	must.SliceEmpty(m.t, m.getXMLDescs,
		must.Sprintf("GetXMLDesc expecting %d more invocations", len(m.getXMLDescs)))
}

var startLeaseAddress = netip.MustParseAddr("192.168.88.3")

func GenerateLeaseRecords(t must.T, count int) []libvirt.NetworkDHCPLease {
	records := make([]libvirt.NetworkDHCPLease, count)
	addr := startLeaseAddress
	for i := range count {
		records[i] = libvirt.NetworkDHCPLease{
			Iface:      "virbr0",
			ExpiryTime: time.Now().Add(time.Duration(i) * time.Hour),
			Type:       libvirt.IP_ADDR_TYPE_IPV4,
			Mac:        generateMacAddress(t),
			IPaddr:     addr.String(),
			Hostname:   generateHostname(t),
			Clientid:   generateClientId(t),
		}
		addr = addr.Next()
	}

	return records
}

func generateRandomBytes(t must.T, length int) []byte {
	t.Helper()

	val := make([]byte, length)
	count, err := rand.Read(val)
	must.NoError(t, err, must.Sprint("entropy exhausted generating random bytes"))
	must.Eq(t, length, count, must.Sprint("failed to generate expected number of random bytes"))

	return val
}

func generateHostname(t must.T) string {
	bytes := generateRandomBytes(t, 8)
	hostname := "nomad-"
	for i := range len(bytes) {
		hostname += fmt.Sprintf("%x", bytes[i])
	}

	return hostname
}

func generateMacAddress(t must.T) (mac string) {
	bytes := generateRandomBytes(t, 6)
	for i := range len(bytes) {
		mac += fmt.Sprintf("%x:", bytes[i])
	}

	return mac[0 : len(mac)-1]
}

func generateClientId(t must.T) (clientid string) {
	bytes := generateRandomBytes(t, 19)
	for i := range len(bytes) {
		clientid += fmt.Sprintf("%x:", bytes[i])
	}

	return clientid[0 : len(clientid)-1]

}

var _ iface.ConnectNetworkShim = (*MockNetwork)(nil)

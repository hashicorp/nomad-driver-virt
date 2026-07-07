// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package nftables

import (
	"fmt"
	"sync"

	"github.com/google/nftables"
	"github.com/shoenig/test/must"
)

// New returns a new nftables mock.
func New(t must.T) *MockNFTables {
	return &MockNFTables{t: t}
}

type AddChain struct {
	Chain  *nftables.Chain
	Result *nftables.Chain
}

type AddRule struct {
	Rule   *nftables.Rule
	Result *nftables.Rule
}

type CreateTable struct {
	Table  *nftables.Table
	Result *nftables.Table
}

type DelChain struct {
	Chain *nftables.Chain
}

type DelRule struct {
	Rule *nftables.Rule
	Err  error
}

type DelTable struct {
	Table *nftables.Table
}

type Flush struct {
	Err error
}

type GetRules struct {
	Table  *nftables.Table
	Chain  *nftables.Chain
	Result []*nftables.Rule
	Err    error
}

type Insert struct {
	Rule   *nftables.Rule
	Result *nftables.Rule
}

type ListChain struct {
	Table  *nftables.Table
	Name   string
	Result *nftables.Chain
	Err    error
}

type ListTableOfFamily struct {
	Name   string
	Family nftables.TableFamily
	Result *nftables.Table
	Err    error
}

type MockNFTables struct {
	addChains           []AddChain
	addRules            []AddRule
	createTables        []CreateTable
	delChains           []DelChain
	delRules            []DelRule
	delTables           []DelTable
	flushes             []Flush
	getRules            []GetRules
	insertRules         []Insert
	listChains          []ListChain
	listTableOfFamilies []ListTableOfFamily
	t                   must.T
	m                   sync.Mutex
}

// Expect adds a list of expected calls.
func (m *MockNFTables) Expect(calls ...any) *MockNFTables {
	for _, call := range calls {
		switch c := call.(type) {
		case AddChain:
			m.ExpectAddChain(c)
		case AddRule:
			m.ExpectAddRule(c)
		case CreateTable:
			m.ExpectCreateTable(c)
		case DelChain:
			m.ExpectDelChain(c)
		case DelRule:
			m.ExpectDelRule(c)
		case DelTable:
			m.ExpectDelTable(c)
		case Flush:
			m.ExpectFlush(c)
		case GetRules:
			m.ExpectGetRules(c)
		case Insert:
			m.ExpectInsert(c)
		case ListChain:
			m.ExpectListChain(c)
		case ListTableOfFamily:
			m.ExpectListTableOfFamily(c)
		default:
			panic(fmt.Sprintf("unsupported type for mock expectation: %T", c))
		}
	}

	return m
}

func (m *MockNFTables) ExpectAddChain(c AddChain) *MockNFTables {
	m.m.Lock()
	defer m.m.Unlock()

	m.addChains = append(m.addChains, c)
	return m
}

func (m *MockNFTables) ExpectAddRule(c AddRule) *MockNFTables {
	m.m.Lock()
	defer m.m.Unlock()

	m.addRules = append(m.addRules, c)
	return m
}

func (m *MockNFTables) ExpectCreateTable(c CreateTable) *MockNFTables {
	m.m.Lock()
	defer m.m.Unlock()

	m.createTables = append(m.createTables, c)
	return m
}

func (m *MockNFTables) ExpectDelChain(c DelChain) *MockNFTables {
	m.m.Lock()
	defer m.m.Unlock()

	m.delChains = append(m.delChains, c)
	return m
}

func (m *MockNFTables) ExpectDelRule(c DelRule) *MockNFTables {
	m.m.Lock()
	defer m.m.Unlock()

	m.delRules = append(m.delRules, c)
	return m
}

func (m *MockNFTables) ExpectDelTable(c DelTable) *MockNFTables {
	m.m.Lock()
	defer m.m.Unlock()

	m.delTables = append(m.delTables, c)
	return m
}

func (m *MockNFTables) ExpectFlush(c Flush) *MockNFTables {
	m.m.Lock()
	defer m.m.Unlock()

	m.flushes = append(m.flushes, c)
	return m
}

func (m *MockNFTables) ExpectGetRules(c GetRules) *MockNFTables {
	m.m.Lock()
	defer m.m.Unlock()

	m.getRules = append(m.getRules, c)
	return m
}

func (m *MockNFTables) ExpectInsert(c Insert) *MockNFTables {
	m.m.Lock()
	defer m.m.Unlock()

	m.insertRules = append(m.insertRules, c)
	return m
}

func (m *MockNFTables) ExpectListChain(c ListChain) *MockNFTables {
	m.m.Lock()
	defer m.m.Unlock()

	m.listChains = append(m.listChains, c)
	return m
}

func (m *MockNFTables) ExpectListTableOfFamily(c ListTableOfFamily) *MockNFTables {
	m.m.Lock()
	defer m.m.Unlock()

	m.listTableOfFamilies = append(m.listTableOfFamilies, c)
	return m
}

func (m *MockNFTables) AddChain(chain *nftables.Chain) *nftables.Chain {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.addChains,
		must.Sprintf("Unexpected call to AddChain - AddChain(%v)", chain))
	call := m.addChains[0]
	m.addChains = m.addChains[1:]
	must.Eq(m.t, call.Chain, chain,
		must.Sprint("AddChain received incorrect arguments"))

	return call.Result
}

func (m *MockNFTables) AddRule(rule *nftables.Rule) *nftables.Rule {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.addRules,
		must.Sprintf("Unexpected call to AddRule - AddRule(%v)", rule))
	call := m.addRules[0]
	m.addRules = m.addRules[1:]
	must.Eq(m.t, call.Rule, rule,
		must.Sprint("AddRule received incorrect arguments"))

	return call.Result
}

func (m *MockNFTables) CreateTable(table *nftables.Table) *nftables.Table {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.createTables,
		must.Sprintf("Unexpected call to CreateTable - CreateTable(%v)", table))
	call := m.createTables[0]
	m.createTables = m.createTables[1:]
	must.Eq(m.t, call.Table, table,
		must.Sprint("CreateTable received incorrect arguments"))

	return call.Result
}

func (m *MockNFTables) DelChain(chain *nftables.Chain) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.delChains,
		must.Sprintf("Unexpected call to DelChain - DelChain(%v)", chain))
	call := m.delChains[0]
	m.delChains = m.delChains[1:]
	must.Eq(m.t, call.Chain, chain,
		must.Sprint("DelChain received incorrect arguments"))
}

func (m *MockNFTables) DelRule(rule *nftables.Rule) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.delRules,
		must.Sprintf("Unexpected call to DelRule - DelRule(%v)", rule))
	call := m.delRules[0]
	m.delRules = m.delRules[1:]
	must.Eq(m.t, call.Rule, rule,
		must.Sprint("DelRule received incorrect arguments"))

	return call.Err
}

func (m *MockNFTables) DelTable(table *nftables.Table) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.delTables,
		must.Sprintf("Unexpected call to DelTable - DelTable(%v)", table))
	call := m.delTables[0]
	m.delTables = m.delTables[1:]
	must.Eq(m.t, call.Table, table,
		must.Sprint("DelRule received incorrect arguments"))
}

func (m *MockNFTables) Flush() error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.flushes,
		must.Sprint("Unexpected call to Flush - Flush()"))
	call := m.flushes[0]
	m.flushes = m.flushes[1:]

	return call.Err
}

func (m *MockNFTables) GetRules(table *nftables.Table, chain *nftables.Chain) ([]*nftables.Rule, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getRules,
		must.Sprintf("Unexpected call to GetRules - GetRules(%v, %v)", table, chain))
	call := m.getRules[0]
	m.getRules = m.getRules[1:]
	must.Eq(m.t, call, GetRules{Table: table, Chain: chain, Result: call.Result, Err: call.Err},
		must.Sprint("GetRules received incorrect arguments"))

	return call.Result, call.Err
}

func (m *MockNFTables) InsertRule(rule *nftables.Rule) *nftables.Rule {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.insertRules,
		must.Sprintf("Unexpected call to InsertRule - InsertRule(%v)", rule))
	call := m.insertRules[0]
	m.insertRules = m.insertRules[1:]
	must.Eq(m.t, call.Rule, rule,
		must.Sprint("InsertRule received incorrect arguments"))

	return call.Result
}

func (m *MockNFTables) ListChain(table *nftables.Table, name string) (*nftables.Chain, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.listChains,
		must.Sprintf("Unexpected call to ListChain - ListChain(%v, %v)", table, name))
	call := m.listChains[0]
	m.listChains = m.listChains[1:]
	must.Eq(m.t, call, ListChain{Table: table, Name: name, Result: call.Result, Err: call.Err},
		must.Sprint("ListChain received incorrect arguments"))

	return call.Result, call.Err
}

func (m *MockNFTables) ListTableOfFamily(name string, family nftables.TableFamily) (*nftables.Table, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.listTableOfFamilies,
		must.Sprintf("Unexpected call to ListTableOfFamily - ListTableOfFamily(%v, %v)", name, family))
	call := m.listTableOfFamilies[0]
	m.listTableOfFamilies = m.listTableOfFamilies[1:]
	must.Eq(m.t, call, ListTableOfFamily{Name: name, Family: family, Result: call.Result, Err: call.Err},
		must.Sprint("ListTableOfFamily received incorrect arguments"))

	return call.Result, call.Err
}

// AssertExpectations verifies that all expected invocations
// have been called.
func (m *MockNFTables) AssertExpectations() {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceEmpty(m.t, m.addChains,
		must.Sprintf("AddChain expecting %d more invocations", len(m.addChains)))
	must.SliceEmpty(m.t, m.addRules,
		must.Sprintf("AddRule expecting %d more invocations", len(m.addRules)))
	must.SliceEmpty(m.t, m.createTables,
		must.Sprintf("CreateTable expecting %d more invocations", len(m.createTables)))
	must.SliceEmpty(m.t, m.delChains,
		must.Sprintf("DelChain expecting %d more invocations", len(m.delChains)))
	must.SliceEmpty(m.t, m.delRules,
		must.Sprintf("DelRule expecting %d more invocations", len(m.delRules)))
	must.SliceEmpty(m.t, m.delTables,
		must.Sprintf("DelTable expecting %d more invocations", len(m.delTables)))
	must.SliceEmpty(m.t, m.flushes,
		must.Sprintf("Flush expecting %d more invocations", len(m.flushes)))
	must.SliceEmpty(m.t, m.getRules,
		must.Sprintf("GetRules expecting %d more invocations", len(m.getRules)))
	must.SliceEmpty(m.t, m.insertRules,
		must.Sprintf("InsertRule expecting %d more invocations", len(m.insertRules)))
	must.SliceEmpty(m.t, m.listChains,
		must.Sprintf("ListChain expecting %d more invocations", len(m.listChains)))
	must.SliceEmpty(m.t, m.listTableOfFamilies,
		must.Sprintf("ListTableOfFamily expecting %d more invocations", len(m.listTableOfFamilies)))
}

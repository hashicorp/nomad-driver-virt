// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"fmt"

	"github.com/shoenig/test/must"
)

// New returns a new mock compatible with net.IPTables
func New(t must.T) *mockIPTables {
	return &mockIPTables{t: t}
}

type Append struct {
	Table, Chain string
	RuleSpec     []string
	Err          error
}

type ClearChain struct {
	Table, Chain string
	Err          error
}

type Delete struct {
	Table, Chain string
	RuleSpec     []string
	Err          error
}

type DeleteChain struct {
	Table, Chain string
	Err          error
}

type DeleteIfExists struct {
	Table, Chain string
	RuleSpec     []string
	Err          error
}

type Insert struct {
	Table, Chain string
	Pos          int
	RuleSpec     []string
	Err          error
}

type ListChains struct {
	Table  string
	Result []string
	Err    error
}

type NewChain struct {
	Table, Chain string
	Err          error
}

type mockIPTables struct {
	appends        []Append
	clearChains    []ClearChain
	deletes        []Delete
	deleteChains   []DeleteChain
	deleteIfExists []DeleteIfExists
	inserts        []Insert
	listChains     []ListChains
	newChains      []NewChain
	t              must.T
}

// Expect adds a list of expected calls.
func (m *mockIPTables) Expect(calls ...any) *mockIPTables {
	for _, call := range calls {
		switch c := call.(type) {
		case Append:
			m.ExpectAppend(c)
		case ClearChain:
			m.ExpectClearChain(c)
		case Delete:
			m.ExpectDelete(c)
		case DeleteChain:
			m.ExpectDeleteChain(c)
		case DeleteIfExists:
			m.ExpectDeleteIfExists(c)
		case Insert:
			m.ExpectInsert(c)
		case ListChains:
			m.ExpectListChains(c)
		case NewChain:
			m.ExpectNewChain(c)
		default:
			panic(fmt.Sprintf("unsupported type for mock expectation: %T", c))
		}
	}

	return m
}

// ExpectAppend adds an expected Append call.
func (m *mockIPTables) ExpectAppend(app Append) *mockIPTables {
	m.appends = append(m.appends, app)
	return m
}

// ExpectClearChain adds an expected ClearChain call.
func (m *mockIPTables) ExpectClearChain(clear ClearChain) *mockIPTables {
	m.clearChains = append(m.clearChains, clear)
	return m
}

// ExpectDelete adds an expected Delete call.
func (m *mockIPTables) ExpectDelete(del Delete) *mockIPTables {
	m.deletes = append(m.deletes, del)
	return m
}

// ExpectDeleteChain adds an expected DeleteChain call.
func (m *mockIPTables) ExpectDeleteChain(del DeleteChain) *mockIPTables {
	m.deleteChains = append(m.deleteChains, del)
	return m
}

// ExpectDeleteIfExists adds an expected DeleteIfExists call.
func (m *mockIPTables) ExpectDeleteIfExists(del DeleteIfExists) *mockIPTables {
	m.deleteIfExists = append(m.deleteIfExists, del)
	return m
}

// ExpectInsert adds an expected Insert call.
func (m *mockIPTables) ExpectInsert(ins Insert) *mockIPTables {
	m.inserts = append(m.inserts, ins)
	return m
}

// ExpectListChains adds an expected ListChains call.
func (m *mockIPTables) ExpectListChains(list ListChains) *mockIPTables {
	m.listChains = append(m.listChains, list)
	return m
}

// ExpectNewChain adds an expected NewChain call.
func (m *mockIPTables) ExpectNewChain(new NewChain) *mockIPTables {
	m.newChains = append(m.newChains, new)
	return m
}

func (m *mockIPTables) Append(table, chain string, rulespec ...string) error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.appends,
		must.Sprintf("Unexpected call to Append - Append(%q, %q, %q)", table, chain, rulespec))
	call := m.appends[0]
	m.appends = m.appends[1:]
	received := Append{
		Table:    table,
		Chain:    chain,
		RuleSpec: rulespec,
		Err:      call.Err,
	}
	must.Eq(m.t, call, received,
		must.Sprint("Append received incorrect arguments"))

	return call.Err
}

func (m *mockIPTables) ClearChain(table, chain string) error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.clearChains,
		must.Sprintf("Unexpected call to ClearChain - ClearChain(%q, %q)", table, chain))
	call := m.clearChains[0]
	m.clearChains = m.clearChains[1:]
	received := ClearChain{
		Table: table,
		Chain: chain,
		Err:   call.Err,
	}
	must.Eq(m.t, call, received,
		must.Sprint("ClearChain received incorrect arguments"))

	return call.Err
}

func (m *mockIPTables) Delete(table, chain string, rulespec ...string) error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.deletes,
		must.Sprintf("Unexpected call to Delete - Delete(%q, %q, %q)", table, chain, rulespec))
	call := m.deletes[0]
	m.deletes = m.deletes[1:]
	received := Delete{
		Table:    table,
		Chain:    chain,
		RuleSpec: rulespec,
		Err:      call.Err,
	}
	must.Eq(m.t, call, received,
		must.Sprint("Delete received incorrect arguments"))

	return call.Err
}

func (m *mockIPTables) DeleteChain(table, chain string) error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.deleteChains,
		must.Sprintf("Unexpected call to DeleteChain - DeleteChain(%q, %q)", table, chain))
	call := m.deleteChains[0]
	m.deleteChains = m.deleteChains[1:]
	received := DeleteChain{
		Table: table,
		Chain: chain,
		Err:   call.Err,
	}
	must.Eq(m.t, call, received,
		must.Sprint("DeleteChain received incorrect arguments"))

	return call.Err
}

func (m *mockIPTables) DeleteIfExists(table, chain string, rulespec ...string) error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.deleteIfExists,
		must.Sprintf("Unexpected call to DeleteIfExists - DeleteIfExists(%q, %q, %q)", table, chain, rulespec))
	call := m.deleteIfExists[0]
	m.deleteIfExists = m.deleteIfExists[1:]
	received := DeleteIfExists{
		Table:    table,
		Chain:    chain,
		RuleSpec: rulespec,
		Err:      call.Err,
	}
	must.Eq(m.t, call, received,
		must.Sprint("DeleteIfExists received incorrect arguments"))

	return call.Err
}

func (m *mockIPTables) Insert(table, chain string, pos int, rulespec ...string) error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.inserts,
		must.Sprintf("Unexpected call to Insert - Insert(%q, %q, %q, %q)", table, chain, pos, rulespec))
	call := m.inserts[0]
	m.inserts = m.inserts[1:]
	received := Insert{
		Table:    table,
		Chain:    chain,
		Pos:      pos,
		RuleSpec: rulespec,
		Err:      call.Err,
	}
	must.Eq(m.t, call, received,
		must.Sprint("Insert received incorrect arguments"))

	return call.Err
}

func (m *mockIPTables) ListChains(table string) ([]string, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.listChains,
		must.Sprintf("Unexpected call to ListChains - ListChains(%q)", table))
	call := m.listChains[0]
	m.listChains = m.listChains[1:]
	must.Eq(m.t, call.Table, table,
		must.Sprint("ListChains received incorrect arguments"))

	return call.Result, call.Err
}

func (m *mockIPTables) NewChain(table, chain string) error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.newChains,
		must.Sprintf("Unexpected call to NewChain - NewChain(%q, %q)", table, chain))
	call := m.newChains[0]
	m.newChains = m.newChains[1:]
	received := NewChain{
		Table: table,
		Chain: chain,
		Err:   call.Err,
	}
	must.Eq(m.t, call, received,
		must.Sprint("NewChain received incorrect arguments"))

	return call.Err
}

// AssertExpectations verifies that all expected invocations
// have been called.
func (m *mockIPTables) AssertExpectations() {
	m.t.Helper()

	must.SliceEmpty(m.t, m.appends,
		must.Sprintf("Append expecting %d more invocations", len(m.appends)))
	must.SliceEmpty(m.t, m.clearChains,
		must.Sprintf("ClearChain expecting %d more invocations", len(m.clearChains)))
	must.SliceEmpty(m.t, m.deletes,
		must.Sprintf("Delete expecting %d more invocations", len(m.deletes)))
	must.SliceEmpty(m.t, m.deleteChains,
		must.Sprintf("DeleteChain expecting %d more invocations", len(m.deleteChains)))
	must.SliceEmpty(m.t, m.deleteIfExists,
		must.Sprintf("DeleteIfExists expecting %d more invocations", len(m.deleteIfExists)))
	must.SliceEmpty(m.t, m.inserts,
		must.Sprintf("Insert expecting %d more invocations", len(m.inserts)))
	must.SliceEmpty(m.t, m.listChains,
		must.Sprintf("ListChains expecting %d more invocations", len(m.listChains)))
	must.SliceEmpty(m.t, m.newChains,
		must.Sprintf("NewChain expecting %d more invocations", len(m.newChains)))
}

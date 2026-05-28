// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"testing"

	"github.com/hashicorp/nomad-driver-virt/testutil/mock"
	"github.com/shoenig/test/must"
)

func TestIPTables_Append(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectAppend(Append{
			Table:    "default",
			Chain:    "default",
			RuleSpec: []string{"RULE1"},
		})

		err := iptables.Append("default", "default", "RULE1")
		must.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectAppend(Append{
			Table:    "default",
			Chain:    "default",
			RuleSpec: []string{"RULE1"},
			Err:      mock.MockTestErr,
		})

		err := iptables.Append("default", "default", "RULE1")
		must.ErrorIs(t, err, mock.MockTestErr)
	})

	t.Run("incorrect arguments", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectAppend(Append{
			Table:    "default",
			Chain:    "non-default",
			RuleSpec: []string{"RULE1"},
		})
		defer mock.AssertIncorrectArguments(t, "Append")

		iptables.Append("default", "default")
	})

	t.Run("unexpected", func(t *testing.T) {
		iptables := New(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "Append")

		iptables.Append("default", "default")
	})
}

func TestIPTables_AppendUnique(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectAppendUnique(AppendUnique{
			Table:    "default",
			Chain:    "default",
			RuleSpec: []string{"RULE1"},
		})

		err := iptables.AppendUnique("default", "default", "RULE1")
		must.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectAppendUnique(AppendUnique{
			Table:    "default",
			Chain:    "default",
			RuleSpec: []string{"RULE1"},
			Err:      mock.MockTestErr,
		})

		err := iptables.AppendUnique("default", "default", "RULE1")
		must.ErrorIs(t, err, mock.MockTestErr)
	})

	t.Run("incorrect arguments", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectAppendUnique(AppendUnique{
			Table:    "default",
			Chain:    "non-default",
			RuleSpec: []string{"RULE1"},
		})
		defer mock.AssertIncorrectArguments(t, "AppendUnique")

		iptables.AppendUnique("default", "default")
	})

	t.Run("unexpected", func(t *testing.T) {
		iptables := New(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "AppendUnique")

		iptables.AppendUnique("default", "default")
	})
}

func TestIPTables_ChainExists(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectChainExists(ChainExists{
			Table:  "default",
			Chain:  "default",
			Result: true,
		})

		result, err := iptables.ChainExists("default", "default")
		must.True(t, result)
		must.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectChainExists(ChainExists{
			Table: "default",
			Chain: "default",
			Err:   mock.MockTestErr,
		})

		_, err := iptables.ChainExists("default", "default")
		must.ErrorIs(t, err, mock.MockTestErr)
	})

	t.Run("incorrect arguments", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectChainExists(ChainExists{
			Table: "default",
			Chain: "default",
		})
		defer mock.AssertIncorrectArguments(t, "ChainExists")

		iptables.ChainExists("default", "non-default")
	})

	t.Run("unexpected", func(t *testing.T) {
		iptables := New(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "ChainExists")

		iptables.ChainExists("default", "default")
	})
}

func TestIPTables_ClearAndDeleteChain(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectClearAndDeleteChain(ClearAndDeleteChain{
			Table: "default",
			Chain: "default",
		})

		err := iptables.ClearAndDeleteChain("default", "default")
		must.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectClearAndDeleteChain(ClearAndDeleteChain{
			Table: "default",
			Chain: "default",
			Err:   mock.MockTestErr,
		})

		err := iptables.ClearAndDeleteChain("default", "default")
		must.ErrorIs(t, err, mock.MockTestErr)
	})

	t.Run("incorrect arguments", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectClearAndDeleteChain(ClearAndDeleteChain{
			Table: "default",
			Chain: "default",
		})
		defer mock.AssertIncorrectArguments(t, "ClearAndDeleteChain")

		iptables.ClearAndDeleteChain("default", "non-default")
	})

	t.Run("unexpected", func(t *testing.T) {
		iptables := New(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "ClearAndDeleteChain")

		iptables.ClearAndDeleteChain("default", "default")
	})
}

func TestIPTables_ClearChain(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectClearChain(ClearChain{
			Table: "default",
			Chain: "default",
		})

		err := iptables.ClearChain("default", "default")
		must.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectClearChain(ClearChain{
			Table: "default",
			Chain: "default",
			Err:   mock.MockTestErr,
		})

		err := iptables.ClearChain("default", "default")
		must.ErrorIs(t, err, mock.MockTestErr)
	})

	t.Run("incorrect arguments", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectClearChain(ClearChain{
			Table: "default",
			Chain: "default",
		})
		defer mock.AssertIncorrectArguments(t, "ClearChain")

		iptables.ClearChain("default", "non-default")
	})

	t.Run("unexpected", func(t *testing.T) {
		iptables := New(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "ClearChain")

		iptables.ClearChain("default", "default")
	})
}

func TestIPTables_Delete(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectDelete(Delete{
			Table:    "default",
			Chain:    "default",
			RuleSpec: []string{"RULE1"},
		})

		err := iptables.Delete("default", "default", "RULE1")
		must.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectDelete(Delete{
			Table:    "default",
			Chain:    "default",
			RuleSpec: []string{"RULE1"},
			Err:      mock.MockTestErr,
		})

		err := iptables.Delete("default", "default", "RULE1")
		must.ErrorIs(t, err, mock.MockTestErr)
	})

	t.Run("incorrect arguments", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectDelete(Delete{
			Table:    "default",
			Chain:    "default",
			RuleSpec: []string{"RULE1"},
		})
		defer mock.AssertIncorrectArguments(t, "Delete")

		iptables.Delete("default", "default", "RULE2")
	})

	t.Run("unexpected", func(t *testing.T) {
		iptables := New(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "Delete")

		iptables.Delete("default", "default", "RULE1")
	})
}

func TestIPTables_DeleteChain(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectDeleteChain(DeleteChain{
			Table: "default",
			Chain: "default",
		})

		err := iptables.DeleteChain("default", "default")
		must.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectDeleteChain(DeleteChain{
			Table: "default",
			Chain: "default",
			Err:   mock.MockTestErr,
		})

		err := iptables.DeleteChain("default", "default")
		must.ErrorIs(t, err, mock.MockTestErr)
	})

	t.Run("incorrect arguments", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectDeleteChain(DeleteChain{
			Table: "default",
			Chain: "default",
		})
		defer mock.AssertIncorrectArguments(t, "DeleteChain")

		iptables.DeleteChain("default", "non-default")
	})

	t.Run("unexpected", func(t *testing.T) {
		iptables := New(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "DeleteChain")

		iptables.DeleteChain("default", "default")
	})
}

func TestIPTables_DeleteIfExists(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectDeleteIfExists(DeleteIfExists{
			Table:    "default",
			Chain:    "default",
			RuleSpec: []string{"RULE1"},
		})

		err := iptables.DeleteIfExists("default", "default", "RULE1")
		must.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectDeleteIfExists(DeleteIfExists{
			Table: "default",
			Chain: "default",
			Err:   mock.MockTestErr,
		})

		err := iptables.DeleteIfExists("default", "default")
		must.ErrorIs(t, err, mock.MockTestErr)
	})

	t.Run("incorrect arguments", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectDeleteIfExists(DeleteIfExists{
			Table: "default",
			Chain: "default",
		})
		defer mock.AssertIncorrectArguments(t, "DeleteIfExists")

		iptables.DeleteIfExists("default", "non-default")
	})

	t.Run("unexpected", func(t *testing.T) {
		iptables := New(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "DeleteIfExists")

		iptables.DeleteIfExists("default", "default")
	})
}

func TestIPTables_Insert(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectInsert(Insert{
			Table:    "default",
			Chain:    "default",
			Pos:      1,
			RuleSpec: []string{"RULE1"},
		})

		err := iptables.Insert("default", "default", 1, "RULE1")
		must.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectInsert(Insert{
			Table:    "default",
			Chain:    "default",
			Pos:      1,
			RuleSpec: []string{"RULE1"},
			Err:      mock.MockTestErr,
		})

		err := iptables.Insert("default", "default", 1, "RULE1")
		must.ErrorIs(t, err, mock.MockTestErr)
	})

	t.Run("incorrect arguments", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectInsert(Insert{
			Table:    "default",
			Chain:    "default",
			Pos:      1,
			RuleSpec: []string{"RULE1"},
		})
		defer mock.AssertIncorrectArguments(t, "Insert")

		iptables.Insert("default", "non-default", 1, "RULE1")
	})

	t.Run("unexpected", func(t *testing.T) {
		iptables := New(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "Insert")

		iptables.Insert("default", "non-default", 1, "RULE1")
	})
}

func TestIPTables_InsertUnique(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectInsertUnique(InsertUnique{
			Table:    "default",
			Chain:    "default",
			Pos:      1,
			RuleSpec: []string{"RULE1"},
		})

		err := iptables.InsertUnique("default", "default", 1, "RULE1")
		must.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectInsertUnique(InsertUnique{
			Table:    "default",
			Chain:    "default",
			Pos:      1,
			RuleSpec: []string{"RULE1"},
			Err:      mock.MockTestErr,
		})

		err := iptables.InsertUnique("default", "default", 1, "RULE1")
		must.ErrorIs(t, err, mock.MockTestErr)
	})

	t.Run("incorrect arguments", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectInsertUnique(InsertUnique{
			Table:    "default",
			Chain:    "default",
			Pos:      1,
			RuleSpec: []string{"RULE1"},
		})
		defer mock.AssertIncorrectArguments(t, "InsertUnique")

		iptables.InsertUnique("default", "non-default", 1, "RULE1")
	})

	t.Run("unexpected", func(t *testing.T) {
		iptables := New(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "InsertUnique")

		iptables.InsertUnique("default", "non-default", 1, "RULE1")
	})
}

func TestIPTables_ListChains(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectListChains(ListChains{
			Table: "default",
		})

		result, err := iptables.ListChains("default")
		must.NoError(t, err)
		must.SliceEmpty(t, result)
	})

	t.Run("error", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectListChains(ListChains{
			Table: "default",
			Err:   mock.MockTestErr,
		})

		result, err := iptables.ListChains("default")
		must.ErrorIs(t, err, mock.MockTestErr)
		must.SliceEmpty(t, result)
	})

	t.Run("incorrect arguments", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectListChains(ListChains{
			Table: "default",
		})
		defer mock.AssertIncorrectArguments(t, "ListChains")

		iptables.ListChains("non-default")
	})

	t.Run("unexpected", func(t *testing.T) {
		iptables := New(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "ListChains")

		iptables.ListChains("default")
	})
}

func TestIPTables_NewChain(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectNewChain(NewChain{
			Table: "default",
			Chain: "default",
		})

		err := iptables.NewChain("default", "default")
		must.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		iptables := New(t)
		iptables.ExpectNewChain(NewChain{
			Table: "default",
			Chain: "default",
			Err:   mock.MockTestErr,
		})

		err := iptables.NewChain("default", "default")
		must.ErrorIs(t, err, mock.MockTestErr)
	})

	t.Run("incorrect arguments", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectNewChain(NewChain{
			Table: "default",
			Chain: "default",
		})
		defer mock.AssertIncorrectArguments(t, "NewChain")

		iptables.NewChain("default", "non-default")
	})

	t.Run("unexpected", func(t *testing.T) {
		iptables := New(mock.MockT())
		defer mock.AssertUnexpectedCall(t, "NewChain")

		iptables.NewChain("default", "default")
	})
}

func TestIPTables_AssertExpectations(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		iptables := New(t)
		iptables.AssertExpectations()
	})

	t.Run("missing Append", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectAppend(Append{})
		defer mock.AssertExpectations(t, "Append")

		iptables.AssertExpectations()
	})

	t.Run("missing ClearChain", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectClearChain(ClearChain{})
		defer mock.AssertExpectations(t, "ClearChain")

		iptables.AssertExpectations()
	})

	t.Run("missing Delete", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectDelete(Delete{})
		defer mock.AssertExpectations(t, "Delete")

		iptables.AssertExpectations()
	})

	t.Run("missing DeleteChain", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectDeleteChain(DeleteChain{})
		defer mock.AssertExpectations(t, "DeleteChain")

		iptables.AssertExpectations()
	})

	t.Run("missing DeleteIfExists", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectDeleteIfExists(DeleteIfExists{})
		defer mock.AssertExpectations(t, "DeleteIfExists")

		iptables.AssertExpectations()
	})

	t.Run("missing Insert", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectInsert(Insert{})
		defer mock.AssertExpectations(t, "Insert")

		iptables.AssertExpectations()
	})

	t.Run("missing ListChains", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectListChains(ListChains{})
		defer mock.AssertExpectations(t, "ListChains")

		iptables.AssertExpectations()
	})

	t.Run("missing NewChain", func(t *testing.T) {
		iptables := New(mock.MockT())
		iptables.ExpectNewChain(NewChain{})
		defer mock.AssertExpectations(t, "NewChain")

		iptables.AssertExpectations()
	})
}

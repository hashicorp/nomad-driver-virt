// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"testing"

	"github.com/shoenig/test/must"
)

func Test_taskStore(t *testing.T) {

	testStore := newTaskStore()
	must.NotNil(t, testStore)
	must.NotNil(t, testStore.store)

	// The store is empty, so a get should yield no result without panicking.
	nonExistentHandle, nonExistentBool := testStore.Get("please")
	must.Nil(t, nonExistentHandle)
	must.False(t, nonExistentBool)

	// Set a couple of entries into the store.
	testStore.Set("task_id_1", &taskHandle{name: "task_id_1"})
	testStore.Set("task_id_2", &taskHandle{name: "task_id_2"})

	// Read both back out, ensuring this works.
	task1Handle, task1Bool := testStore.Get("task_id_1")
	must.True(t, task1Bool)
	must.NotNil(t, task1Handle)
	must.Eq(t, "task_id_1", task1Handle.name)

	task2Handle, task2Bool := testStore.Get("task_id_2")
	must.True(t, task2Bool)
	must.NotNil(t, task2Handle)
	must.Eq(t, "task_id_2", task2Handle.name)

	// Delete both, then ensure the map is empty.
	testStore.Delete("task_id_1")
	testStore.Delete("task_id_2")
	must.MapEmpty(t, testStore.store)

	// Ensure deletes of entries that do not exist do not cause adverse
	// behaviour.
	testStore.Delete("task_id_1")
	testStore.Delete("task_id_2")
	testStore.Delete("please")
}

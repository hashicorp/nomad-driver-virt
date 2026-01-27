// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package plugin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad/client/structs"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/shoenig/test/must"
)

func Test_GetStats(t *testing.T) {
	mockError := errors.New("oh no!")

	tests := []struct {
		name           string
		expectedError  error
		getterError    error
		info           *vm.Info
		expectedResult *drivers.TaskResourceUsage
	}{
		{
			name: "successful_stats_returned",
			info: &vm.Info{
				State:     vm.VMStateRunning,
				Memory:    666,
				CPUTime:   66,
				MaxMemory: 6666,
				NrVirtCPU: 6,
			},
			expectedResult: &drivers.TaskResourceUsage{
				ResourceUsage: &structs.ResourceUsage{
					MemoryStats: &structs.MemoryStats{Usage: 666, MaxUsage: 6666},
					CpuStats:    &structs.CpuStats{ThrottledTime: 66},
				},
			},
		},
		{
			name:          "getter_error_propagation",
			expectedError: mockError,
			getterError:   mockError,
		},
		{
			name:          "task_not_found_error",
			expectedError: drivers.ErrTaskNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dgm := &mockTaskGetter{
				err:  tt.getterError,
				info: tt.info,
			}

			th := &taskHandle{
				name:       "test-vm",
				taskGetter: dgm,
			}

			stats, err := th.GetStats()
			must.Eq(t, tt.expectedError, errors.Unwrap(err))
			if err == nil {
				dgm.lock.Lock()
				must.StrContains(t, "test-vm", dgm.name)
				dgm.lock.Unlock()

				must.Eq(t, tt.expectedResult.ResourceUsage, stats.ResourceUsage)
			}
		})
	}
}

func Test_Monitor(t *testing.T) {
	dgm := &mockTaskGetter{
		info: &vm.Info{
			State: vm.VMStateRunning,
		},
	}

	th := &taskHandle{
		logger:     hclog.NewNullLogger(),
		name:       "test-vm",
		taskGetter: dgm,
		procState:  drivers.TaskStateRunning,
	}

	exitChannel := make(chan *drivers.ExitResult, 1)
	defer close(exitChannel)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go th.monitor(ctx, exitChannel)

	time.Sleep(2 * time.Second)

	must.Zero(t, len(exitChannel))

	th.stateLock.Lock()
	must.Eq(t, drivers.TaskStateRunning, th.procState)
	th.stateLock.Unlock()

	// An error from the vm getter should cause the task to move
	// to an unknown state.
	dgm.lock.Lock()
	dgm.err = errors.New("oh no! an error!")
	dgm.lock.Unlock()

	time.Sleep(2 * time.Second)

	th.stateLock.Lock()
	must.Eq(t, drivers.TaskStateUnknown, th.procState)
	th.stateLock.Unlock()

	// A vm reporting a crash should force the monitor to send an exit
	// result and return.
	dgm.lock.Lock()
	dgm.err = nil
	dgm.info.State = vm.VMStateError
	dgm.lock.Unlock()

	time.Sleep(2 * time.Second)

	must.One(t, len(exitChannel))

	res := <-exitChannel

	must.One(t, res.ExitCode)
	must.Eq(t, ErrTaskCrashed, res.Err)

	th.stateLock.Lock()
	must.Eq(t, drivers.TaskStateExited, th.procState)
	th.stateLock.Unlock()
}

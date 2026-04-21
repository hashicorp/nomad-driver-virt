// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package plugin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	mock_virt "github.com/hashicorp/nomad-driver-virt/testutil/mock/virt"
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
		info           mock_virt.GetVM
		expectedResult *drivers.TaskResourceUsage
	}{
		{
			name: "successful_stats_returned",
			info: mock_virt.GetVM{
				Name: "test-vm",
				Result: &vm.Info{
					State:     vm.VMStateRunning,
					Memory:    666,
					CPUTime:   66,
					MaxMemory: 6666,
					NrVirtCPU: 6,
				},
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
			info:          mock_virt.GetVM{Name: "test-vm", Err: mockError},
		},
		{
			name:          "task_not_found_error",
			info:          mock_virt.GetVM{Name: "test-vm", Err: errs.ErrNotFound},
			expectedError: drivers.ErrTaskNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dgm := mock_virt.NewMock(t)
			dgm.Expect(tt.info)

			th := &taskHandle{
				name:       "test-vm",
				taskGetter: dgm,
			}

			stats, err := th.GetStats()
			must.Eq(t, tt.expectedError, errors.Unwrap(err))
			if err == nil {
				must.Eq(t, tt.expectedResult.ResourceUsage, stats.ResourceUsage)
			}
		})
	}
}

func Test_Monitor(t *testing.T) {
	errTest := errors.New("testing error")

	dgm := mock_virt.NewMock(t)
	dgm.Expect(
		mock_virt.GetVM{Name: "test-vm", Result: &vm.Info{State: vm.VMStateRunning}},
		mock_virt.GetVM{Name: "test-vm", Result: &vm.Info{State: vm.VMStateRunning}},
		mock_virt.GetVM{Name: "test-vm", Err: errTest},
		mock_virt.GetVM{Name: "test-vm", Result: &vm.Info{State: vm.VMStateError}},
	)

	th := &taskHandle{
		logger:     hclog.NewNullLogger(),
		name:       "test-vm",
		taskGetter: dgm,
		procState:  drivers.TaskStateRunning,
	}

	// Start monitoring
	exitChannel := make(chan *drivers.ExitResult, 1)
	defer close(exitChannel)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go th.monitor(ctx, 50*time.Millisecond, exitChannel)

	// Allow for two check to occur
	time.Sleep(110 * time.Millisecond)

	// No exit should be detected yet
	must.Zero(t, len(exitChannel))

	// Check the task handle state
	th.stateLock.Lock()
	must.Eq(t, drivers.TaskStateRunning, th.procState)
	th.stateLock.Unlock()

	// Wait for next check
	time.Sleep(50 * time.Millisecond)

	// An error should be returned so state should be unknown
	th.stateLock.Lock()
	must.Eq(t, drivers.TaskStateUnknown, th.procState)
	th.stateLock.Unlock()

	// Wait for the next check
	time.Sleep(50 * time.Millisecond)

	// Error state should force the monitor to send exit and return
	must.One(t, len(exitChannel))
	res := <-exitChannel

	must.One(t, res.ExitCode)
	must.Eq(t, ErrTaskCrashed, res.Err)

	th.stateLock.Lock()
	must.Eq(t, drivers.TaskStateExited, th.procState)
	th.stateLock.Unlock()
}

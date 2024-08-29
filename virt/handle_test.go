// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/go-hclog"
	domain "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad/client/structs"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/shoenig/test/must"
)

type domainGetterMock struct {
	name string
	info *domain.Info
	err  error
}

func (dgm *domainGetterMock) GetDomain(name string) (*domain.Info, error) {
	dgm.name = name
	return dgm.info, dgm.err
}

func Test_GetStats(t *testing.T) {
	mockError := errors.New("oh no!")

	tests := []struct {
		name           string
		expectedError  error
		getterError    error
		info           *domain.Info
		expectedResult *drivers.TaskResourceUsage
	}{
		{
			name: "successful_stats_returned",
			info: &domain.Info{
				State:     "running",
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
			dgm := &domainGetterMock{
				err:  tt.getterError,
				info: tt.info,
			}

			th := &taskHandle{
				name:       "test-domain",
				taskGetter: dgm,
			}

			stats, err := th.GetStats()
			must.Eq(t, tt.expectedError, errors.Unwrap(err))
			if err == nil {
				must.StrContains(t, "test-domain", dgm.name)
				must.Eq(t, tt.expectedResult.ResourceUsage, stats.ResourceUsage)
			}
		})
	}
}

func Test_Monitor(t *testing.T) {
	dgm := &domainGetterMock{
		info: &domain.Info{
			State: "running",
		},
	}

	th := &taskHandle{
		logger:     hclog.NewNullLogger(),
		name:       "test-domain",
		taskGetter: dgm,
		procState:  drivers.TaskStateRunning,
	}

	exitChannel := make(chan *drivers.ExitResult, 1)
	defer close(exitChannel)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go th.monitor(ctx, exitChannel)

	time.Sleep(2 * time.Second)

	must.Zero(t, len(exitChannel))
	must.Eq(t, drivers.TaskStateRunning, th.procState)

	// An error from the domain getter should cause the task to move
	// to an unknown state.
	dgm.err = errors.New("oh no! an error!")
	time.Sleep(2 * time.Second)

	must.Eq(t, drivers.TaskStateUnknown, th.procState)

	// A domain reporting a crash should force the monitor to send an exit
	// result and return.
	dgm.err = nil
	dgm.info.State = "crashed"

	time.Sleep(2 * time.Second)

	must.One(t, len(exitChannel))

	res := <-exitChannel

	must.One(t, res.ExitCode)
	must.Eq(t, ErrTaskCrashed, res.Err)
	must.Eq(t, drivers.TaskStateExited, th.procState)
}

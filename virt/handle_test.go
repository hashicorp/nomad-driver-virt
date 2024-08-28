// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"errors"
	"testing"

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

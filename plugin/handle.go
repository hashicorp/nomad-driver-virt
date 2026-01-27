// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package plugin

import (
	"context"
	"fmt"
	"sync"
	"time"

	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/virt"
	"github.com/hashicorp/nomad-driver-virt/virt/net"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/client/structs"
	"github.com/hashicorp/nomad/plugins/drivers"
)

var (
	defaultMonitorInterval = time.Second
	defaultStatsInterval   = time.Second
)

// taskHandle should store all relevant runtime information
// such as process ID if this is a local task or other meta
// data if this driver deals with external APIs
type taskHandle struct {
	// stateLock syncs access to procState and procStats
	stateLock sync.RWMutex

	logger      hclog.Logger
	taskConfig  *drivers.TaskConfig
	procState   drivers.TaskState
	startedAt   time.Time
	completedAt time.Time
	name        string
	exitResult  *drivers.ExitResult

	taskGetter virt.VMGetter

	// netTeardown is the specification used to delete all the network
	// configuration associated to a VM.
	netTeardown *net.TeardownSpec
}

func (h *taskHandle) TaskStatus() *drivers.TaskStatus {
	h.stateLock.RLock()
	defer h.stateLock.RUnlock()

	return &drivers.TaskStatus{
		ID:               h.taskConfig.ID,
		Name:             h.taskConfig.Name,
		State:            h.procState,
		StartedAt:        h.startedAt,
		CompletedAt:      h.completedAt,
		DriverAttributes: map[string]string{},
		ExitResult:       h.exitResult.Copy(),
	}
}

func (h *taskHandle) GetStats() (*drivers.TaskResourceUsage, error) {
	virtvm, err := h.taskGetter.GetVM(h.name)
	if err != nil {
		return nil, fmt.Errorf("virt: unable to get task %s stats: %w", h.name, err)
	}

	if virtvm == nil {
		return nil, fmt.Errorf("virt: task not found %s: %w", h.name, drivers.ErrTaskNotFound)
	}

	return fillStats(virtvm), nil
}

func (h *taskHandle) IsRunning() bool {
	h.stateLock.RLock()
	defer h.stateLock.RUnlock()
	return h.procState == drivers.TaskStateRunning
}

// Run is in charge of monitoring and updating the task status. It  will only return
// when the task is stopped or no longer present or when the context is cancelled.
func (h *taskHandle) monitor(ctx context.Context, exitCh chan<- *drivers.ExitResult) {

	ticker := time.NewTicker(defaultMonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			virtvm, err := h.taskGetter.GetVM(h.name)
			if err != nil {
				h.logger.Error("virt: unable to get task state", "task", h.name, "error", err)
				h.stateLock.Lock()
				h.procState = drivers.TaskStateUnknown
				h.stateLock.Unlock()

				continue
			}

			if virtvm == nil || virtvm.State != vm.VMStateRunning {
				er := fillExitResult(virtvm)

				h.stateLock.Lock()
				h.procState = drivers.TaskStateExited
				h.completedAt = time.Now()
				h.exitResult = er
				h.stateLock.Unlock()

				exitCh <- er
				return
			}

		case <-ctx.Done():
			return
		}
	}
}

func fillExitResult(info *vm.Info) *drivers.ExitResult {
	er := &drivers.ExitResult{}

	if info == nil {
		er.Err = drivers.ErrTaskNotFound
		er.ExitCode = 1
		return er
	}

	switch info.State {
	case vm.VMStateError:
		er.ExitCode = 1
		er.Err = ErrTaskCrashed
	case vm.VMStateShutdown, vm.VMStatePowerOff:
		er.ExitCode = 0
	default:
		er.ExitCode = 1
		er.Err = fmt.Errorf("unexpected state: %s (%s)", info.State, info.RawState)
	}

	return er
}

func fillStats(info *vm.Info) *structs.TaskResourceUsage {
	return &structs.TaskResourceUsage{
		Timestamp: time.Now().UnixNano(),
		ResourceUsage: &structs.ResourceUsage{
			MemoryStats: &structs.MemoryStats{
				Usage:    info.Memory,
				MaxUsage: info.MaxMemory,
			},
			CpuStats: &structs.CpuStats{
				ThrottledTime: info.CPUTime,
			},
		},
	}
}

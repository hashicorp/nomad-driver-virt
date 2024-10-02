// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	"context"
	"fmt"
	"sync"
	"time"

	domain "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/libvirt"
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

	taskGetter DomainGetter

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
	domain, err := h.taskGetter.GetDomain(h.name)
	if err != nil {
		return nil, fmt.Errorf("virt: unable to get task %s stats: %w", h.name, err)
	}

	if domain == nil {
		return nil, fmt.Errorf("virt: task not found %s: %w", h.name, drivers.ErrTaskNotFound)
	}

	return fillStats(domain), nil
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
			domain, err := h.taskGetter.GetDomain(h.name)
			if err != nil {
				h.logger.Error("virt: unable to get task's %s state: %w", h.name, err)
				h.stateLock.Lock()
				h.procState = drivers.TaskStateUnknown
				h.stateLock.Unlock()

				continue
			}

			if domain == nil || domain.State != libvirt.DomainRunning {
				er := fillExitResult(domain)

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

func fillExitResult(info *domain.Info) *drivers.ExitResult {
	er := &drivers.ExitResult{}

	if info == nil {
		er.Err = drivers.ErrTaskNotFound
		er.ExitCode = 1
		return er
	}

	switch info.State {
	case libvirt.DomainCrashed:
		er.ExitCode = 1
		er.Err = ErrTaskCrashed
	case libvirt.DomainShutdown, libvirt.DomainShutOff:
		er.ExitCode = 0
	default:
		er.ExitCode = 1
		er.Err = fmt.Errorf("unexpected state: %s", info.State)
	}

	return er
}

func fillStats(info *domain.Info) *structs.TaskResourceUsage {
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

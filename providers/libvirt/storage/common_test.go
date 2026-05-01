// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	mock_libvirt_storage "github.com/hashicorp/nomad-driver-virt/testutil/mock/providers/libvirt/storage"
	"github.com/shoenig/test/must"
	"libvirt.org/go/libvirt"
)

func Test_refreshPool(t *testing.T) {
	testErr := &libvirt.Error{
		Code:    libvirt.ErrorNumber(libvirt.ERR_INTERNAL_ERROR),
		Message: "internal error: pool 'test' has asynchronous jobs running.",
	}

	t.Run("ok", func(t *testing.T) {
		pool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
			mock_libvirt_storage.Refresh{},
		)
		defer pool.AssertExpectations()

		must.NoError(t, refreshPool(t.Context(), pool, time.Millisecond, time.Millisecond))
	})

	t.Run("retries", func(t *testing.T) {
		pool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
			mock_libvirt_storage.Refresh{Err: testErr},
			mock_libvirt_storage.Refresh{Err: testErr},
			mock_libvirt_storage.Refresh{Err: testErr},
			mock_libvirt_storage.Refresh{},
		)
		defer pool.AssertExpectations()

		must.NoError(t, refreshPool(t.Context(), pool, 1*time.Millisecond, 5*time.Millisecond))
	})

	t.Run("retries timeout", func(t *testing.T) {
		pool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
			mock_libvirt_storage.Refresh{Err: testErr},
			mock_libvirt_storage.Refresh{Err: testErr},
			mock_libvirt_storage.Refresh{Err: testErr},
		)
		// NOTE: Do not assert pool expectations since we don't care
		// if all the refreshes are called.

		must.ErrorIs(t, refreshPool(t.Context(), pool, 1*time.Millisecond, 2*time.Millisecond), context.DeadlineExceeded)
	})

	t.Run("other error", func(t *testing.T) {
		customErr := errors.New("custom test error")

		pool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
			mock_libvirt_storage.Refresh{Err: testErr},
			mock_libvirt_storage.Refresh{Err: customErr},
		)
		defer pool.AssertExpectations()

		must.ErrorIs(t, refreshPool(t.Context(), pool, 1*time.Millisecond, 5*time.Millisecond), customErr)
	})

	t.Run("canceled context", func(t *testing.T) {
		pool := mock_libvirt_storage.NewMockStoragePool(t).Expect(
			mock_libvirt_storage.Refresh{Err: testErr},
			mock_libvirt_storage.Refresh{Err: testErr},
			mock_libvirt_storage.Refresh{Err: testErr},
		)
		// NOTE: Do not assert pool expectations since we don't care
		// if all the refreshes are called.

		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		timer := time.AfterFunc(2*time.Millisecond, func() { cancel() })
		defer timer.Stop()

		must.ErrorIs(t, refreshPool(ctx, pool, 1*time.Millisecond, 5*time.Millisecond), context.Canceled)
	})
}

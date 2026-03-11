// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"context"
	"errors"
	"os"

	"github.com/hashicorp/go-hclog"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"libvirt.org/go/libvirtxml"
)

const defaultDirectoryImageFormat = "qcow2"

// directory provides a local directory based implementation
// of a storage pool.
type directory struct {
	*pool
}

// newDirectoryPool creates a new directory based storage pool.
func newDirectoryPool(ctx context.Context, logger hclog.Logger, l libvirtStorage, poolName string, config storage.Directory, s storage.Storage) (*directory, error) {
	logger = logger.Named("storage-pool").With("name", poolName)
	p, err := l.FindStoragePool(poolName)
	if err != nil && !errors.Is(err, vm.ErrNotFound) {
		return nil, err
	}

	// Be sure to free the pool instance before leaving
	// if it was found or created.
	defer func() {
		if p != nil {
			p.Free()
		}
	}()

	if p == nil {
		// Make sure the directory exists before setting up the pool
		if err := os.MkdirAll(config.Path, 0755); err != nil {
			return nil, err
		}

		if p, err = l.CreateStoragePool(&libvirtxml.StoragePool{
			Name: poolName,
			Target: &libvirtxml.StoragePoolTarget{
				Path: config.Path,
			},
			Type: "dir",
		}); err != nil {
			return nil, err
		}
	}

	return &directory{pool: &pool{ctx: ctx, logger: logger, name: poolName, l: l, s: s}}, nil
}

// Type implements storage.Pool
func (d *directory) Type() string {
	return storage.PoolTypeDirectory
}

// DefaultImageFormat implements storage.Pool
func (d *directory) DefaultImageFormat() string {
	return defaultDirectoryImageFormat
}

// copy creates a new copy of this pool with updated context
// and storage.
func (d *directory) copy(ctx context.Context, s *Storage, l libvirtStorage) *directory {
	return &directory{pool: d.pool.copy(ctx, s, l)}
}

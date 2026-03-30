// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"github.com/hashicorp/nomad-driver-virt/virt/disks"
	"libvirt.org/go/libvirtxml"
)

const defaultDirectoryImageFormat = disks.DiskFormatQcow2

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

// ValidateDisk validates the provided disk and returns any configuration errors found.
// implements disks.DiskValidator
func (d *directory) ValidateDisk(disk *disks.Disk) error {
	var mErr *multierror.Error

	// Directory pool currently supports qcow2 and raw volumes
	if disk.Format != disks.DiskFormatQcow2 && disk.Format != disks.DiskFormatRaw {
		mErr = multierror.Append(mErr,
			fmt.Errorf("%w: format only supports raw or qcow2 for directory volumes", disks.ErrInvalidConfiguration))
	}

	// Disk chaining is only supported with qcow2 images.
	if disk.Format != disks.DiskFormatQcow2 && disk.Chained {
		mErr = multierror.Append(mErr,
			fmt.Errorf("%w - format must be qcow2 if chained enabled for directory volumes", disks.ErrInvalidConfiguration))
	}

	if mErr != nil {
		return mErr
	}

	return nil
}

// Type returns the type of the storage pool.
// implements storage.Pool
func (d *directory) Type() string {
	return storage.PoolTypeDirectory
}

// DefaultImageFormat returns the default image format for the pool.
// implements storage.Pool
func (d *directory) DefaultImageFormat() string {
	return defaultDirectoryImageFormat
}

// copy creates a new copy of this pool with updated context
// and storage.
func (d *directory) copy(ctx context.Context, s *Storage, l libvirtStorage) *directory {
	return &directory{pool: d.pool.copy(ctx, s, l)}
}

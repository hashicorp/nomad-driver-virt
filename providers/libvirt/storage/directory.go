// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"errors"
	"fmt"
	"os"

	"github.com/hashicorp/go-hclog"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

const defaultDirectoryImageFormat = "qcow2"

// directory provides a local directory based implementation
// of a storage pool.
type directory struct {
	poolName string
	logger   hclog.Logger
	l        libvirtStorage
	s        storage.Storage
}

// newDirectoryPool creates a new directory based storage pool.
func newDirectoryPool(logger hclog.Logger, l libvirtStorage, poolName string, config storage.Directory, s storage.Storage) (storage.Pool, error) {
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

		if p, err = l.CreateStoragePool(libvirtxml.StoragePool{
			Name: poolName,
			Target: &libvirtxml.StoragePoolTarget{
				Path: config.Path,
			},
			Type: "dir",
		}); err != nil {
			return nil, err
		}
	}

	return &directory{logger: logger, poolName: poolName, l: l, s: s}, nil
}

// Name implements storage.Pool
func (d *directory) Name() string {
	return d.poolName
}

// Type implements storage.Pool
func (d *directory) Type() string {
	return storage.PoolTypeDirectory
}

// AddVolume implements storage.Pool
func (d *directory) AddVolume(name string, opts storage.Options) (*storage.Volume, error) {
	// The directory pool does not support cloning volumes or snapshots
	if opts.Source.Volume != "" || opts.Source.Snapshot != "" {
		return nil, fmt.Errorf("cannot clone volumes or snapshots - %w", vm.ErrNotSupported)
	}

	src := opts.Source.Path
	d.logger.Debug("adding new volume", "source", src, "name", name, "options", hclog.Fmt("%#v", opts))

	pool, err := d.l.FindStoragePool(d.poolName)
	if err != nil {
		return nil, err
	}
	defer pool.Free()

	// First check if the volume already exists
	volume, err := findVolume(pool, name)
	if err == nil {
		return volume, nil
	}

	if !errors.Is(err, ErrVolumeNotFound) {
		return nil, err
	}

	// Attempt to set the size if not set
	// NOTE: We don't generate an error when the size is unset. The config validation
	// should prevent it. A zero size is allowed so tests against libvirt testing
	// endpoint are successful.
	if opts.Size, err = guessVolumeSize(pool, opts); err != nil {
		return nil, err
	}

	// If the options don't specify a target format,
	// use the default format
	if opts.Target.Format == "" {
		opts.Target.Format = defaultDirectoryImageFormat
	}

	newVolume := libvirtxml.StorageVolume{Name: name}

	var srcFmt string
	// If the volume is chained, find the parent volume or
	// create it, and then set the volume modifier
	if opts.Chained {
		var path string
		path, srcFmt, err = findOrCreateParentVolume(d.l, pool, d.s.ImageHandler(), opts)
		if err != nil {
			return nil, err
		}

		newVolume.BackingStore = &libvirtxml.StorageVolumeBackingStore{
			Path: path,
			Format: &libvirtxml.StorageVolumeTargetFormat{
				Type: srcFmt,
			},
		}
	}

	newVolume.Target = &libvirtxml.StorageVolumeTarget{
		Format: &libvirtxml.StorageVolumeTargetFormat{
			Type: opts.Target.Format,
		},
	}

	// Set the capacity for the new volume
	newVolume.Capacity = &libvirtxml.StorageVolumeSize{
		Unit:  "B",
		Value: opts.Size,
	}

	// If the volume should be sparse, do not allocate anything. only allocate a single byte and
	if opts.Sparse {
		newVolume.Allocation = &libvirtxml.StorageVolumeSize{
			Unit:  "B",
			Value: 0,
		}
	}

	volData, err := newVolume.Marshal()
	if err != nil {
		return nil, err
	}

	vol, err := pool.StorageVolCreateXML(volData, 0)
	if err != nil {
		return nil, err
	}
	defer vol.Free()

	if !opts.Chained && opts.Source.Path != "" {
		// If this isn't a chained volume and an image is defined
		// then upload the image into the volume
		if err := uploadToVolume(d.l, opts.Source.Path, vol, &opts); err != nil {
			return nil, err
		}
	}

	return &storage.Volume{
		Name:   name,
		Pool:   d.poolName,
		Format: opts.Target.Format,
	}, nil
}

// DeleteVolume implements storage.Pool
func (d *directory) DeleteVolume(name string) error {
	pool, err := d.l.FindStoragePool(d.poolName)
	if err != nil {
		return err
	}
	defer pool.Free()

	vol, err := findRawVolume(pool, name)
	if err != nil {
		if errors.Is(err, ErrVolumeNotFound) {
			return nil
		}
		return err
	}
	defer vol.Free()

	return vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
}

// GetVolume implements storage.Pool
func (d *directory) GetVolume(name string) (*storage.Volume, error) {
	pool, err := d.l.FindStoragePool(d.poolName)
	if err != nil {
		return nil, err
	}
	defer pool.Free()

	return findVolume(pool, name)
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

// volOverwriteFn is the signature for overwriting data into a volume.
type volOverwriteFn func(v shims.StorageVol, path string) error

// volResizeFn is the signature for resizing a volume.
type volResizeFn func(v shims.StorageVol, sizeBytes uint64, sparse bool) error

type pool struct {
	ctx        context.Context
	logger     hclog.Logger
	name       string
	l          libvirtStorage
	s          storage.Storage
	overwriter volOverwriteFn
	resizer    volResizeFn
}

// Name returns the name of the storage pool.
// implements storage.Pool
func (p *pool) Name() string {
	return p.name
}

// GetVolume retrieves a volume from the storage pool if it exists.
// implements storage.Pool
func (p *pool) GetVolume(name string) (*storage.Volume, error) {
	pool, err := p.l.FindStoragePool(p.name)
	if err != nil {
		return nil, err
	}
	defer pool.Free()

	return findVolume(pool, name)
}

// ListVolumes returns the volume names in the pool.
func (p *pool) ListVolumes() ([]string, error) {
	pool, err := p.l.FindStoragePool(p.name)
	if err != nil {
		return nil, err
	}
	defer pool.Free()

	return pool.ListStorageVolumes()
}

// AddVolume adds a new volume to the storage pool.
// implements storage.Pool
func (p *pool) AddVolume(name string, opts storage.Options) (*storage.Volume, error) {
	pool, err := p.l.FindStoragePool(p.name)
	if err != nil {
		return nil, err
	}
	defer pool.Free()

	// Always start with refreshing the pool.
	if err := refreshPool(p.ctx, pool, asyncJobErrRetryDefaultInterval, asyncJobErrRetryDefaultTimeout); err != nil {
		return nil, err
	}

	// If the options don't specify a target format,
	// use the default format
	if opts.Target.Format == "" {
		opts.Target.Format = defaultDirectoryImageFormat
	}

	// Check if the volume already exists
	volume, err := findVolume(pool, name)
	if err == nil {
		return volume, nil
	}

	if !errors.Is(err, ErrVolumeNotFound) {
		return nil, err
	}

	// Start configuring the new volume
	newVolume := &libvirtxml.StorageVolume{
		Name: name,
		Target: &libvirtxml.StorageVolumeTarget{
			Format: &libvirtxml.StorageVolumeTargetFormat{
				Type: opts.Target.Format,
			},
		},
		Capacity: &libvirtxml.StorageVolumeSize{
			Unit:  "B",
			Value: opts.Size,
		},
		Allocation: &libvirtxml.StorageVolumeSize{
			Unit:  "B",
			Value: opts.Size,
		},
	}

	// If this should be a sparse volume, do not allocate
	// any space.
	if opts.Sparse {
		newVolume.Allocation.Value = 0
	}

	// If a source volume is defined, load it
	var srcVol shims.StorageVol
	if opts.Source.Volume != "" {
		srcVol, err = findRawVolume(pool, opts.Source.Volume)
		if err != nil {
			return nil, err
		}
		defer srcVol.Free()
	}

	if opts.Chained {
		if srcVol == nil {
			return nil, fmt.Errorf("%w - %s", ErrVolumeNotFound, opts.Source.Volume)
		}
		srcPath, err := srcVol.GetPath()
		if err != nil {
			return nil, err
		}
		srcFmt, err := getVolumeFormat(srcVol)
		if err != nil {
			return nil, err
		}

		newVolume.BackingStore = &libvirtxml.StorageVolumeBackingStore{
			Path: srcPath,
			Format: &libvirtxml.StorageVolumeTargetFormat{
				Type: srcFmt,
			},
		}
	}

	// Generate the volume definition
	volData, err := newVolume.Marshal()
	if err != nil {
		return nil, err
	}

	p.logger.Debug("new volume definition defined", "definition", volData)

	var vol shims.StorageVol
	defer func() {
		if vol != nil {
			vol.Free()
		}
	}()

	// If a source volume is provided and the disk is not chained, clone
	// the volume. Otherwise, create the volume and upload an image if
	// one is provided.
	if srcVol != nil && !opts.Chained {
		vol, err = pool.StorageVolCreateXMLFrom(volData, srcVol, libvirtNoFlags)
		if err != nil {
			return nil, err
		}
	} else {
		vol, err = pool.StorageVolCreateXML(volData, libvirtNoFlags)
		if err != nil {
			return nil, err
		}

		// If a source path is set, upload it.
		if opts.Source.Path != "" {
			overwriterFn := p.defaultOverwriter
			if p.overwriter != nil {
				overwriterFn = p.overwriter
			}

			if err := overwriterFn(vol, opts.Source.Path); err != nil {
				return nil, err
			}
		}
	}

	// Always run a resize after volume creation even if size has
	// not changed to ensure expected allocation.
	resizeFn := p.defaultResizer
	if p.resizer != nil {
		resizeFn = p.resizer
	}

	if err := resizeFn(vol, opts.Size, opts.Sparse); err != nil {
		return nil, err
	}

	// Grab the new volume information to fill in the result.
	info, err := getVolumeInfo(vol)
	if err != nil {
		return nil, err
	}

	v := &storage.Volume{
		Name: name,
		Pool: p.name,
		Kind: info.Type,
	}

	if info.Capacity != nil {
		v.Size = info.Capacity.Value
	}

	if info.Target != nil && info.Target.Format != nil {
		v.Format = info.Target.Format.Type
	}

	return v, nil
}

// DeleteVolume deletes a volume from the storage pool.
// implements storage.Pool
func (p *pool) DeleteVolume(name string) error {
	p.logger.Debug("deleting volume from storage pool", "name", name)

	pool, err := p.l.FindStoragePool(p.name)
	if err != nil {
		return err
	}
	defer pool.Free()

	// Refresh the pool to ensure volume list is up-to-date
	if err := refreshPool(p.ctx, pool, asyncJobErrRetryDefaultInterval, asyncJobErrRetryDefaultTimeout); err != nil {
		return err
	}

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

// defaultOverwriter overwrites the volume with the content at the path.
func (p *pool) defaultOverwriter(vol shims.StorageVol, path string) error {
	// Open the source file for uploading
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Get the info for the source file to get the size
	info, err := file.Stat()
	if err != nil {
		return err
	}

	// If the source file is empty, there is nothing to upload. This
	// really only happens in tests where uploads are not supported.
	if info.Size() == 0 {
		return nil
	}

	// Create a new stream for uploading the data
	stream, err := p.l.NewStream()
	if err != nil {
		return err
	}

	// Abort the stream if an error was encountered
	// and ensure the stream is freed.
	defer func() {
		if err != nil {
			stream.Abort()
		}
		stream.Free()
	}()

	// Connect the stream to the volume for upload
	// NOTE: Only set the sparse flag if the stream supports it
	var flags libvirt.StorageVolUploadFlags = libvirtNoFlags
	if stream.Sparse() {
		flags = libvirt.STORAGE_VOL_UPLOAD_SPARSE_STREAM
	}
	if err = vol.Upload(stream, 0, uint64(info.Size()), flags); err != nil {
		return err
	}

	// Copy the file contents onto the stream
	if _, err = io.Copy(stream, file); err != nil {
		return err
	}

	// Finish the stream so the other side knows the upload is complete
	if err = stream.Finish(); err != nil {
		return err
	}

	return nil
}

// defaultResizer resizes the volume to the new size.
func (p *pool) defaultResizer(vol shims.StorageVol, sizeBytes uint64, sparse bool) error {
	info, err := vol.GetInfo()
	if err != nil {
		return err
	}

	// If the size hasn't changed, check if the resize is needed. The only time it
	// will be needed is if the volume should be fully allocated to the capacity.
	if info.Capacity == sizeBytes {
		// If the volume is sparse there is no need to resize. Resizing when
		// the size has not changed is only needed to force allocation of the
		// volume.
		if sparse {
			return nil
		}

		// If the current size of the volume is 0 and the desired size is 0
		// resizing the volume does nothing even if the volume is not sparse.
		if sizeBytes == 0 {
			return nil
		}

		// If the volume is fully allocated to the define capacity, no resizing
		// is required.
		if info.Capacity == info.Allocation {
			return nil
		}

		// In some cases the allocation of a volume will be greater than the
		// capacity. This can happen when the entire capacity of the volume
		// is used. The excess allocation is the space for the metadata of
		// the volume. If that is the case here, set the size to the current
		// allocation to allow the resize request to be successful.
		if info.Allocation > info.Capacity {
			sizeBytes = info.Allocation
		}

		// Finally, resizing to force allocation is only allowed on raw type
		// volumes.
		fmt, err := getVolumeFormat(vol)
		if err != nil {
			return err
		}
		if fmt != "raw" {
			return nil
		}
	}

	flags := libvirt.STORAGE_VOL_RESIZE_ALLOCATE
	if sparse {
		flags = libvirtNoFlags
	}

	p.logger.Debug("resizing volume", "current-size", info.Capacity, "desired-size", sizeBytes)
	if err := vol.Resize(sizeBytes, flags); err != nil {
		return err
	}

	return nil
}

// copy creates a new copy of this pool with updated context
// and storage.
func (p *pool) copy(ctx context.Context, s *Storage, l libvirtStorage) *pool {
	return &pool{
		logger:     p.logger,
		name:       p.name,
		overwriter: p.overwriter,
		resizer:    p.resizer,
		ctx:        ctx,
		s:          s,
		l:          l,
	}
}

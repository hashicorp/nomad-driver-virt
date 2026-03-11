// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

// volUploadFn is the signature for uploading a volume.
type volUploadFn func(v shims.StorageVol, path string) error

type volResizeFn func(v shims.StorageVol, sizeBytes uint64, sparse bool) error

type pool struct {
	ctx      context.Context
	logger   hclog.Logger
	name     string
	l        libvirtStorage
	s        storage.Storage
	uploader volUploadFn
	resizer  volResizeFn
}

// Name implements storage.Pool
func (p *pool) Name() string {
	return p.name
}

// GetVolume implements storage.Pool
func (p *pool) GetVolume(name string) (*storage.Volume, error) {
	pool, err := p.l.FindStoragePool(p.name)
	if err != nil {
		return nil, err
	}
	defer pool.Free()

	return findVolume(pool, name)
}

// AddVolume implements storage.Pool
func (p *pool) AddVolume(name string, opts storage.Options) (*storage.Volume, error) {
	p.logger.Debug("adding a new volume to storage pool", "name", name, "sfmt", opts.Source.Format, "tfmt", opts.Target.Format)
	pool, err := p.l.FindStoragePool(p.name)
	if err != nil {
		return nil, err
	}
	defer pool.Free()

	// Always start with refreshing the pool.
	if err := pool.Refresh(libvirtNoFlags); err != nil {
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

	// Attempt to set the size if not set
	if opts.Size, err = guessVolumeSize(pool, opts); err != nil {
		p.logger.Debug("failed to determine volume size via guess", "error", err)
		return nil, err
	}

	// Start configuring the new volume
	// NOTE: Allocation is not configured because it is ignored
	// when libvirt creates the volume.
	newVolume := libvirtxml.StorageVolume{
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
	}

	// If this should be a sparse volume, do not allocate
	// any space.
	if opts.Sparse {
		newVolume.Allocation = &libvirtxml.StorageVolumeSize{
			Unit:  "B",
			Value: 0,
		}
	}

	// A source volume will be set if the new volume is
	// being chained.
	var srcVol shims.StorageVol

	// If a source volume is defined, find it.
	if opts.Source.Volume != "" {
		srcVol, err = findRawVolume(pool, opts.Source.Volume)
		if err != nil {
			return nil, err
		}
		defer srcVol.Free()
	}

	// If the volume is chained, locate the existing source
	// volume or create the source volume.
	if opts.Chained {
		srcVol, err = findRawVolume(pool, opts.Source.Identifier)
		if err != nil && !errors.Is(err, ErrVolumeNotFound) {
			return nil, err
		}

		if srcVol == nil {
			srcVol, err = p.createVolumeFromImage(pool, opts.Source.Identifier,
				opts.Source.Path, opts.Source.Format, opts.Target.Format)
			if err != nil {
				return nil, err
			}
		}
		defer srcVol.Free()
	}

	// Generate the volume definition
	volData, err := newVolume.Marshal()
	if err != nil {
		return nil, err
	}

	var vol shims.StorageVol
	var resize bool

	// If a source volume is defined then create the new volume.
	if srcVol != nil {
		p.logger.Trace("creating new volume from existing volume", "name", name)
		vol, err = pool.StorageVolCreateXMLFrom(volData, srcVol, libvirtNoFlags)
		if err != nil {
			return nil, err
		}
		defer vol.Free()
		resize = true
	} else {
		// If no image is provided, just create an empty volume.
		if opts.Source.Path == "" {
			vol, err = pool.StorageVolCreateXML(volData, libvirtNoFlags)
			if err != nil {
				return nil, err
			}
			defer vol.Free()
		} else {
			// Otherwise, create the volume from the image
			vol, err = p.createVolumeFromImage(pool, name, opts.Source.Path,
				opts.Source.Format, opts.Target.Format)
			if err != nil {
				return nil, err
			}
			defer vol.Free()
			resize = true
		}
	}

	// If flagged, resize the volume to the correct size
	if resize {
		resizeFn := p.defaultResizer
		if p.resizer != nil {
			resizeFn = p.resizer
		}

		if err := resizeFn(vol, opts.Size, opts.Sparse); err != nil {
			return nil, err
		}
	}

	return &storage.Volume{
		Name:   name,
		Pool:   p.name,
		Format: opts.Target.Format,
	}, nil
}

// DeleteVolume implements storage.Pool
func (p *pool) DeleteVolume(name string) error {
	p.logger.Debug("deleting volume from storage pool", "name", name)

	pool, err := p.l.FindStoragePool(p.name)
	if err != nil {
		return err
	}
	defer pool.Free()

	// Refresh the pool to ensure volume list is up-to-date
	if err := pool.Refresh(libvirtNoFlags); err != nil {
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

// createVolumeFromImage creates a new volume and uploads the image into
// the volume.
// NOTE: caller is responsible to free result
func (p *pool) createVolumeFromImage(pool shims.StoragePool, name, image, srcFmt, targetFmt string) (shims.StorageVol, error) {
	var err error
	// If the source format isn't set, get it.
	if srcFmt == "" {
		if srcFmt, err = p.s.ImageHandler().GetImageFormat(image); err != nil {
			return nil, err
		}
	}

	// If the image isn't in the target format, convert it.
	if srcFmt != targetFmt {
		p.logger.Debug("converting image", "source-format", srcFmt, "target-format", targetFmt, "image", image)
		dst := image + ".converted"
		if err := p.s.ImageHandler().ConvertImage(image, srcFmt, dst, targetFmt); err != nil {
			return nil, err
		}
		defer os.RemoveAll(dst)
		image = dst
	}

	// Stat the file to get the size
	info, err := os.Stat(image)
	if err != nil {
		return nil, err
	}

	// Create the volume definition.
	vol := libvirtxml.StorageVolume{
		Name: name,
		Target: &libvirtxml.StorageVolumeTarget{
			Format: &libvirtxml.StorageVolumeTargetFormat{
				Type: targetFmt,
			},
		},
		Capacity: &libvirtxml.StorageVolumeSize{
			Unit:  "B",
			Value: uint64(info.Size()),
		},
		Allocation: &libvirtxml.StorageVolumeSize{
			Unit:  "B",
			Value: 0,
		},
	}
	volInfo, err := vol.Marshal()
	if err != nil {
		return nil, err
	}

	// Create the actual volume.
	v, err := pool.StorageVolCreateXML(volInfo, libvirtNoFlags)
	if err != nil {
		return nil, err
	}

	// Upload the image into the volume. If a custom upload function
	// is provided, use that.
	uploadFn := p.defaultUploader
	if p.uploader != nil {
		uploadFn = p.uploader
	}

	if err := uploadFn(v, image); err != nil {
		return nil, err
	}

	return v, nil
}

// defaultUploader uploads the content at the path to the volume.
func (p *pool) defaultUploader(vol shims.StorageVol, path string) error {
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
	if err = vol.Upload(stream, 0, uint64(info.Size()), libvirtNoFlags); err != nil {
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

	// If the size hasn't changed, there is nothing to do.
	if info.Capacity == sizeBytes {
		return nil
	}

	flags := libvirt.STORAGE_VOL_RESIZE_ALLOCATE
	if sparse {
		flags = libvirtNoFlags
	}

	if err := vol.Resize(sizeBytes, flags); err != nil {
		return err
	}

	return nil
}

// copy creates a new copy of this pool with updated context
// and storage.
func (p *pool) copy(ctx context.Context, s *Storage, l libvirtStorage) *pool {
	return &pool{
		logger:   p.logger,
		name:     p.name,
		uploader: p.uploader,
		resizer:  p.resizer,
		ctx:      ctx,
		s:        s,
		l:        l,
	}
}

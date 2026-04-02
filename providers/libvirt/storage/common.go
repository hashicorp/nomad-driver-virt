// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"errors"
	"io"
	"os"

	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

// findRawVolume returns the libvirt volume if found in the pool
// NOTE: caller is responsible to free result
func findRawVolume(pool shims.StoragePool, name string) (result shims.StorageVol, err error) {
	vol, err := pool.LookupStorageVolByName(name)
	if err != nil && !errors.Is(err, libvirt.ERR_NO_STORAGE_VOL) {
		return nil, err
	}

	if vol == nil {
		return nil, ErrVolumeNotFound
	}

	return vol, nil
}

// findVolume returns the volume if found in the pool
func findVolume(pool shims.StoragePool, name string) (*storage.Volume, error) {
	v, err := findRawVolume(pool, name)
	if err != nil {
		return nil, err
	}

	poolName, err := pool.GetName()
	if err != nil {
		return nil, err
	}

	info, err := getVolumeInfo(v)
	if err != nil {
		return nil, err
	}

	defer v.Free()

	vol := &storage.Volume{
		Name: name,
		Pool: poolName,
		Kind: info.Type,
	}

	if info.Capacity != nil {
		vol.Size = info.Capacity.Value
	}

	if info.Target != nil && info.Target.Format != nil {
		vol.Format = info.Target.Format.Type
	}

	return vol, nil
}

// getVolumeFormat gets the format of the given volume
func getVolumeFormat(vol shims.StorageVol) (string, error) {
	infoXml, err := vol.GetXMLDesc(libvirtNoFlags)
	if err != nil {
		return "", err
	}
	volInfo := &libvirtxml.StorageVolume{}

	if err := volInfo.Unmarshal(infoXml); err != nil {
		return "", err
	}

	if volInfo.Target != nil && volInfo.Target.Format != nil {
		return volInfo.Target.Format.Type, nil
	}

	return "", nil // return empty value, allow auto detection if available
}

// uploadToVolume uploads a file to a volume.
// TODO: add specialization to support sparse files on upload
func uploadToVolume(l libvirtStorage, src string, vol shims.StorageVol, opts *storage.Options) error {
	// Open the source file for uploading
	file, err := os.Open(src)
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
	stream, err := l.NewStream()
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

	// After the upload the capacity will clamp to the size of the image,
	// so if options were passed and a size is set, resize the volume.
	if opts != nil && opts.Size > 0 && opts.Size > uint64(info.Size()) {
		var flag libvirt.StorageVolResizeFlags
		if !opts.Sparse {
			flag = libvirt.STORAGE_VOL_RESIZE_ALLOCATE // ensure size is fully allocated if not sparse
		}

		if err := vol.Resize(opts.Size, flag); err != nil {
			return err
		}
	}

	return nil
}

// getPoolInfo returns the pool description information.
func getPoolInfo(pool shims.StoragePool) (*libvirtxml.StoragePool, error) {
	poolXml, err := pool.GetXMLDesc(libvirtNoFlags)
	if err != nil {
		return nil, err
	}
	poolInfo := new(libvirtxml.StoragePool)
	if err := poolInfo.Unmarshal(poolXml); err != nil {
		return nil, err
	}

	return poolInfo, nil
}

func getVolumeInfo(vol shims.StorageVol) (*libvirtxml.StorageVolume, error) {
	volXml, err := vol.GetXMLDesc(libvirtNoFlags)
	if err != nil {
		return nil, err
	}
	volInfo := new(libvirtxml.StorageVolume)
	if err := volInfo.Unmarshal(volXml); err != nil {
		return nil, err
	}

	return volInfo, nil
}

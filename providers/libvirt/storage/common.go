// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"context"
	"errors"
	"strings"
	"time"

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

// getVolumeInfo returns the volume description information.
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

// refreshPool is a helper for refreshing the pool that handles errors
// caused by running async jobs by polling for a successful result.
// NOTE: libvirt issue related to this: https://gitlab.com/libvirt/libvirt/-/work_items/454
func refreshPool(ctx context.Context, pool shims.StoragePool, retryInterval, retryTimeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, retryTimeout)
	defer cancel()
	retryAfter := time.Duration(0)

	for {
		select {
		case <-time.After(retryAfter):
			err := pool.Refresh(libvirtNoFlags)
			if err == nil {
				return nil
			}
			if !isAsyncJobErr(err) {
				return err
			}
			retryAfter = retryInterval
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// asyncJobErrMsg is content found within an async job error from libvirt.
const asyncJobErrMsg = "asynchronous jobs running"

var (
	// asyncJobErrRetryDefaultInterval is the polling interval used for retries.
	asyncJobErrRetryDefaultInterval = time.Second

	// asyncJobErrRetryDefaultTimeout is the maximum amount of time to attempt retries.
	asyncJobErrRetryDefaultTimeout = 30 * time.Second
)

// isAsyncJobErr is a helper function to determine if an error is an async job
// error from libvirt.
func isAsyncJobErr(err error) bool {
	return errors.Is(err, libvirt.ERR_INTERNAL_ERROR) &&
		strings.Contains(err.Error(), asyncJobErrMsg)
}

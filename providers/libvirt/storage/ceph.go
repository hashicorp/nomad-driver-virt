// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"plugin"
	"slices"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"github.com/hashicorp/nomad-driver-virt/virt/disks"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

const (
	// default format used for ceph based volumes
	defaultCephImageFormat = disks.DiskFormatRaw
)

// cephPlugin holds the plugin for direct volume uploads.
var cephPlugin *plugin.Plugin

type CephConnect struct {
	Username string
	Key      string
	Hosts    []string
}

type ceph struct {
	*pool
}

// newCephPool loads the ceph backed storage pool, creating or updating it if needed.
func newCephPool(ctx context.Context, logger hclog.Logger, l libvirtStorage, poolName string, config storage.Ceph, s storage.Storage) (*ceph, error) {
	logger = logger.Named("storage-pool").With("pool", poolName)

	// Ensure the ceph plugin is loaded
	if err := pluginLoader("ceph", cephPlugin); err != nil {
		logger.Error("failed to load ceph storage plugin, ensure librados and rdb libraries are available",
			"error", err)
		return nil, err
	}

	p, err := l.FindStoragePool(poolName)
	if err != nil && !errors.Is(err, vm.ErrNotFound) {
		logger.Debug("unexpected error during pool lookup", "error", err)
		return nil, err
	}

	if p != nil {
		defer p.Free()
	}

	secretId, err := l.SetCephSecret(poolName, config.Authentication.Secret)
	if err != nil {
		logger.Debug("unexpected error creating ceph secret", "error", err)
		return nil, err
	}

	cephHosts := []libvirtxml.StoragePoolSourceHost{}
	for _, cephHost := range config.Hosts {
		// Cheat and format host as URL to easily split
		// host and port
		u, err := url.Parse("test://" + cephHost)
		if err != nil {
			logger.Debug("unable to parse ceph host", "host", cephHost, "error", err)
			return nil, err
		}

		cephHosts = append(cephHosts, libvirtxml.StoragePoolSourceHost{
			Name: u.Hostname(),
			Port: u.Port(),
		})
	}

	if p == nil {
		logger.Debug("creating new ceph storage pool")
		if p, err = l.CreateStoragePool(&libvirtxml.StoragePool{
			Name: poolName,
			Type: "rbd",
			Source: &libvirtxml.StoragePoolSource{
				Name: config.Pool,
				Host: cephHosts,
				Auth: &libvirtxml.StoragePoolSourceAuth{
					Type:     "ceph",
					Username: config.Authentication.Username,
					Secret: &libvirtxml.StoragePoolSourceAuthSecret{
						UUID: secretId,
					},
				},
			},
		}); err != nil {
			return nil, err
		}
		defer p.Free()
	} else {
		var needUpdate bool
		info, err := getPoolInfo(p)
		if err != nil {
			return nil, err
		}

		if !slices.Equal(info.Source.Host, cephHosts) {
			info.Source.Host = cephHosts
			needUpdate = true
		}

		if info.Source.Name != config.Pool {
			info.Source.Name = config.Pool
			needUpdate = true
		}

		if needUpdate {
			logger.Debug("updating existing ceph storage pool")
			if err := l.UpdateStoragePool(info); err != nil {
				return nil, err
			}
		}
	}

	// Ensure the pool is actually running. If it's not, start it.
	poolRunning, err := p.IsActive()
	if err != nil {
		logger.Debug("unexpected error checking pool status", "error", err)
		return nil, err
	}

	if !poolRunning {
		logger.Debug("pool is not active, creating")
		if err := p.Create(libvirt.STORAGE_POOL_CREATE_NORMAL); err != nil {
			return nil, err
		}
	}

	// Build the base pool
	basePool := &pool{
		ctx:    ctx,
		logger: logger,
		name:   poolName,
		l:      l,
		s:      s,
	}
	// Build the ceph wrapper
	c := &ceph{pool: basePool}
	// Set customized functions for this pool type.
	basePool.overwriter = c.overwriteVolume
	basePool.resizer = c.resizeVol

	return c, nil
}

// ValidateDisk validates the provided disk and returns any configuration errors found.
// implements disks.DiskValidator
func (c *ceph) ValidateDisk(disk *disks.Disk) error {
	var mErr *multierror.Error

	// Only raw format is supported for ceph volumes
	if disk.Format != disks.DiskFormatRaw {
		mErr = multierror.Append(mErr,
			fmt.Errorf("%w: format can only be raw for ceph volumes", disks.ErrInvalidConfiguration))
	}

	if disk.Sparse != nil && *disk.Sparse {
		mErr = multierror.Append(mErr,
			fmt.Errorf("%w: sparse cannot be enabled for ceph volumes", disks.ErrInvalidConfiguration))
	}

	if mErr != nil {
		return mErr
	}

	return nil
}

// Type returns the type of the storage pool.
// implements storage.Pool
func (c *ceph) Type() string {
	return storage.PoolTypeCeph
}

// DefaultImageFormat returns the default image format for the pool.
// implements storage.Pool
func (c *ceph) DefaultImageFormat() string {
	return defaultCephImageFormat
}

// AddVolume adds a new volume to the storage pool. Forces a raw format for the target prior
// to adding the volume.
// implements storage.Pool
func (c *ceph) AddVolume(name string, opts storage.Options) (*storage.Volume, error) {
	// If a volume is set to be cloned (volume source and not chained) then use custom
	// helper since libvirt does not support fully cloning a volume.
	if !opts.Chained && opts.Source.Volume != "" {
		return c.copyVol(name, opts)
	}

	// If the disk should be chained, libvirt doesn't support a volume defining a
	// backing store. Instead, cloning a volume will create a chained volume, so
	// disable the option before proceeding.
	if opts.Chained {
		opts.Chained = false
	}

	// Ceph volumes are always sparsely populated, so force sparse.
	opts.Sparse = true

	// Add the new volume to the pool.
	return c.pool.AddVolume(name, opts)
}

// copyVol will create a new full copy of the source volume
func (c *ceph) copyVol(name string, opts storage.Options) (*storage.Volume, error) {
	connOpts, err := c.buildConnectOpts()
	if err != nil {
		return nil, err
	}

	// The name of the ceph pool is needed since the copy is performed
	// using a direct connection. Extract it from the pool info.
	p, err := c.l.FindStoragePool(c.pool.name)
	if err != nil {
		return nil, err
	}
	defer p.Free()

	info, err := getPoolInfo(p)
	if err != nil {
		return nil, err
	}

	if info.Source == nil || info.Source.Name == "" {
		return nil, fmt.Errorf("failed to determine remote ceph pool name")
	}

	// Grab the volume copy function and make the copy.
	fn, err := cephPlugin.Lookup("VolumeCopy")
	if err != nil {
		return nil, err
	}

	// Create the new cloned volume.
	err = fn.(func(ctx context.Context, connInfo *CephConnect, pool, srcVol, dstVol string) error)(
		c.pool.ctx, connOpts, info.Source.Name, opts.Source.Volume, name)
	if err != nil {
		return nil, err
	}

	// Refresh the pool so the new volume is found.
	if err := p.Refresh(libvirtNoFlags); err != nil {
		return nil, err
	}

	v, err := findRawVolume(p, name)
	if err != nil {
		return nil, err
	}
	defer v.Free()

	// Run the resizer on the new volume so if the new volume size
	// is larger it is properly expanded.
	if err := c.resizeVol(v, opts.Size, true); err != nil {
		return nil, err
	}

	// Return the volume loaded with expected information.
	return findVolume(p, name)
}

// resizeVol is a custom volume resizer for rbd volumes that ignores sparseness
// since rbd volumes do not support immediate allocation.
func (c *ceph) resizeVol(vol shims.StorageVol, sizeBytes uint64, _ bool) error {
	info, err := vol.GetInfo()
	if err != nil {
		return err
	}

	// If the size hasn't changed, there is nothing to do.
	if info.Capacity == sizeBytes {
		return nil
	}

	if err := vol.Resize(sizeBytes, libvirtNoFlags); err != nil {
		return err
	}

	return nil
}

// overwriteVolume overwrites the volume with the content at the path.
// NOTE: libvirt does not support streams for rbd volumes, so direct
// connection is used for uploads.
func (c *ceph) overwriteVolume(v shims.StorageVol, path string) error {
	c.logger.Debug("uploading content to volume", "path", path)

	// The path of the volume is the ceph pool name
	// and the volume name.
	volPath, err := v.GetPath()
	if err != nil {
		return err
	}

	pathParts := strings.SplitN(volPath, "/", 2)
	if len(pathParts) != 2 {
		return fmt.Errorf("%w - invalid volume path: %s", ErrInvalidVolumeConfiguration, volPath)
	}

	poolName := pathParts[0]
	name := pathParts[1]

	connOpts, err := c.buildConnectOpts()
	if err != nil {
		return err
	}

	fn, err := cephPlugin.Lookup("VolumeUpload")
	if err != nil {
		return err
	}

	err = fn.(func(ctx context.Context, connInfo *CephConnect, pool, volume, path string) error)(
		c.pool.ctx, connOpts, poolName, name, path)
	if err != nil {
		return err
	}

	return nil
}

func (c *ceph) buildConnectOpts() (*CephConnect, error) {
	pool, err := c.l.FindStoragePool(c.name)
	if err != nil {
		return nil, err
	}
	defer pool.Free()

	// Grab the pool information to get the configured hosts
	info, err := getPoolInfo(pool)
	if err != nil {
		return nil, err
	}

	// Pull the stored key. The value will be provided base64
	// encoded which is what the connection will want.
	key, err := c.l.GetCephSecret(c.name)
	if err != nil {
		return nil, err
	}

	// Collect all the monitor hosts
	// TODO: Need to check for IPv6 and wrap with brackets if port is set
	hosts := make([]string, len(info.Source.Host))
	for i := range len(info.Source.Host) {
		h := info.Source.Host[i]
		if h.Port != "" {
			hosts[i] = fmt.Sprintf("%s:%s", h.Name, h.Port)
		} else {
			hosts[i] = h.Name
		}
	}

	return &CephConnect{
		Username: info.Source.Auth.Username,
		Key:      key,
		Hosts:    hosts,
	}, nil
}

// copy creates a new copy of this pool with updated context
// and storage.
func (c *ceph) copy(ctx context.Context, s *Storage, l libvirtStorage) *ceph {
	return &ceph{pool: c.pool.copy(ctx, s, l)}
}

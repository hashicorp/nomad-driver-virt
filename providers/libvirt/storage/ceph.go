// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/ceph/go-ceph/rados"
	"github.com/ceph/go-ceph/rbd"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/internal/ctxio"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

const (
	// default format used for ceph based volumes
	defaultCephImageFormat = "raw"

	// timeout for operations handled by monitors
	radosMonOpTimeout = "30"
	// timeout for operations handled by osds
	radosOsdOpTimeout = "30"
)

type volUploadFn func(v shims.StorageVol, path string) error

type ceph struct {
	ctx      context.Context
	poolName string
	logger   hclog.Logger

	// This defines the function to use for uploading an image to
	// the volume, isolated so it can be swapped during testing.
	uploader volUploadFn

	l libvirtStorage
	s storage.Storage
}

// newCephPool loads the ceph backed storage pool, creating or updating it if needed.
func newCephPool(ctx context.Context, logger hclog.Logger, l libvirtStorage, poolName string, config storage.Ceph, s storage.Storage) (*ceph, error) {
	logger = logger.Named("storage-pool").With("pool", poolName)
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

	c := &ceph{logger: logger, poolName: poolName, l: l, s: s, ctx: ctx}
	c.uploader = c.uploadToVolume

	return c, nil
}

// Name implements storage.Pool
func (c *ceph) Name() string {
	return c.poolName
}

// Type implements storage.Pool
func (c *ceph) Type() string {
	return storage.PoolTypeCeph
}

// DefaultImageFormat implements storage.Pool
func (c *ceph) DefaultImageFormat() string {
	return defaultCephImageFormat
}

// AddVolume implements storage.Pool
func (c *ceph) AddVolume(name string, opts storage.Options) (*storage.Volume, error) {
	c.logger.Debug("adding a new volume to storage pool", "name", name, "sfmt", opts.Source.Format, "tfmt", opts.Target.Format)
	pool, err := c.l.FindStoragePool(c.poolName)
	if err != nil {
		return nil, err
	}
	defer pool.Free()

	// If the target format is specified, warn and modify if not raw.
	if opts.Target.Format != defaultCephImageFormat {
		c.logger.Warn("only raw format is allowed for target, modifying to raw", "name", name,
			"original-format", opts.Target.Format)
		opts.Target.Format = defaultCephImageFormat
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
		c.logger.Debug("failed to determine volume size via guess", "error", err)
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
			srcVol, err = c.createVolumeFromImage(pool, opts.Source.Identifier,
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
		c.logger.Trace("creating new volume from existing volume", "name", name)
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
			vol, err = c.createVolumeFromImage(pool, name, opts.Source.Path,
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
		info, err := vol.GetInfo()
		if err != nil {
			return nil, err
		}

		if info.Capacity < opts.Size {
			c.logger.Debug("volume has shrunk from expected capacity, resizing", "expected", opts.Size, "actual", info.Capacity)

			// NOTE: Volumes in the ceph pool do not support allocating on resize so
			// the sparse setting is ignored.
			if err := vol.Resize(opts.Size, libvirtNoFlags); err != nil {
				return nil, err
			}
		}
	}

	return &storage.Volume{
		Name:   name,
		Pool:   c.poolName,
		Format: opts.Target.Format,
	}, nil
}

// DeleteVolume implements storage.Pool
func (c *ceph) DeleteVolume(name string) error {
	c.logger.Debug("deleting volume from storage pool", "name", name)

	pool, err := c.l.FindStoragePool(c.poolName)
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

// GetVolume implements storage.Pool
func (c *ceph) GetVolume(name string) (*storage.Volume, error) {
	pool, err := c.l.FindStoragePool(c.poolName)
	if err != nil {
		return nil, err
	}
	defer pool.Free()

	return findVolume(pool, name)
}

// createVolumeFromImage creates a new volume and uploads the image into
// the volume.
// NOTE: caller is responsible to free result
func (c *ceph) createVolumeFromImage(pool shims.StoragePool, name, image, srcFmt, targetFmt string) (shims.StorageVol, error) {
	var err error
	// If the source format isn't set, get it.
	if srcFmt == "" {
		if srcFmt, err = c.s.ImageHandler().GetImageFormat(image); err != nil {
			return nil, err
		}
	}

	// If the image isn't in the target format, convert it.
	if srcFmt != targetFmt {
		c.logger.Debug("converting image", "source-format", srcFmt, "target-format", targetFmt, "image", image)
		dst := image + ".converted"
		if err := c.s.ImageHandler().ConvertImage(image, srcFmt, dst, targetFmt); err != nil {
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

	// Upload the image into the volume.
	if err := c.uploader(v, image); err != nil {
		return nil, err
	}

	return v, nil
}

// uploadToVolume will upload the content at the given path to the volume.
// NOTE: libvirt does not support streams for rbd volumes, so direct
// connection is used for uploads.
func (c *ceph) uploadToVolume(v shims.StorageVol, path string) error {
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

	conn, err := c.cephConnect()
	if err != nil {
		return err
	}
	defer conn.Shutdown()

	// Disable writethrough caching. If this is not disabled
	// uploads will be extremely slow.
	if err := conn.SetConfigOption("rbd_cache_writethrough_until_flush", "false"); err != nil {
		return err
	}

	// Open a new IO context on the connection which can
	// be used for interacting with the volume.
	ioctx, err := conn.OpenIOContext(poolName)
	if err != nil {
		return err
	}
	defer ioctx.Destroy()

	// Open the remote volume.
	img, err := rbd.OpenImage(ioctx, name, rbd.NoSnapshot)
	if err != nil {
		return err
	}
	defer img.Close()

	// Open the local file to upload.
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Stat the file so the size can be verified
	// after upload
	fInfo, err := f.Stat()
	if err != nil {
		return err
	}

	// Grab an exclusive lock on the image to prevent anything
	// being done to it while it is being used.
	if err := img.LockAcquire(rbd.LockModeExclusive); err != nil {
		return err
	}
	defer img.LockRelease()

	// Copy the source image into the volume. The reader and writer
	// are wrapped with a context to allow the copy to be interrupted
	// if the task has been stopped.
	wrote, err := io.Copy(
		ctxio.NewWriterAt(c.ctx, img),
		ctxio.NewReaderAt(c.ctx, f),
	)
	if err != nil {
		return err
	}

	// Check that everything was uploaded.
	if wrote != fInfo.Size() {
		return fmt.Errorf("upload to volume failed, missing %d bytes", fInfo.Size()-wrote)
	}

	return nil
}

// cephConnect establishes a connection to the ceph cluster the
// pool is located on.
func (c *ceph) cephConnect() (*rados.Conn, error) {
	pool, err := c.l.FindStoragePool(c.poolName)
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
	key, err := c.l.GetCephSecret(c.poolName)
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

	// Setup a new connection using the configured username
	conn, err := rados.NewConnWithUser(info.Source.Auth.Username)
	if err != nil {
		return nil, err
	}

	// Set timeout options on the connection
	if err := conn.SetConfigOption("rados_mon_op_timeout", radosMonOpTimeout); err != nil {
		return nil, err
	}

	if err := conn.SetConfigOption("rados_osd_op_timeout", radosOsdOpTimeout); err != nil {
		return nil, err
	}

	// Set the key credential
	if err := conn.SetConfigOption("key", key); err != nil {
		return nil, err
	}

	// Set the collection of monitor hosts
	if err := conn.SetConfigOption("mon_host", strings.Join(hosts, ",")); err != nil {
		return nil, err
	}

	// Attempt to make the connection
	if err := conn.Connect(); err != nil {
		return nil, err
	}

	// Looks like it's working \o/
	return conn, nil
}

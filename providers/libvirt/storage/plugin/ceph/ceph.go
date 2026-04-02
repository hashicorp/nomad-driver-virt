// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ceph/go-ceph/rados"
	"github.com/ceph/go-ceph/rbd"
	"github.com/hashicorp/nomad-driver-virt/internal/ctxio"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/storage"
	"github.com/hashicorp/nomad/helper/uuid"
)

const (
	// timeout for operations handled by monitors
	radosMonOpTimeout = "30"
	// timeout for operations handled by osds
	radosOsdOpTimeout = "30"
)

const (
	// Value used for shared lock on source image when copying.
	imageCopyLockTag = "nomad-vol-copy"
)

// VolumeCopy creates a new volume in the pool by creating a full copy of an existing volume.
func VolumeCopy(ctx context.Context, con *storage.CephConnect, pool, srcVol, dstVol string) error {
	conn, err := cephConnect(con)
	if err != nil {
		return fmt.Errorf("ceph copy connection failure: %w", err)
	}
	defer conn.Shutdown()

	// Generate a cookie for locking the source image.
	lockCookie := uuid.Short()
	// Define a name for the ephemeral snapshot.
	snapName := fmt.Sprintf("nomad-tmp-clone-%s", lockCookie)

	// Create an IO context which is used for interacting
	// with the pool.
	ioctx, err := conn.OpenIOContext(pool)
	if err != nil {
		return fmt.Errorf("ioctx failure: %w", err)
	}
	defer ioctx.Destroy()

	// Open the source image and lock it before starting.
	src, err := rbd.OpenImage(ioctx, srcVol, rbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("source open failure: %w", err)
	}
	defer src.Close()

	if err := src.LockShared(lockCookie, imageCopyLockTag); err != nil {
		return fmt.Errorf("source lock failure: %w", err)
	}
	defer src.Unlock(lockCookie)

	// Create a new snapshot of the source volume. This snapshot
	// will only exist for this copy, so delete it when complete.
	snap, err := src.CreateSnapshot(snapName)
	if err != nil {
		return fmt.Errorf("snapshot create failure: %w", err)
	}
	defer snap.Remove()

	// Protect the snapshot to prevent deletion.
	if err := snap.Protect(); err != nil {
		return fmt.Errorf("snapshot protect failure: %w", err)
	}
	defer snap.Unprotect()

	// New image options for the clone just sets the clone format
	// to the new (latest) version.
	cloneOpts := rbd.NewRbdImageOptions()
	defer cloneOpts.Destroy()
	if err := cloneOpts.SetUint64(rbd.ImageOptionCloneFormat, 2); err != nil {
		return fmt.Errorf("clone options setup failure: %w", err)
	}

	// Clone the source volume to the new volume.
	if err := rbd.CloneImage(ioctx, srcVol, snapName, ioctx, dstVol, cloneOpts); err != nil {
		return fmt.Errorf("clone of source volume failed: %w", err)
	}

	// The cloned volume will be chained to the snapshot in the source
	// volume. To make the new volume fully standalone, flatten the
	// volume which forces the blocks to be copied into the new volume.
	dst, err := rbd.OpenImage(ioctx, dstVol, rbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("volume open failure: %w", err)
	}
	defer dst.Close()

	// Generally flattening an image is relatively fast, but may be
	// slower depending on cluster conditions or size of the image.
	// Spin it can be interrupted.
	ch := make(chan struct{}, 1)
	var fErr error
	go func() {
		if err := dst.Flatten(); err != nil {
			fErr = fmt.Errorf("volume flatten failure: %w", err)
		}
	}()

	select {
	case <-ch:
		if fErr != nil {
			return fErr
		}
	case <-ctx.Done():
		return fmt.Errorf("volume flatten interrupted: %w", ctx.Err())
	}

	return nil
}

// VolumeUpload will upload the data within path to the volume.
func VolumeUpload(ctx context.Context, con *storage.CephConnect, pool, volume, path string) error {
	conn, err := cephConnect(con)
	if err != nil {
		return fmt.Errorf("ceph copy connection failure: %w", err)
	}
	defer conn.Shutdown()

	// Disable writethrough caching. If this is not disabled
	// uploads will be extremely slow.
	if err := conn.SetConfigOption("rbd_cache_writethrough_until_flush", "false"); err != nil {
		return fmt.Errorf("ceph connection option failure: %w", err)
	}

	// Open a new IO context on the connection which can
	// be used for interacting with the volume.
	ioctx, err := conn.OpenIOContext(pool)
	if err != nil {
		return fmt.Errorf("ioctx failure: %w", err)
	}
	defer ioctx.Destroy()

	// Open the remote volume.
	img, err := rbd.OpenImage(ioctx, volume, rbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("volume open failure: %w", err)
	}
	defer img.Close()

	// Open the local file to upload.
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("path open failure: %w", err)
	}
	defer f.Close()

	// Stat the file so the size can be verified
	// after upload
	fInfo, err := f.Stat()
	if err != nil {
		return fmt.Errorf("path info failure: %w", err)
	}

	// Grab an exclusive lock on the image to prevent anything
	// being done to it while it is being used.
	if err := img.LockAcquire(rbd.LockModeExclusive); err != nil {
		return fmt.Errorf("volume lock failure: %w", err)
	}
	defer img.LockRelease()

	// Copy the source image into the volume. The reader and writer
	// are wrapped with a context to allow the copy to be interrupted
	// if the task has been stopped.
	wrote, err := io.Copy(
		ctxio.NewWriter(ctx, img),
		ctxio.NewReaderFrom(ctx, f),
	)
	if err != nil {
		return fmt.Errorf("data copy to volume failure: %w", err)
	}

	// Check that everything was uploaded.
	if wrote != fInfo.Size() {
		return fmt.Errorf("upload to volume failed, missing %d bytes", fInfo.Size()-wrote)
	}

	return nil
}

// cephConnect establishes a connection to the ceph monitors.
func cephConnect(con *storage.CephConnect) (*rados.Conn, error) {
	conn, err := rados.NewConnWithUser(con.Username)
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
	if err := conn.SetConfigOption("key", con.Key); err != nil {
		return nil, err
	}

	// Set the collection of monitor hosts
	if err := conn.SetConfigOption("mon_host", strings.Join(con.Hosts, ",")); err != nil {
		return nil, err
	}

	// Attempt to make the connection
	if err := conn.Connect(); err != nil {
		return nil, err
	}

	// Looks like it's working \o/
	return conn, nil
}

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
)

const (
	// timeout for operations handled by monitors
	radosMonOpTimeout = "30"
	// timeout for operations handled by osds
	radosOsdOpTimeout = "30"
)

func VolumeUpload(ctx context.Context, con *storage.CephConnect, pool, volume, path string) error {
	conn, err := cephConnect(con)
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
	ioctx, err := conn.OpenIOContext(pool)
	if err != nil {
		return err
	}
	defer ioctx.Destroy()

	// Open the remote volume.
	img, err := rbd.OpenImage(ioctx, volume, rbd.NoSnapshot)
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
		ctxio.NewWriter(ctx, img),
		ctxio.NewReaderFrom(ctx, f),
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

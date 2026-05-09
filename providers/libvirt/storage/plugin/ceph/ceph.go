// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

// This plugin provides some functionality that is currently lacking
// in libvirt for Ceph backed volumes. Currently the plugin is providing:
//
// * volume upload support
// * full volume copy support

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ceph/go-ceph/rados"
	"github.com/ceph/go-ceph/rbd"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/storage"
	"github.com/hashicorp/nomad/helper/uuid"
)

const (
	// timeout for operations handled by monitors
	radosMonOpTimeout = "30"
	// timeout for operations handled by osds
	radosOsdOpTimeout = "30"
)

var errNoSnapshot = errors.New("no current snapshot found")

// VolumeCopy creates a new volume in the pool by creating a full copy of an existing volume.
func VolumeCopy(ctx context.Context, con *storage.CephConnect, pool, srcVol, dstVol string) error {
	conn, err := cephConnect(con)
	if err != nil {
		return fmt.Errorf("ceph copy connection failure: %w", err)
	}
	defer conn.Shutdown()

	// Create an IO context which is used for interacting
	// with the pool.
	ioctx, err := conn.OpenIOContext(pool)
	if err != nil {
		return fmt.Errorf("ioctx failure: %w", err)
	}
	defer ioctx.Destroy()

	// Find an existing up-to-date snapshot to use for the clone.
	snapName, err := getImageSnapshot(ioctx, srcVol)
	if err != nil {
		if !errors.Is(err, errNoSnapshot) {
			return err
		}

		// No viable snapshot is available so generate a new one.
		snapName, err = generateSnapshot(ioctx, srcVol)
		if err != nil {
			return err
		}
	}

	// Open the base image.
	img, err := rbd.OpenImageReadOnly(ioctx, srcVol, rbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("volume read-only open failure: %w", err)
	}
	defer img.Close()

	// Check that the image has the layering feature enabled. It is required
	// for cloning the image. The layering feature is enabled by default, so
	// this should (hopefully) be something that is rarely seen.
	if ok, err := imageHasFeature(img, rbd.FeatureLayering); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("source volume missing required 'layering' feature")
	}

	// Load the snapshot.
	snap := img.GetSnapshot(snapName)

	// Snapshots must be protected for cloning. If the snapshot is unprotected
	// then protect it during the clone and flatten process and return it to its
	// unprotected state once done.
	if ok, err := snap.IsProtected(); err != nil {
		return fmt.Errorf("snapshot protection detection failure: %w", err)
	} else if !ok {
		if err := protectSnapshot(ioctx, srcVol, snapName); err != nil {
			return err
		}

		defer unprotectSnapshot(ioctx, srcVol, snapName)
	}

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
	// Run it in a goroutine so it can be interrupted if the context
	// completes.
	ch := make(chan error, 1)
	go func() {
		ch <- dst.Flatten()
	}()

	select {
	case err := <-ch:
		if err != nil {
			return fmt.Errorf("volume flatten failure: %w", err)
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

	// Lock the image for modification.
	unlocker, err := writeLock(img)
	if err != nil {
		return err
	}
	defer unlocker()

	// Copy the source image into the volume. The reader and writer
	// are wrapped with a context to allow the copy to be interrupted
	// if the task has been stopped.
	wrote, err := io.Copy(SparseWriter(ctx, img), f)
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

// writeLock locks the image for modification and returns a function for
// unlocking the image.
func writeLock(img *rbd.Image) (func() error, error) {
	// Check the image for the `exclusive-lock` feature. If it is enabled acquire a managed
	// lock for modifying the image. The feature is enabled by default so this should
	// generally be the locking method used.
	if ok, err := imageHasFeature(img, rbd.FeatureExclusiveLock); err != nil {
		return nil, err
	} else if ok {
		if err := img.LockAcquire(rbd.LockModeExclusive); err != nil {
			return nil, err
		}

		return img.LockRelease, nil
	}

	// Without the `exclusive-lock` feature, use an unmanaged lock.
	cookie := uuid.Short()
	if err := img.LockExclusive(cookie); err != nil {
		return nil, err
	}

	return func() error { return img.Unlock(cookie) }, nil
}

// hasFeature checks if the image has the requested feature enabled.
func imageHasFeature(img *rbd.Image, feature uint64) (bool, error) {
	currentFeatures, err := img.GetFeatures()
	if err != nil {
		return false, fmt.Errorf("volume features failure: %w", err)
	}

	return (currentFeatures & feature) == feature, nil
}

// protectSnapshot will protect the snapshot of the image. Snapshots must be
// protected to be cloned.
func protectSnapshot(ioctx *rados.IOContext, imgName, snapName string) error {
	img, err := rbd.OpenImage(ioctx, imgName, rbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("volume open failure: %w", err)
	}
	defer img.Close()

	unlocker, err := writeLock(img)
	if err != nil {
		return fmt.Errorf("volume write lock failure: %w", err)
	}
	defer unlocker()

	if err := img.GetSnapshot(snapName).Protect(); err != nil {
		return fmt.Errorf("volume snapshot protection failure: %w", err)
	}

	return nil
}

// unprotectSnapshot will unprotect the snapshot of the image. Snapshots must
// not have any clones to be unprotected.
func unprotectSnapshot(ioctx *rados.IOContext, imgName, snapName string) error {
	img, err := rbd.OpenImage(ioctx, imgName, rbd.NoSnapshot)
	if err != nil {
		return fmt.Errorf("volume open failure: %w", err)
	}
	defer img.Close()

	unlocker, err := writeLock(img)
	if err != nil {
		return fmt.Errorf("volume write lock failure: %w", err)
	}
	defer unlocker()

	if err := img.GetSnapshot(snapName).Unprotect(); err != nil {
		return fmt.Errorf("volume snapshot protection failure: %w", err)
	}

	return nil
}

// getImageSnapshot returns the name of an existing up-to-date snapshot of the
// image. If an up-to-date snapshot of the image does not exist, errNoSnapshot
// will be returned.
func getImageSnapshot(ioctx *rados.IOContext, imgName string) (string, error) {
	img, err := rbd.OpenImageReadOnly(ioctx, imgName, rbd.NoSnapshot)
	if err != nil {
		return "", fmt.Errorf("volume open failure: %w", err)
	}
	defer img.Close()

	// Start with getting the size of the image.
	imgSize, err := img.GetSize()
	if err != nil {
		return "", fmt.Errorf("volume size unavailable: %w", err)
	}

	// Get the list of existing snapshots.
	snaps, err := img.GetSnapshotNames()
	if err != nil {
		return "", fmt.Errorf("volume snapshot list failure: %w", err)
	}

	// If there are no snapshots, there is no latest.
	if len(snaps) == 0 {
		return "", errNoSnapshot
	}

	// diffResult is used in the diff callback to surface the result
	type diffResult struct {
		detected  bool // flag that difference was detected
		earlyExit bool // flag that error was an early exit
	}

	// Attempt to find an up-to-date snapshot of the image. To do this we run a
	// diff between the image and the snapshot. If no difference is detected then
	// it is an up-to-date snapshot that can be used for cloning.
	for _, snapInfo := range snaps {
		// This value is used to surface the result of the check.
		snapIsDiff := &diffResult{}
		// Build the configuration for the diff operation. The callback is invoked
		// with all the diff chunks between the image and the snapshot. To prevent
		// processing all the chunks, an error will be forced immediately.
		config := rbd.DiffIterateConfig{
			SnapName:      snapInfo.Name,
			Offset:        0,
			Length:        imgSize,
			IncludeParent: rbd.IncludeParent,
			WholeObject:   rbd.EnableWholeObject,
			Data:          snapIsDiff,
			Callback: func(offset, length uint64, exists int, data interface{}) int {
				d, ok := data.(*diffResult)
				if !ok {
					return -1
				}
				d.detected = true  // mark that diff is detected
				d.earlyExit = true // mark that we are exiting early

				// return an error to force stop processing diffs since
				// we only care about the first one.
				return -1
			},
		}

		err := img.DiffIterate(config)
		// The error is only real if it wasn't an early exit.
		if err != nil && !snapIsDiff.earlyExit {
			return "", fmt.Errorf("failed to iterate snapshot diffs on %q: %w", snapInfo.Name, err)
		}

		// If there was no difference detected then this
		// snapshot can be used for cloning.
		if !snapIsDiff.detected {
			return snapInfo.Name, nil
		}
	}

	// If we are still here then there are no existing snapshots of the
	// image in its current state.
	return "", errNoSnapshot
}

// generateSnapshot generates a new protected snapshot of the image and returns the snapshot name.
func generateSnapshot(ioctx *rados.IOContext, imgName string) (string, error) {
	// Open the source image and lock it before starting.
	img, err := rbd.OpenImage(ioctx, imgName, rbd.NoSnapshot)
	if err != nil {
		return "", fmt.Errorf("volume open failure: %w", err)
	}
	defer img.Close()

	// Only generate a snapshot if the image supports layering since
	// the generated snapshots are used for cloning.
	if ok, err := imageHasFeature(img, rbd.FeatureLayering); err != nil {
		return "", err
	} else if !ok {
		return "", fmt.Errorf("volume missing required 'layering' feature for snapshot")
	}

	// Lock the image for modification.
	unlocker, err := writeLock(img)
	if err != nil {
		return "", fmt.Errorf("volume lock failure: %w", err)
	}
	defer unlocker()

	// Create a new snapshot of the source volume.
	snapshotName := fmt.Sprintf("nomad-snap-%s", uuid.Short())
	snap, err := img.CreateSnapshot(snapshotName)
	if err != nil {
		return "", fmt.Errorf("volume snapshot create failure: %w", err)
	}

	// Protect the snapshot to prevent deletion.
	if err := snap.Protect(); err != nil {
		return "", fmt.Errorf("volume snapshot protection failure: %w", err)
	}

	return snapshotName, nil
}

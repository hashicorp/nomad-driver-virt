// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

// Pool defines the required interface for supporting storage pools
type Pool interface {
	// AddVolume adds a new volume to the storage pool
	AddVolume(name string, opts Options) (*Volume, error)
	// DeleteVolume deletes a volume from the storage pool
	DeleteVolume(name string) error
}

// Options are supported options for AddVolume
type Options struct {
	Chained bool   // Volume is chained to a parent volume
	Size    string // Size of the volume (10GiB or 10GB)
	Source  Source // Describes the source of the volume
	Target  Target // Options for the target to be created
}

// Target includes options for volume creation
type Target struct {
	Format string // Format of the created volume ("raw", "qcow2", etc.)
}

// Source describes the source of the volume to create
type Source struct {
	Format   string // Format of the source
	Path     string // Path to a source image file
	Snapshot string // Snapshot to clone
	Volume   string // Volume to clone
}

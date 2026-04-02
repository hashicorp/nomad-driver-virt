// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

// Pool defines the required interface for supporting storage pools.
type Pool interface {
	// AddVolume adds a new volume to the storage pool.
	AddVolume(name string, opts Options) (*Volume, error)
	// GetVolume retrieves a volume from the storage pool if it exists.
	GetVolume(name string) (*Volume, error)
	// DeleteVolume deletes a volume from the storage pool.
	DeleteVolume(name string) error
	// Name returns the name of the storage pool.
	Name() string
	// Type returns the type of the storage pool.
	Type() string
	// DefaultImageFormat returns the default image format for the pool.
	DefaultImageFormat() string
}

// Options are supported options for AddVolume
type Options struct {
	Chained bool   // Volume is chained to a parent volume
	Sparse  bool   // Volume should be sparse (full capacity not allocated)
	Size    uint64 // Size of the volume in bytes
	Source  Source // Describes the source of the volume
	Target  Target // Options for the target to be created
}

// Target includes options for volume creation
type Target struct {
	Format string // Format of the created volume ("raw", "qcow2", etc.)
}

// Source describes the source of the volume to create
type Source struct {
	Path   string // Path to a source image file
	Volume string // Volume to clone
}

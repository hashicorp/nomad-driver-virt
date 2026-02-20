// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import "github.com/hashicorp/nomad-driver-virt/storage/image_tools"

// Storage defines the required interface for support storage
type Storage interface {
	// DefaultPool returns the default storage pool
	DefaultPool() (Pool, error)
	// GetPool returns the requested storage pool by name
	GetPool(name string) (Pool, error)
	// ImageHandler returns an image handler
	ImageHandler() image_tools.ImageHandler
	// DefaultDiskDriver provides the name of the default disk driver
	DefaultDiskDriver() string
	// GenerateDeviceName generates a new device name for a disk
	GenerateDeviceName(busType string, existingDevices []string) string
}

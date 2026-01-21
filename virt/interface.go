// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package virt

import (
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
)

// Virtualizer is the interface that defins the virtualization system.
type Virtualizer interface {
	// Start is responsible for initialzing the virtualization
	// provider. This is handled with a dedicated function to
	// allow errors to be properly returned.
	Start() error

	// CreateVM creates new virtual machine using the provided
	// configuration.
	CreateVM(config *vm.Config) error

	// StopVM stops the named virtual machine.
	StopVM(name string) error

	// DestroyVM destroys the named virtual machine.
	DestroyVM(name string) error

	// GetVM gets information about the named virtual machine.
	GetVM(name string) (*vm.Info, error)

	// GetInfo returns information about the virtualization provider.
	GetInfo() (vm.VirtualizerInfo, error)

	// GetNetworkInterfaces returns the network interfaces for the
	// name virtual machine.
	GetNetworkInterfaces(name string) ([]vm.NetworkInterface, error)

	// UseCloudInit informs if the provider supports cloud-init.
	UseCloudInit() bool
}

// VMGetter is a slim interface for retrieving information about virtual machines.
type VMGetter interface {
	// GetVM gets information about the named virtual machine.
	GetVM(name string) (*vm.Info, error)
}

// ImageHandler is the interface handling image files directly.
type ImageHandler interface {
	GetImageFormat(basePath string) (string, error)
	CreateThinCopy(basePath string, destination string, sizeM int64) error
}

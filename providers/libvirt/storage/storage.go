// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/go-hclog"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
	"github.com/hashicorp/nomad-driver-virt/storage"
	"github.com/hashicorp/nomad-driver-virt/storage/image_tools"
	"github.com/hashicorp/nomad/plugins/shared/structs"
	"libvirt.org/go/libvirtxml"
)

const (
	defaultDiskDriver = "qemu"
	providerName      = "libvirt"
)

var (
	ErrInvalidVolumeConfiguration = fmt.Errorf("%w for volume", vm.ErrInvalidConfiguration)
	ErrVolumeNotFound             = fmt.Errorf("volume %w", vm.ErrNotFound)
	ErrPoolNotFound               = fmt.Errorf("pool %w", vm.ErrNotFound)
)

// This interface defines what functions are needed from the driver
type libvirtStorage interface {
	CreateStoragePool(def libvirtxml.StoragePool) (shims.StoragePool, error)
	FindStoragePool(name string) (shims.StoragePool, error)
	NewStream() (shims.Stream, error)
}

// New creates a new storage instance
func New(logger hclog.Logger, l libvirtStorage, config *storage.Config) (*store, error) {
	logger = logger.Named("storage")
	s := &store{
		logger:       logger,
		pools:        make(map[string]storage.Pool),
		imageHandler: image_tools.NewQemuHandler(logger),
	}

	for name, d := range config.Directory {
		logger.Debug("adding new directory storage pool", "name", name, "path", d.Path)
		pool, err := newDirectoryPool(logger, l, name, d, s)
		if err != nil {
			return nil, err
		}
		s.pools[name] = pool

		if s.defaultPool == nil || d.Default {
			logger.Info("default storage pool set", "name", name)
			s.defaultPool = pool
		}
	}

	if len(config.Ceph) > 0 {
		return nil, fmt.Errorf("ceph storage pools %w", vm.ErrNotImplemented)
	}

	return s, nil
}

type store struct {
	logger       hclog.Logger
	defaultPool  storage.Pool
	pools        map[string]storage.Pool
	imageHandler image_tools.ImageHandler
}

// DefaultPool implements storage.Storage
func (s *store) DefaultPool() (storage.Pool, error) {
	return s.defaultPool, nil
}

// GetPool implements storage.Storage
func (s *store) GetPool(name string) (storage.Pool, error) {
	if pool, ok := s.pools[name]; ok {
		return pool, nil
	}

	return nil, ErrPoolNotFound
}

// DefaultDiskDriver implements storage.Storage
func (s *store) DefaultDiskDriver() string {
	return defaultDiskDriver
}

// ImageHandler implements storage.Storage
func (s *store) ImageHandler() image_tools.ImageHandler {
	return s.imageHandler
}

// GenerateDeviceName implemenets storage.Storage
func (s *store) GenerateDeviceName(busType string, existingNames []string) string {
	var prefix string
	switch busType {
	case "ide":
		prefix = "hd"
	case "virtio":
		prefix = "vd"
	default:
		prefix = "sr"
	}
	validNames := []string{}
	for _, n := range existingNames {
		n = strings.ToLower(n)
		if strings.HasPrefix(n, prefix) {
			validNames = append(validNames, n)
		}
	}

	if len(validNames) == 0 {
		return prefix + "a"
	}

	max := slices.Max(validNames)
	return prefix + string(max[len(max)-1]+1)
}

// Fingerprint implements storage.Storage
func (s *store) Fingerprint(attrs map[string]*structs.Attribute) {
	for name, pool := range s.pools {
		poolKey := fmt.Sprintf("%s.storage_pool.%s",
			vm.FingerprintAttributeKeyPrefix, name)

		attrs[poolKey] = structs.NewStringAttribute(pool.Type())
		attrs[poolKey+".provider"] = structs.NewStringAttribute(providerName)
		if s.defaultPool == pool {
			attrs[poolKey+".default"] = structs.NewBoolAttribute(true)
		}
	}
}

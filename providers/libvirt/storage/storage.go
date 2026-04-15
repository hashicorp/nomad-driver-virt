// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"context"
	"fmt"
	"maps"
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
	// Default disk driver to assign in libvirt
	defaultDiskDriver = "qemu"
	// Name of this provider
	providerName = "libvirt"
	// Value when passing no flags to libvirt
	libvirtNoFlags = 0
)

var (
	ErrInvalidStorageConfiguration = fmt.Errorf("%w for storage", vm.ErrInvalidConfiguration)
	ErrInvalidVolumeConfiguration  = fmt.Errorf("%w for volume", vm.ErrInvalidConfiguration)
	ErrVolumeNotFound              = fmt.Errorf("volume %w", vm.ErrNotFound)
	ErrPoolNotFound                = fmt.Errorf("pool %w", vm.ErrNotFound)
)

// This interface defines what functions are needed from the libvirt provider.
type libvirtStorage interface {
	CreateStoragePool(def *libvirtxml.StoragePool) (shims.StoragePool, error)
	FindStoragePool(name string) (shims.StoragePool, error)
	GetCephSecret(name string) (string, error)
	GetCephSecretID(name string) (string, error)
	NewStream() (shims.Stream, error)
	SetCephSecret(name, credential string) (string, error)
	UpdateStoragePool(def *libvirtxml.StoragePool) error
}

// New creates a new storage instance.
func New(ctx context.Context, logger hclog.Logger, l libvirtStorage, config *storage.Config) (*Storage, error) {
	logger = logger.Named("storage")
	s := &Storage{
		logger:       logger,
		pools:        make(map[string]storage.Pool),
		imageHandler: image_tools.NewQemuHandler(logger),
		l:            l,
	}

	// NOTE: the pools are sorted for iteration so pools are setup in a deterministic
	// order which helps for properly testing setup with multiple pools.
	for _, name := range slices.Sorted(maps.Keys(config.Directory)) {
		d := config.Directory[name]
		logger.Debug("adding new directory storage pool", "name", name, "path", d.Path)
		pool, err := newDirectoryPool(ctx, logger, l, name, d, s)
		if err != nil {
			return nil, err
		}
		s.pools[name] = pool
	}

	for _, name := range slices.Sorted(maps.Keys(config.Ceph)) {
		c := config.Ceph[name]
		logger.Debug("adding new ceph storage pool", "name", name)
		pool, err := newCephPool(ctx, logger, l, name, c, s)
		if err != nil {
			return nil, err
		}
		s.pools[name] = pool
	}

	// If no default pool is defined, automatically set the default
	// if there is only a single storage pool defined. If more than
	// a single storage pool is defined, force an error.
	if config.Default == "" {
		if len(s.pools) == 1 {
			for _, p := range s.pools {
				s.defaultPool = p
			}
			return s, nil
		}

		return nil, fmt.Errorf("no default pool set %w", ErrInvalidStorageConfiguration)
	}

	if p, err := s.GetPool(config.Default); err != nil {
		return nil, fmt.Errorf("cannot set default pool - %w", err)
	} else {
		s.defaultPool = p
	}

	return s, nil
}

type Storage struct {
	config       *storage.Config
	logger       hclog.Logger
	defaultPool  storage.Pool
	pools        map[string]storage.Pool
	imageHandler image_tools.ImageHandler
	l            libvirtStorage
}

// Copy creates a new copy of the storage using the new context and
// libvirtStorage interface.
func (s *Storage) Copy(ctx context.Context, l libvirtStorage) *Storage {
	newS := &Storage{
		logger:       s.logger,
		imageHandler: s.imageHandler,
		pools:        make(map[string]storage.Pool),
		l:            l,
	}

	for name, p := range s.pools {
		switch pool := p.(type) {
		case *ceph:
			newS.pools[name] = pool.copy(ctx, newS, l)
		case *directory:
			newS.pools[name] = pool.copy(ctx, newS, l)
		default:
			// NOTE: This should never happen
			panic(fmt.Sprintf("cannot copy unknown storage pool type - %T", p))
		}

		if s.defaultPool.Name() == p.Name() {
			newS.defaultPool = newS.pools[name]
		}
	}

	return newS
}

// DefaultPool returns the default storage pool.
// implements storage.Storage
func (s *Storage) DefaultPool() (storage.Pool, error) {
	if s.defaultPool == nil {
		return nil, ErrPoolNotFound
	}

	return s.defaultPool, nil
}

// GetPool returns the requested storage pool by name.
// implements storage.Storage
func (s *Storage) GetPool(name string) (storage.Pool, error) {
	if pool, ok := s.pools[name]; ok {
		return pool, nil
	}

	return nil, ErrPoolNotFound
}

// DefaultDiskDriver provides the name of the default disk driver.
// implements storage.Storage
func (s *Storage) DefaultDiskDriver() string {
	return defaultDiskDriver
}

// ImageHandler returns an image handler.
// implements storage.Storage
func (s *Storage) ImageHandler() image_tools.ImageHandler {
	return s.imageHandler
}

// GenerateDeviceName generates a new device name for a disk.
// implemenets storage.Storage
func (s *Storage) GenerateDeviceName(busType string, existingNames []string) string {
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

// Fingerprint adds fingerprint information for available storage pools.
// implements storage.Storage
func (s *Storage) Fingerprint(attrs map[string]*structs.Attribute) {
	for name, pool := range s.pools {
		poolKey := fmt.Sprintf("%s.storage_pool.%s",
			vm.FingerprintAttributeKeyPrefix, name)

		attrs[poolKey] = structs.NewStringAttribute(pool.Type())
		attrs[poolKey+".provider."+providerName] = structs.NewBoolAttribute(true)
		if s.defaultPool == pool {
			attrs[poolKey+".default"] = structs.NewBoolAttribute(true)
		}
	}
}

// ListPools returns the name of available storage pools.
// implements storage.Storage
func (s *Storage) ListPools() []string {
	return slices.Sorted(maps.Keys(s.pools))
}

// VolumeToDisk will convert a storage volume into a domain disk.
// NOTE: Volumes _should_ be consistently converted into disk configuration,
// however, some pools don't support volumes backing disks which is a sad
// state of affairs.
func (s *Storage) VolumeToDisk(vol storage.Volume) (*libvirtxml.DomainDisk, error) {
	// Sometimes after uploading an image into a volume, libvirt will overwrite
	// the volume type from raw to iso. Check for that when setting the format.
	diskFmt := vol.Format
	if diskFmt == "iso" {
		diskFmt = "raw"
	}

	disk := &libvirtxml.DomainDisk{
		Device: vol.Kind,
		Driver: &libvirtxml.DomainDiskDriver{
			Name: vol.Driver,
			Type: diskFmt,
		},
		Target: &libvirtxml.DomainDiskTarget{
			Dev: vol.DeviceName,
			Bus: vol.BusType,
		},
	}

	// If volume is a nomad volume, set the source and return.
	if vol.Block != "" {
		disk.Source = &libvirtxml.DomainDiskSource{
			Block: &libvirtxml.DomainDiskSourceBlock{
				Dev: vol.Block,
			},
		}

		return disk, nil
	}

	pool, err := s.GetPool(vol.Pool)
	if err != nil {
		return nil, err
	}

	switch pool.Type() {
	case storage.PoolTypeDirectory:
		disk.Source = &libvirtxml.DomainDiskSource{
			Volume: &libvirtxml.DomainDiskSourceVolume{
				Pool:   vol.Pool,
				Volume: vol.Name,
			},
		}
	case storage.PoolTypeCeph:
		p, err := s.l.FindStoragePool(vol.Pool)
		if err != nil {
			return nil, err
		}
		defer p.Free()
		info, err := getPoolInfo(p)
		if err != nil {
			return nil, err
		}
		secretId, err := s.l.GetCephSecretID(vol.Pool)
		if err != nil {
			return nil, err
		}

		hosts := make([]libvirtxml.DomainDiskSourceHost, len(info.Source.Host))
		for i := range len(info.Source.Host) {
			h := info.Source.Host[i]
			hosts[i] = libvirtxml.DomainDiskSourceHost{
				Name: h.Name,
				Port: h.Port,
			}
		}

		disk.Source = &libvirtxml.DomainDiskSource{
			Network: &libvirtxml.DomainDiskSourceNetwork{
				Protocol: "rbd",
				Name:     fmt.Sprintf("%s/%s", info.Source.Name, vol.Name),
				Hosts:    hosts,
				Auth: &libvirtxml.DomainDiskAuth{
					Username: info.Source.Auth.Username,
					Secret: &libvirtxml.DomainDiskSecret{
						Type: "ceph",
						UUID: secretId,
					},
				},
			},
		}
	default:
		return nil, fmt.Errorf("%w: unknown storage type - %s", vm.ErrNotImplemented, pool.Type())
	}

	return disk, nil
}

// DiscoverVolumes will inspect a disk collection and return the set of storage
// volumes it represents.
func (s *Storage) DiscoverVolumes(disks []libvirtxml.DomainDisk) ([]storage.Volume, error) {
	vols := make([]storage.Volume, 0)

	cephPools := make(map[string]string)
	for name, pool := range s.pools {
		if pool.Type() != storage.PoolTypeCeph {
			continue
		}
		p, err := s.l.FindStoragePool(name)
		if err != nil {
			return nil, err
		}
		defer p.Free()
		info, err := getPoolInfo(p)
		if err != nil {
			return nil, err
		}
		cephPools[info.Source.Name] = name
	}

	for _, disk := range disks {
		// If no source is configured on disk it can't be handled
		if disk.Source == nil {
			continue
		}

		// Disk is properly configured as backed by storage pool volume
		if disk.Source.Volume != nil {
			vols = append(vols, storage.Volume{
				Pool: disk.Source.Volume.Pool,
				Name: disk.Source.Volume.Volume,
			})

			continue
		}

		// Disk is Ceph volume
		if disk.Source.Network != nil && disk.Source.Network.Protocol == "rbd" {
			nameParts := strings.Split(disk.Source.Network.Name, "/")
			if len(nameParts) < 2 {
				return nil, fmt.Errorf("invalid rbd source name - %s", disk.Source.Network.Name)
			}
			cephPool := nameParts[0]
			img := strings.Join(nameParts[1:], "/")
			localPool, ok := cephPools[cephPool]
			if !ok {
				return nil, fmt.Errorf("failed to find rbd source storage pool - %s", disk.Source.Network.Name)
			}

			vols = append(vols, storage.Volume{
				Pool: localPool,
				Name: img,
			})

			continue
		}
		s.logger.Debug("cannot detect volume from disk", "disk", hclog.Fmt("%#v", disk))
	}

	return vols, nil
}

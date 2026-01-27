// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package providers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/hashicorp/go-hclog"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt"
	"github.com/hashicorp/nomad-driver-virt/virt"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

var (
	ErrUnavailableProvider = errors.New("requested provider is not available")
	ErrNoProvidersEnabled  = errors.New("no providers enabled in configuration")
)

// Providers is the interface for interacting with virt.Virtualizer
// implementations.
type Providers interface {
	// Setup is responsible for initializing any providers defined
	// within the configuration.
	Setup(config *virt.Config) error
	// Get will return the named provider if it is available.
	Get(name string) (virt.Virtualizer, error)
	// Default will return the provider selected as the default.
	Default() (virt.Virtualizer, error)
	// GetVM will return the virtual machine information for the
	// named virtual machine.
	GetVM(name string) (*vm.Info, error)
	// GetProviderForVM will return the virt.Virtualizer responsible
	// for the named virtual machine.
	GetProviderForVM(name string) (virt.Virtualizer, error)
	// Fingerprint will generate node fingerprint information based
	// on available providers.
	Fingerprint() (*drivers.Fingerprint, error)
}

type dispenseProvider func() (virt.Virtualizer, error)

// New creates a new providers instance.
func New(ctx context.Context, logger hclog.Logger) Providers {
	return &providers{
		ctx:        ctx,
		dispensers: make(map[string]dispenseProvider),
		logger:     logger.Named("providers"),
	}
}

type providers struct {
	ctx              context.Context
	logger           hclog.Logger
	defaultDispenser dispenseProvider
	dispensers       map[string]dispenseProvider
	l                sync.RWMutex
}

func (p *providers) Setup(config *virt.Config) error {
	p.l.Lock()
	defer p.l.Unlock()

	if config == nil || config.Provider == nil {
		return ErrNoProvidersEnabled
	}

	dispensers := make(map[string]dispenseProvider)

	if config.Provider.Libvirt != nil {
		// Create an instance and perform initialization
		lv := libvirt.New(p.ctx, p.logger, libvirt.WithDataDir(config.DataDir),
			libvirt.WithConfig(config.Provider.Libvirt))
		if err := lv.Init(); err != nil {
			return err
		}

		// Load the networking sub-system and initialize
		lvnet, err := lv.Networking()
		if err != nil {
			return err
		}
		if err := lvnet.Init(); err != nil {
			return err
		}

		// Add the dispenser for the libvirt provider
		dispensers["libvirt"] = func() (virt.Virtualizer, error) { return lv.Copy(), nil }

		// If marked as the default, set it
		if config.Provider.Libvirt.Default {
			p.defaultDispenser = dispensers["libvirt"]
		}
	}

	if len(dispensers) == 0 {
		return ErrNoProvidersEnabled
	}

	// If no default was defined, set one
	if p.defaultDispenser == nil {
		for name, fn := range dispensers {
			p.logger.Info("default provider automatically set", "provider", name)
			p.defaultDispenser = fn
			break
		}
	}

	p.dispensers = dispensers

	return nil
}

func (p *providers) Fingerprint() (*drivers.Fingerprint, error) {
	p.l.RLock()
	defer p.l.RUnlock()

	// Start with marking the virt driver in the attributes:
	//
	//   drivers.virt = true
	attrs := map[string]*structs.Attribute{
		vm.FingerprintAttributeKeyPrefix: structs.NewBoolAttribute(true),
	}

	// Get fingerprint information for all available providers
	for name, dispense := range p.dispensers {
		pv, err := dispense()
		if err != nil {
			return nil, err
		}

		// Generate the prefix for this provider and mark it in the attributes:
		//
		//   drivers.virt.provider.libvirt = true
		keyPrefix := fmt.Sprintf("%s.provider.%s", vm.FingerprintAttributeKeyPrefix, name)
		attrs[keyPrefix] = structs.NewBoolAttribute(true)

		pvAttrs, err := pv.Fingerprint()
		if err != nil {
			return nil, err
		}

		// Add the returned attributes. If the name starts with the root prefix,
		// leave it as-is. Otherwise, add the provider specific prefix to the name.
		for key, value := range pvAttrs {
			if strings.HasPrefix(key, vm.FingerprintAttributeKeyPrefix) {
				attrs[key] = value
			} else {
				attrs[fmt.Sprintf("%s.%s", keyPrefix, key)] = value
			}
		}
	}

	return &drivers.Fingerprint{
		Attributes:        attrs,
		Health:            drivers.HealthStateHealthy,
		HealthDescription: drivers.DriverHealthy,
	}, nil
}

func (p *providers) Get(name string) (virt.Virtualizer, error) {
	p.l.RLock()
	defer p.l.RUnlock()

	dispense, ok := p.dispensers[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnavailableProvider, name)
	}
	return dispense()
}

func (p *providers) Default() (virt.Virtualizer, error) {
	p.l.RLock()
	defer p.l.RUnlock()

	if p.defaultDispenser == nil {
		return nil, ErrUnavailableProvider
	}

	return p.defaultDispenser()
}

func (p *providers) GetVM(name string) (*vm.Info, error) {
	p.l.RLock()
	defer p.l.RUnlock()

	for _, dispense := range p.dispensers {
		pv, err := dispense()
		if err != nil {
			return nil, err
		}

		info, err := pv.GetVM(name)
		if err != nil {
			if !errors.Is(err, vm.ErrNotFound) {
				return nil, err
			}

			continue
		}

		return info, err
	}

	return nil, vm.ErrNotFound
}

func (p *providers) GetProviderForVM(name string) (virt.Virtualizer, error) {
	p.l.RLock()
	defer p.l.RUnlock()

	for _, dispense := range p.dispensers {
		pv, err := dispense()
		if err != nil {
			return nil, err
		}

		_, err = pv.GetVM(name)
		if err != nil {
			if !errors.Is(err, vm.ErrNotFound) {
				return nil, err
			}

			continue
		}

		return pv, nil
	}

	return nil, vm.ErrNotFound
}

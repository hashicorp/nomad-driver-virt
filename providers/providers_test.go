// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package providers

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/go-hclog"
	vm "github.com/hashicorp/nomad-driver-virt/internal/shared"
	mock_virtualizers "github.com/hashicorp/nomad-driver-virt/testutil/mock/virt"
	"github.com/hashicorp/nomad-driver-virt/virt"
	"github.com/hashicorp/nomad/plugins/shared/structs"
	"github.com/shoenig/test/must"
)

func TestProviders(t *testing.T) {
	ctx := context.Background()
	logger := hclog.NewNullLogger()
	stub := mock_virtualizers.NewStatic()
	vstub := virt.Virtualizer(stub)

	t.Run("Setup", func(t *testing.T) {
		t.Run("without config", func(t *testing.T) {
			p := New(ctx, logger)
			err := p.Setup(nil)
			must.ErrorIs(t, err, ErrNoProvidersEnabled)
		})

		t.Run("with empty config", func(t *testing.T) {
			p := New(ctx, logger)
			err := p.Setup(&virt.Config{})
			must.ErrorIs(t, err, ErrNoProvidersEnabled)
		})

		// NOTE: Running setup with config will init providers, which
		// can cause issues due to things like networking initializations.
		// Should we include a Teardown on the net interface as a complement
		// to Init?
	})

	t.Run("Get", func(t *testing.T) {
		p := New(ctx, logger)
		stubProvider(p, "test-virt", stub)

		t.Run("existing provider", func(t *testing.T) {
			pv, err := p.Get("test-virt")
			must.NoError(t, err)
			must.Eq(t, vstub, pv)
		})

		t.Run("missing provider", func(t *testing.T) {
			pv, err := p.Get("unknown")
			must.ErrorIs(t, err, ErrUnavailableProvider)
			must.Nil(t, pv)
		})
	})

	t.Run("Default", func(t *testing.T) {
		t.Run("no providers", func(t *testing.T) {
			p := New(ctx, logger)
			pv, err := p.Default()
			must.ErrorIs(t, err, ErrUnavailableProvider)
			must.Nil(t, pv)
		})

		t.Run("with providers", func(t *testing.T) {
			p := New(ctx, logger)
			stubProvider(p, "test-virt", stub)
			pv, err := p.Default()
			must.NoError(t, err)
			must.Eq(t, vstub, pv)
		})
	})

	t.Run("GetVM", func(t *testing.T) {
		t.Run("no providers", func(t *testing.T) {
			p := New(ctx, logger)
			v, err := p.GetVM("test")
			must.ErrorIs(t, err, vm.ErrNotFound)
			must.Nil(t, v)
		})

		t.Run("with providers", func(t *testing.T) {
			p := New(ctx, logger)
			stubProvider(p, "v1", mock_virtualizers.NewStatic())
			v2 := mock_virtualizers.NewStatic()
			stubProvider(p, "v2", v2)

			t.Run("when VM is not registered", func(t *testing.T) {
				v, err := p.GetVM("test")
				must.ErrorIs(t, err, vm.ErrNotFound)
				must.Nil(t, v)
			})

			t.Run("when VM is registered", func(t *testing.T) {
				v2.GetVMResult = &vm.Info{State: vm.VMStateRunning}
				v, err := p.GetVM("test")
				must.NoError(t, err)
				must.NotNil(t, v)
				must.Eq(t, vm.VMStateRunning, v.State)
			})
		})
	})

	t.Run("GetProviderForVM", func(t *testing.T) {
		t.Run("no providers", func(t *testing.T) {
			p := New(ctx, logger)
			v, err := p.GetProviderForVM("test-vm")
			must.ErrorIs(t, err, vm.ErrNotFound)
			must.Nil(t, v)
		})

		t.Run("no matching providers", func(t *testing.T) {
			p := New(ctx, logger)
			stubProvider(p, "test-virt", mock_virtualizers.NewStatic())
			stubProvider(p, "other-virt", mock_virtualizers.NewStatic())
			v, err := p.GetProviderForVM("test-vm")
			must.ErrorIs(t, err, vm.ErrNotFound)
			must.Nil(t, v)
		})

		t.Run("with matching provider", func(t *testing.T) {
			p := New(ctx, logger)
			stubProvider(p, "test-virt", mock_virtualizers.NewStatic())
			stub := &mock_virtualizers.StaticVirt{GetVMResult: &vm.Info{}}
			stubProvider(p, "other-virt", stub)

			v, err := p.GetProviderForVM("test-vm")
			must.NoError(t, err)
			must.Eq(t, virt.Virtualizer(stub), v)
		})
	})

	t.Run("Fingerprint", func(t *testing.T) {
		first_stub := &mock_virtualizers.StaticVirt{
			FingerprintResult: map[string]*structs.Attribute{
				"test":               structs.NewStringAttribute("test-value"),
				"driver.virt.manual": structs.NewStringAttribute("no-key-prefix"),
			},
		}
		second_stub := &mock_virtualizers.StaticVirt{
			FingerprintResult: map[string]*structs.Attribute{
				"test": structs.NewStringAttribute("other-value"),
			},
		}

		t.Run("with single provider", func(t *testing.T) {
			p := New(ctx, logger)
			stubProvider(p, "first-stub", first_stub)
			res, err := p.Fingerprint()
			must.NoError(t, err)
			must.MapLen(t, 4, res.Attributes)
			must.MapContainsKey(t, res.Attributes, "driver.virt")
			must.True(t, *res.Attributes["driver.virt"].Bool)
			must.MapContainsKey(t, res.Attributes, "driver.virt.provider.first-stub")
			must.True(t, *res.Attributes["driver.virt.provider.first-stub"].Bool)
			must.MapContainsKey(t, res.Attributes, "driver.virt.provider.first-stub.test")
			must.Eq(t, "test-value", *res.Attributes["driver.virt.provider.first-stub.test"].String)
			must.MapContainsKey(t, res.Attributes, "driver.virt.manual")
			must.Eq(t, "no-key-prefix", *res.Attributes["driver.virt.manual"].String)
		})

		t.Run("with multiple providers", func(t *testing.T) {
			p := New(ctx, logger)
			stubProvider(p, "first-stub", first_stub)
			stubProvider(p, "second-stub", second_stub)
			res, err := p.Fingerprint()
			must.NoError(t, err)
			must.MapLen(t, 6, res.Attributes)
			must.MapContainsKey(t, res.Attributes, "driver.virt")
			must.True(t, *res.Attributes["driver.virt"].Bool)
			must.MapContainsKey(t, res.Attributes, "driver.virt.provider.first-stub")
			must.True(t, *res.Attributes["driver.virt.provider.first-stub"].Bool)
			must.MapContainsKey(t, res.Attributes, "driver.virt.provider.second-stub")
			must.True(t, *res.Attributes["driver.virt.provider.second-stub"].Bool)
			must.MapContainsKey(t, res.Attributes, "driver.virt.provider.first-stub.test")
			must.Eq(t, "test-value", *res.Attributes["driver.virt.provider.first-stub.test"].String)
			must.MapContainsKey(t, res.Attributes, "driver.virt.manual")
			must.Eq(t, "no-key-prefix", *res.Attributes["driver.virt.manual"].String)
			must.MapContainsKey(t, res.Attributes, "driver.virt.provider.second-stub.test")
			must.Eq(t, "other-value", *res.Attributes["driver.virt.provider.second-stub.test"].String)
		})
	})
}

func stubProvider(p Providers, name string, provider virt.Virtualizer) {
	ps, ok := p.(*providers)
	if !ok {
		panic(fmt.Sprintf("expected *providers type but received %T", p))
	}
	if ps.dispensers == nil {
		ps.dispensers = make(map[string]dispenseProvider)
	}
	ps.dispensers[name] = func() (virt.Virtualizer, error) { return provider, nil }

	if ps.defaultDispenser == nil {
		ps.defaultDispenser = ps.dispensers[name]
	}
}

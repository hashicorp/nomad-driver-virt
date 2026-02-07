// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/cloudinit"
	domain "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/hashicorp/nomad-driver-virt/providers/libvirt/shims"
	"github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/shoenig/test/must"
)

// validate the driver implements the connect interface
var _ shims.Connect = (*driver)(nil)

type cloudInitMock struct {
	passedConfig *cloudinit.Config
	err          error
}

func (cim *cloudInitMock) Apply(ci *cloudinit.Config, path string) error {
	if err := os.WriteFile(path, []byte("Hello, World!"), 0644); err != nil {
		return err
	}

	cim.passedConfig = ci

	return cim.err
}

func TestGetInfo(t *testing.T) {
	// The "test:///default" uri connects to a mock hypervisor provided by libvirt
	// to use for testing.
	ld := New(context.Background(), hclog.NewNullLogger(), WithConnectionURI("test:///default"))

	err := ld.Init()

	must.NoError(t, err)
	i, err := ld.GetInfo()
	must.NoError(t, err)

	must.NonZero(t, i.LibvirtVersion)
	must.NonZero(t, i.EmulatorVersion)
	must.NonZero(t, i.StoragePools)
	// The test driver has one running  machine.
	must.One(t, i.RunningDomains)
	must.Zero(t, i.InactiveDomains)

	ld.Close()
}

func TestStartDomain(t *testing.T) {
	t.Parallel()

	makeConfig := func() *domain.Config {
		return &domain.Config{
			Memory:   66600,
			CPUs:     2,
			HostName: "test-hostname",
			SSHKey:   "sshkey lkbfubwfu...",
			Password: "test-password",
			CMDs:     []string{"cmd arg arg", "cmd arg arg"},
			BOOTCMDs: []string{"cmd arg arg", "cmd arg arg"},
			Mounts: []domain.MountFileConfig{
				{
					Source:      "/mount/source/one",
					Destination: "/path/to/file/one",
					Tag:         "tagOne",
					ReadOnly:    true,
				},
				{Source: "/mount/source/two",
					Destination: "/path/to/file/two",
					Tag:         "tagTwo",
					ReadOnly:    false},
			},
			Files: []domain.File{
				{
					Path:        "/path/to/file/one",
					Content:     "content",
					Permissions: "444",
					Encoding:    "b64",
				},
				{
					Path:        "/path/to/file/two",
					Content:     "content",
					Permissions: "666",
				},
			},
		}

	}

	t.Run("domain created successfully", func(t *testing.T) {
		ld := New(context.Background(), hclog.NewNullLogger(),
			WithConnectionURI(TestURI))
		err := ld.Init()
		must.NoError(t, err)
		defer ld.Close()

		i, err := ld.GetInfo()
		must.NoError(t, err)
		existingRunning := i.RunningDomains

		domConfig := makeConfig()
		domConfig.Name = "domain-1"

		must.NoError(t, ld.CreateVM(domConfig))
		i, err = ld.GetInfo()
		must.NoError(t, err)
		must.One(t, i.RunningDomains-existingRunning)
	})

	t.Run("duplicated domain error", func(t *testing.T) {
		ld := New(context.Background(), hclog.NewNullLogger(),
			WithConnectionURI(TestURI))
		err := ld.Init()
		must.NoError(t, err)
		defer ld.Close()

		i, err := ld.GetInfo()
		must.NoError(t, err)
		existingRunning := i.RunningDomains

		domConfig := makeConfig()
		domConfig.Name = "domain-1"

		must.NoError(t, ld.CreateVM(domConfig))
		i, err = ld.GetInfo()
		must.NoError(t, err)
		must.One(t, i.RunningDomains-existingRunning)

		// try again
		err = ld.CreateVM(domConfig)
		must.ErrorIs(t, err, ErrDomainExists)
	})
}

func Test_CreateStopAndDestroyDomain(t *testing.T) {
	// The "test:///default" uri connects to a mock hypervisor provided by libvirt
	// to use for testing.
	ld := New(context.Background(), hclog.NewNullLogger(),
		WithConnectionURI("test:///default"))
	err := ld.Init()
	must.NoError(t, err)
	defer ld.Close()

	info, err := ld.GetInfo()
	must.NoError(t, err)

	must.Zero(t, info.InactiveDomains)

	doms, err := ld.GetAllDomains()
	must.NoError(t, err)

	// The test hypervisor has one running  machine from the start.
	must.Len(t, 1, doms)

	domainName := "test-nomad-domain"
	err = ld.CreateVM(&domain.Config{
		RemoveConfigFiles: true,
		Name:              domainName,
		Memory:            66600,
		CPUs:              6,
	})
	must.NoError(t, err)

	doms, err = ld.GetAllDomains()
	must.NoError(t, err)

	// The initial test hypervisor has one plus the one that was just started.
	must.Len(t, 2, doms)

	err = ld.StopVM(domainName)
	must.NoError(t, err)

	info, err = ld.GetInfo()
	must.NoError(t, err)

	// The stopped domain.
	must.One(t, info.InactiveDomains)

	doms, err = ld.GetAllDomains()
	must.NoError(t, err)

	// Back to the initial test hypervisor one.
	must.Len(t, 1, doms)

	info, err = ld.GetInfo()
	must.NoError(t, err)
	// The domain is still present, but inactive
	must.One(t, info.InactiveDomains)

	err = ld.DestroyVM(domainName)
	must.NoError(t, err)

	info, err = ld.GetInfo()
	must.NoError(t, err)

	// The domain is present as inactive anymore.
	must.Zero(t, info.InactiveDomains)
}

func Test_GetNetworkInterfaces(t *testing.T) {
	// The "test:///default" uri connects to a mock hypervisor provided by libvirt
	// to use for testing.
	ld := New(context.Background(), hclog.NewNullLogger(),
		WithConnectionURI("test:///default"))
	err := ld.Init()
	must.NoError(t, err)
	defer ld.Close()

	domainName := "test-nomad-domain"
	err = ld.CreateVM(&domain.Config{
		RemoveConfigFiles: true,
		Name:              domainName,
		Memory:            66600,
		CPUs:              6,
		NetworkInterfaces: []*net.NetworkInterfaceConfig{
			{
				Bridge: &net.NetworkInterfaceBridgeConfig{
					Name: "testbr0",
				},
			},
		},
	})
	must.NoError(t, err)

	interfaces, err := ld.GetNetworkInterfaces(domainName)
	must.NoError(t, err)
	must.Len(t, 1, interfaces)
	must.StrContains(t, interfaces[0].MAC, ":")
}

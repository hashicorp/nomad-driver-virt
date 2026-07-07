// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package linux

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	"github.com/hashicorp/nomad-driver-virt/net/filter/linux/shared"
	"github.com/hashicorp/nomad-driver-virt/testutil/mock/net/filter/backend"
	virtnet "github.com/hashicorp/nomad-driver-virt/virt/net"
	"github.com/shoenig/test/must"
)

func Test_tables_Configure(t *testing.T) {
	testErr := errors.New("test error")

	testCases := []struct {
		name     string
		mappings virtnet.PortMappings
		cfg      *virtnet.NetworkInterfaceBridgeConfig
		ip       string
		identity string
		backend  Backend
		err      error
		errStr   string
		result   *virtnet.FilterRemoval
		modifyFn func(*testing.T, *tables)
	}{
		{
			name:     "ok",
			cfg:      &virtnet.NetworkInterfaceBridgeConfig{},
			ip:       "10.1.1.2",
			identity: "test-ident",
			result: &virtnet.FilterRemoval{
				Name: "backend:test-backend",
				Data: shared.Teardown{
					Identifier:  "nmd-id:test-ident",
					RulesWithin: []shared.Chain{},
				},
			},
		},
		{
			name:     "ok - single forward",
			ip:       "10.1.1.2",
			identity: "test-ident",
			cfg: &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"test"},
			},
			mappings: virtnet.PortMappings{
				{
					Label:  "test",
					Value:  8080,
					To:     9090,
					HostIP: "10.10.1.20",
				},
			},
			result: &virtnet.FilterRemoval{
				Name: "backend:test-backend",
				Data: shared.Teardown{
					Identifier: "nmd-id:test-ident",
					RulesWithin: []shared.Chain{
						{Table: "nat", Chain: "NOMAD_VT_PRT"},
						{Table: "filter", Chain: "NOMAD_VT_FW"},
					},
				},
			},
			backend: backend.NewMock(t).Expect(
				backend.Add{
					Chains: []shared.Chain{},
					Rules: []shared.Rule{
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "NOMAD_VT_PRT",
							},
							Spec: shared.Spec{
								Destination:     "10.10.1.20",
								InInterface:     "test0",
								Protocol:        "tcp",
								DestinationPort: 8080,
								Jump:            "DNAT",
								ToDestination:   "10.1.1.2:9090",
							},
							Identifier: "nmd-id:test-ident",
							Removable:  true,
						},
						{
							Chain: shared.Chain{
								Table: "filter",
								Chain: "NOMAD_VT_FW",
							},
							Spec: shared.Spec{
								Destination:     "10.1.1.2",
								Protocol:        "tcp",
								State:           "NEW",
								DestinationPort: 9090,
								Jump:            "ACCEPT",
							},
							Identifier: "nmd-id:test-ident",
							Removable:  true,
						},
					},
				},
				backend.Name{Name: "test-backend"},
			),
		},
		{
			name:     "ok - multiple forwards",
			ip:       "10.1.1.2",
			identity: "test-ident",
			cfg: &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"test", "other-test"},
			},
			mappings: virtnet.PortMappings{
				{
					Label:  "test",
					Value:  8080,
					To:     9090,
					HostIP: "10.10.1.20",
				},
				{
					Label:  "other-test",
					Value:  8888,
					To:     9999,
					HostIP: "10.10.1.20",
				},
			},
			result: &virtnet.FilterRemoval{
				Name: "backend:test-backend",
				Data: shared.Teardown{
					Identifier: "nmd-id:test-ident",
					RulesWithin: []shared.Chain{
						{Table: "nat", Chain: "NOMAD_VT_PRT"},
						{Table: "filter", Chain: "NOMAD_VT_FW"},
					},
				},
			},
			backend: backend.NewMock(t).Expect(
				backend.Add{
					Chains: []shared.Chain{},
					Rules: []shared.Rule{
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "NOMAD_VT_PRT",
							},
							Spec: shared.Spec{
								Destination:     "10.10.1.20",
								InInterface:     "test0",
								Protocol:        "tcp",
								DestinationPort: 8080,
								Jump:            "DNAT",
								ToDestination:   "10.1.1.2:9090",
							},
							Identifier: "nmd-id:test-ident",
							Removable:  true,
						},
						{
							Chain: shared.Chain{
								Table: "filter",
								Chain: "NOMAD_VT_FW",
							},
							Spec: shared.Spec{
								Destination:     "10.1.1.2",
								Protocol:        "tcp",
								State:           "NEW",
								DestinationPort: 9090,
								Jump:            "ACCEPT",
							},
							Identifier: "nmd-id:test-ident",
							Removable:  true,
						},
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "NOMAD_VT_PRT",
							},
							Spec: shared.Spec{
								Destination:     "10.10.1.20",
								InInterface:     "test0",
								Protocol:        "tcp",
								DestinationPort: 8888,
								Jump:            "DNAT",
								ToDestination:   "10.1.1.2:9999",
							},
							Identifier: "nmd-id:test-ident",
							Removable:  true,
						},
						{
							Chain: shared.Chain{
								Table: "filter",
								Chain: "NOMAD_VT_FW",
							},
							Spec: shared.Spec{
								Destination:     "10.1.1.2",
								Protocol:        "tcp",
								State:           "NEW",
								DestinationPort: 9999,
								Jump:            "ACCEPT",
							},
							Identifier: "nmd-id:test-ident",
							Removable:  true,
						},
					},
				},
				backend.Name{Name: "test-backend"},
			),
		},
		{
			name:     "ok - single loopback forward",
			ip:       "10.1.1.2",
			identity: "test-ident",
			cfg: &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"test"},
			},
			mappings: virtnet.PortMappings{
				{
					Label:  "test",
					Value:  8080,
					To:     9090,
					HostIP: "127.1.3.20",
				},
			},
			result: &virtnet.FilterRemoval{
				Name: "backend:test-backend",
				Data: shared.Teardown{
					Identifier: "nmd-id:test-ident",
					RulesWithin: []shared.Chain{
						{Table: "nat", Chain: "NOMAD_VT_OUT"},
					},
				},
			},
			backend: backend.NewMock(t).Expect(
				backend.Add{
					Chains: []shared.Chain{
						{
							Table: "nat",
							Chain: "NOMAD_VT_OUT",
						},
						{
							Table: "nat",
							Chain: "NOMAD_VT_PST",
						},
					},
					Rules: []shared.Rule{
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "OUTPUT",
							},
							Spec: shared.Spec{
								Jump: "NOMAD_VT_OUT",
							},
						},
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "POSTROUTING",
							},
							Spec: shared.Spec{
								Jump: "NOMAD_VT_PST",
							},
						},
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "NOMAD_VT_PST",
							},
							Spec: shared.Spec{
								OutInterface:    "testRoute0",
								SourceType:      "LOCAL",
								DestinationType: "UNICAST",
								Jump:            "MASQUERADE",
							},
						},
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "NOMAD_VT_OUT",
							},
							Spec: shared.Spec{
								DestinationPort: 8080,
								Jump:            "DNAT",
								OutInterface:    "test0",
								Protocol:        "tcp",
								Source:          "127.1.3.20",
								ToDestination:   "10.1.1.2:9090",
							},
							Identifier: "nmd-id:test-ident",
							Removable:  true,
						},
					},
				},
				backend.Name{Name: "test-backend"},
			),
			modifyFn: func(t *testing.T, tt *tables) {
				dir := t.TempDir()
				tt.routeLocalnetPathTemplate = filepath.Join(dir, "%s.device")
				f, err := os.Create(filepath.Join(dir, "testRoute0.device"))
				if err != nil {
					t.Fatalf("setup failure: %s", err)
				}
				f.WriteString("1")
				f.Close()
			},
		},
		{
			name:     "ok - multiple loopback forwards",
			ip:       "10.1.1.2",
			identity: "test-ident",
			cfg: &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"test", "other-test"},
			},
			mappings: virtnet.PortMappings{
				{
					Label:  "test",
					Value:  8080,
					To:     9090,
					HostIP: "127.1.3.20",
				},
				{
					Label:  "other-test",
					Value:  8888,
					To:     9999,
					HostIP: "127.1.3.20",
				},
			},
			result: &virtnet.FilterRemoval{
				Name: "backend:test-backend",
				Data: shared.Teardown{
					Identifier: "nmd-id:test-ident",
					RulesWithin: []shared.Chain{
						{Table: "nat", Chain: "NOMAD_VT_OUT"},
					},
				},
			},
			backend: backend.NewMock(t).Expect(
				backend.Add{
					Chains: []shared.Chain{
						{
							Table: "nat",
							Chain: "NOMAD_VT_OUT",
						},
						{
							Table: "nat",
							Chain: "NOMAD_VT_PST",
						},
					},
					Rules: []shared.Rule{
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "OUTPUT",
							},
							Spec: shared.Spec{
								Jump: "NOMAD_VT_OUT",
							},
						},
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "POSTROUTING",
							},
							Spec: shared.Spec{
								Jump: "NOMAD_VT_PST",
							},
						},
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "NOMAD_VT_PST",
							},
							Spec: shared.Spec{
								OutInterface:    "testRoute0",
								SourceType:      "LOCAL",
								DestinationType: "UNICAST",
								Jump:            "MASQUERADE",
							},
						},
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "NOMAD_VT_OUT",
							},
							Spec: shared.Spec{
								DestinationPort: 8080,
								Jump:            "DNAT",
								OutInterface:    "test0",
								Protocol:        "tcp",
								Source:          "127.1.3.20",
								ToDestination:   "10.1.1.2:9090",
							},
							Identifier: "nmd-id:test-ident",
							Removable:  true,
						},
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "NOMAD_VT_OUT",
							},
							Spec: shared.Spec{
								DestinationPort: 8888,
								Jump:            "DNAT",
								OutInterface:    "test0",
								Protocol:        "tcp",
								Source:          "127.1.3.20",
								ToDestination:   "10.1.1.2:9999",
							},
							Identifier: "nmd-id:test-ident",
							Removable:  true,
						},
					},
				},
				backend.Name{Name: "test-backend"},
			),
			modifyFn: func(t *testing.T, tt *tables) {
				dir := t.TempDir()
				tt.routeLocalnetPathTemplate = filepath.Join(dir, "%s.device")
				f, err := os.Create(filepath.Join(dir, "testRoute0.device"))
				if err != nil {
					t.Fatalf("setup failure: %s", err)
				}
				f.WriteString("1")
				f.Close()
			},
		},
		{
			name:     "ok - mixed forwards",
			ip:       "10.1.1.2",
			identity: "test-ident",
			cfg: &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"test", "other-test", "lo-test", "lo-other-test"},
			},
			mappings: virtnet.PortMappings{
				{
					Label:  "test",
					Value:  3030,
					To:     4040,
					HostIP: "10.10.1.20",
				},
				{
					Label:  "other-test",
					Value:  3333,
					To:     4444,
					HostIP: "10.10.1.20",
				},
				{
					Label:  "lo-test",
					Value:  8080,
					To:     9090,
					HostIP: "127.1.3.20",
				},
				{
					Label:  "lo-other-test",
					Value:  8888,
					To:     9999,
					HostIP: "127.1.3.20",
				},
			},
			result: &virtnet.FilterRemoval{
				Name: "backend:test-backend",
				Data: shared.Teardown{
					Identifier: "nmd-id:test-ident",
					RulesWithin: []shared.Chain{
						{Table: "nat", Chain: "NOMAD_VT_PRT"},
						{Table: "filter", Chain: "NOMAD_VT_FW"},
						{Table: "nat", Chain: "NOMAD_VT_OUT"},
					},
				},
			},
			backend: backend.NewMock(t).Expect(
				backend.Add{
					Chains: []shared.Chain{
						{
							Table: "nat",
							Chain: "NOMAD_VT_OUT",
						},
						{
							Table: "nat",
							Chain: "NOMAD_VT_PST",
						},
					},
					Rules: []shared.Rule{
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "NOMAD_VT_PRT",
							},
							Spec: shared.Spec{
								Destination:     "10.10.1.20",
								InInterface:     "test0",
								Protocol:        "tcp",
								DestinationPort: 3030,
								Jump:            "DNAT",
								ToDestination:   "10.1.1.2:4040",
							},
							Identifier: "nmd-id:test-ident",
							Removable:  true,
						},
						{
							Chain: shared.Chain{
								Table: "filter",
								Chain: "NOMAD_VT_FW",
							},
							Spec: shared.Spec{
								Destination:     "10.1.1.2",
								Protocol:        "tcp",
								State:           "NEW",
								DestinationPort: 4040,
								Jump:            "ACCEPT",
							},
							Identifier: "nmd-id:test-ident",
							Removable:  true,
						},
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "NOMAD_VT_PRT",
							},
							Spec: shared.Spec{
								Destination:     "10.10.1.20",
								InInterface:     "test0",
								Protocol:        "tcp",
								DestinationPort: 3333,
								Jump:            "DNAT",
								ToDestination:   "10.1.1.2:4444",
							},
							Identifier: "nmd-id:test-ident",
							Removable:  true,
						},
						{
							Chain: shared.Chain{
								Table: "filter",
								Chain: "NOMAD_VT_FW",
							},
							Spec: shared.Spec{
								Destination:     "10.1.1.2",
								Protocol:        "tcp",
								State:           "NEW",
								DestinationPort: 4444,
								Jump:            "ACCEPT",
							},
							Identifier: "nmd-id:test-ident",
							Removable:  true,
						},
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "OUTPUT",
							},
							Spec: shared.Spec{
								Jump: "NOMAD_VT_OUT",
							},
						},
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "POSTROUTING",
							},
							Spec: shared.Spec{
								Jump: "NOMAD_VT_PST",
							},
						},
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "NOMAD_VT_PST",
							},
							Spec: shared.Spec{
								OutInterface:    "testRoute0",
								SourceType:      "LOCAL",
								DestinationType: "UNICAST",
								Jump:            "MASQUERADE",
							},
						},
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "NOMAD_VT_OUT",
							},
							Spec: shared.Spec{
								DestinationPort: 8080,
								Jump:            "DNAT",
								OutInterface:    "test0",
								Protocol:        "tcp",
								Source:          "127.1.3.20",
								ToDestination:   "10.1.1.2:9090",
							},
							Identifier: "nmd-id:test-ident",
							Removable:  true,
						},
						{
							Chain: shared.Chain{
								Table: "nat",
								Chain: "NOMAD_VT_OUT",
							},
							Spec: shared.Spec{
								DestinationPort: 8888,
								Jump:            "DNAT",
								OutInterface:    "test0",
								Protocol:        "tcp",
								Source:          "127.1.3.20",
								ToDestination:   "10.1.1.2:9999",
							},
							Identifier: "nmd-id:test-ident",
							Removable:  true,
						},
					},
				},
				backend.Name{Name: "test-backend"},
			),
			modifyFn: func(t *testing.T, tt *tables) {
				dir := t.TempDir()
				tt.routeLocalnetPathTemplate = filepath.Join(dir, "%s.device")
				f, err := os.Create(filepath.Join(dir, "testRoute0.device"))
				if err != nil {
					t.Fatalf("setup failure: %s", err)
				}
				f.WriteString("1")
				f.Close()
			},
		},
		{
			name:     "error - net config",
			ip:       "10.1.1.2",
			identity: "test-ident",
			err:      errs.ErrPacketFilterConfiguration,
			errStr:   "missing bridge config",
		},
		{
			name:     "error - empty mappings",
			ip:       "10.1.1.2",
			identity: "test-ident",
			cfg: &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"test"},
			},
			err:    errs.ErrPacketFilterConfiguration,
			errStr: "missing port mappings",
		},
		{
			name:     "error - missing mapping",
			ip:       "10.1.1.2",
			identity: "test-ident",
			cfg: &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"test"},
			},
			mappings: virtnet.PortMappings{
				{
					Label: "not-test",
				},
			},
			err:    errs.ErrPacketFilterConfigure,
			errStr: "failed to find reserved port",
		},
		{
			name:     "error - no interface",
			ip:       "10.1.1.2",
			identity: "test-ident",
			cfg: &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"test"},
			},
			mappings: virtnet.PortMappings{
				{
					Label:  "test",
					Value:  8080,
					To:     9090,
					HostIP: "10.10.1.20",
				},
			},
			err:    errs.ErrPacketFilterConfigure,
			errStr: "failed to identify IP interface",
			modifyFn: func(_ *testing.T, tt *tables) {
				tt.interfaceByIPGetter = func(net.IP) (string, error) {
					return "", testErr
				}
			},
		},
		{
			name:     "error - invalid host ip",
			ip:       "10.1.1.2",
			identity: "test-ident",
			cfg: &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"test"},
			},
			mappings: virtnet.PortMappings{
				{
					Label:  "test",
					Value:  8080,
					To:     9090,
					HostIP: "10.10.1.20.99",
				},
			},
			err:    errs.ErrPacketFilterConfigure,
			errStr: "failed to parse host IP address",
		},
		{
			name:     "error - missing loopback forwarding",
			ip:       "10.1.1.2",
			identity: "test-ident",
			cfg: &virtnet.NetworkInterfaceBridgeConfig{
				Ports: []string{"test"},
			},
			mappings: virtnet.PortMappings{
				{
					Label:  "test",
					Value:  8080,
					To:     9090,
					HostIP: "127.1.3.20",
				},
			},
			err: errs.ErrLoopbackNotEnabled,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tt := testTables()
			if tc.backend != nil {
				tt.backend = tc.backend
				if mocked, ok := tc.backend.(*backend.MockBackend); ok {
					defer mocked.AssertExpectations()
				}
			}

			if tc.modifyFn != nil {
				tc.modifyFn(t, tt)
			}

			result, err := tt.Configure(tc.mappings, tc.cfg, tc.ip, tc.identity)
			if tc.err == nil && tc.errStr == "" {
				must.NoError(t, err)
			}
			if tc.err != nil {
				must.ErrorIs(t, err, tc.err)
			}
			if tc.errStr != "" {
				must.ErrorContains(t, err, tc.errStr)
			}

			must.Eq(t, tc.result, result)
		})
	}
}

func Test_tables_Teardown(t *testing.T) {
	testErr := errors.New("test error")

	testCases := []struct {
		name     string
		teardown *virtnet.FilterRemoval
		backend  Backend
		err      error
		errStr   string
	}{
		{
			name: "nil",
		},
		{
			name:     "empty",
			teardown: &virtnet.FilterRemoval{},
		},
		{
			name: "ok",
			teardown: &virtnet.FilterRemoval{
				Data: shared.Teardown{},
			},
			backend: backend.NewMock(t).Expect(
				backend.Remove{
					Teardown: shared.Teardown{},
				},
			),
		},
		{
			name:     "bad data",
			teardown: &virtnet.FilterRemoval{Data: "bad data"},
			err:      errs.ErrPacketFilterTeardown,
			errStr:   "invalid teardown data",
		},
		{
			name: "remove error",
			teardown: &virtnet.FilterRemoval{
				Data: shared.Teardown{},
			},
			backend: backend.NewMock(t).Expect(
				backend.Remove{
					Teardown: shared.Teardown{},
					Err:      testErr,
				},
			),
			err: testErr,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tt := testTables()
			if tc.backend != nil {
				tt.backend = tc.backend
				if mocked, ok := tc.backend.(*backend.MockBackend); ok {
					t.Cleanup(mocked.AssertExpectations)
				}
			}
			err := tt.Teardown(tc.teardown)
			if tc.err == nil && tc.errStr == "" {
				must.NoError(t, err)
				return
			}

			if tc.err != nil {
				must.ErrorIs(t, err, tc.err)
			}

			if tc.errStr != "" {
				must.ErrorContains(t, err, tc.errStr)
			}
		})
	}
}

func Test_virtTables_loopbackPortForwardsSupported(t *testing.T) {
	testCases := []struct {
		desc          string
		deviceContent string // empty string will result in no file
		globalContent string // empty string will result in no file
		result        bool
	}{
		{
			desc:          "global only enabled",
			globalContent: "1",
			result:        true,
		},
		{
			desc:          "global only disabled",
			globalContent: "0",
			result:        false,
		},
		{
			desc:          "device only route localnet enabled",
			deviceContent: "1",
			result:        true,
		},
		{
			desc:          "device only route localnet disabled",
			deviceContent: "0",
			result:        false,
		},
		{
			desc:          "global enabled device enabled",
			globalContent: "1",
			deviceContent: "1",
			result:        true,
		},
		{
			desc:          "global enabled device disabled",
			globalContent: "1",
			deviceContent: "0",
			result:        true,
		},
		{
			desc:          "global disabled device enabled",
			globalContent: "0",
			deviceContent: "1",
			result:        true,
		},
		{
			desc:          "global disabled device disabled",
			globalContent: "0",
			deviceContent: "0",
			result:        false,
		},
		{
			desc:   "all route localnet missing",
			result: false,
		},
	}
	deviceName := "test-dev"

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			tdir := t.TempDir()
			tmpl := filepath.Join(tdir, "/%s_route_localnet")
			devPath := fmt.Sprintf(tmpl, deviceName)
			globalPath := fmt.Sprintf(tmpl, routeLocalnetGlobalName)
			if tc.globalContent != "" {
				f, err := os.Create(globalPath)
				must.NoError(t, err)
				_, err = f.WriteString(tc.globalContent)
				must.NoError(t, err)
				f.Close()
			}

			if tc.deviceContent != "" {
				f, err := os.Create(devPath)
				must.NoError(t, err)
				_, err = f.WriteString(tc.deviceContent)
				must.NoError(t, err)
				f.Close()
			}

			tt := testTables()
			tt.routeLocalnetPathTemplate = tmpl
			must.Eq(t, tc.result, tt.loopbackPortForwardsSupported(deviceName))
		})
	}
}

func testTables() *tables {
	back := backend.NewStatic()
	back.NameResult = "test-backend"
	return &tables{
		logger:                     hclog.NewNullLogger(),
		names:                      NewNames(),
		backend:                    back,
		interfaceByIPGetter:        func(net.IP) (string, error) { return "test0", nil },
		routingInterfaceByIPGetter: func(string) (string, error) { return "testRoute0", nil },
		routeLocalnetPathTemplate:  "/dev/null",
	}
}

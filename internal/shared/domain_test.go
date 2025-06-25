// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package domain

import (
	"testing"

	"github.com/hashicorp/go-multierror"
)

func TestConfig_Validate(t *testing.T) {
	allowedPath := "/allowed/path/"

	validConfig := Config{
		Name:   "test-domain",
		CPUs:   2,
		Memory: 600,
		OsVariant: &OSVariant{
			Arch:    "x86_64",
			Machine: "pc-i440fx-2.9",
		},
	}

	tests := []struct {
		name    string
		config  Config
		wantErr error
	}{
		{
			name:    "Valid_configuration",
			config:  validConfig,
			wantErr: nil,
		},
		{
			name: "Missing_domain_name",
			config: Config{
				Memory:    validConfig.Memory,
				CPUs:      validConfig.CPUs,
				OsVariant: validConfig.OsVariant,
			},
			wantErr: multierror.Append(nil, ErrEmptyName),
		},
		{
			name: "Not_enough_memory",
			config: Config{
				Name:      validConfig.Name,
				Memory:    2,
				CPUs:      validConfig.CPUs,
				OsVariant: validConfig.OsVariant,
			},
			wantErr: multierror.Append(nil, ErrNotEnoughMemory),
		},
		{
			name: "No_cpus_assigned",
			config: Config{
				Name:      validConfig.Name,
				Memory:    validConfig.Memory,
				CPUs:      0,
				OsVariant: validConfig.OsVariant,
			},
			wantErr: multierror.Append(nil, ErrNoCPUS),
		},
		{
			name: "Incomplete_OS_variant",
			config: Config{
				Name:   validConfig.Name,
				Memory: validConfig.Memory,
				CPUs:   validConfig.CPUs,
				OsVariant: &OSVariant{
					Arch:    "",
					Machine: "",
				},
			},
			wantErr: multierror.Append(nil, ErrIncompleteOSVariant),
		},
		{
			name: "All_errors",
			config: Config{
				OsVariant: &OSVariant{
					Arch:    "",
					Machine: "",
				},
			},
			wantErr: multierror.Append(nil, ErrEmptyName,
				ErrNotEnoughMemory, ErrIncompleteOSVariant,
				ErrNoCPUS),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			err := tt.config.Validate([]string{allowedPath})
			if err != nil && tt.wantErr == nil {
				t.Errorf("expected no error, got %v", err)
			} else if err == nil && tt.wantErr != nil {
				t.Errorf("expected error, got none")
			} else if err != nil && tt.wantErr != nil && err.Error() != tt.wantErr.Error() {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

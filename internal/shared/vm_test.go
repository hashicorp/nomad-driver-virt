// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package vm

import (
	"testing"

	"github.com/shoenig/test/must"
)

func TestConfig_Validate(t *testing.T) {
	validConfig := Config{
		Name:   "test-vm",
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
		wantErr []error
	}{
		{
			name:    "Valid_configuration",
			config:  validConfig,
			wantErr: nil,
		},
		{
			name: "Missing_vm_name",
			config: Config{
				Memory:    validConfig.Memory,
				CPUs:      validConfig.CPUs,
				OsVariant: validConfig.OsVariant,
			},
			wantErr: []error{ErrEmptyName},
		},
		{
			name: "Not_enough_memory",
			config: Config{
				Name:      validConfig.Name,
				Memory:    2,
				CPUs:      validConfig.CPUs,
				OsVariant: validConfig.OsVariant,
			},
			wantErr: []error{ErrNotEnoughMemory},
		},
		{
			name: "No_cpus_assigned",
			config: Config{
				Name:      validConfig.Name,
				Memory:    validConfig.Memory,
				CPUs:      0,
				OsVariant: validConfig.OsVariant,
			},
			wantErr: []error{ErrNoCPUS},
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
			wantErr: []error{ErrIncompleteOSVariant},
		},
		{
			name: "All_errors",
			config: Config{
				OsVariant: &OSVariant{
					Arch:    "",
					Machine: "",
				},
			},
			wantErr: []error{ErrEmptyName, ErrNotEnoughMemory,
				ErrIncompleteOSVariant, ErrNoCPUS},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			err := tt.config.Validate()
			if err != nil && len(tt.wantErr) == 0 {
				t.Errorf("expected no error, got %v", err)
			} else if err == nil && len(tt.wantErr) != 0 {
				t.Errorf("expected error, got none")
			} else if err != nil && len(tt.wantErr) != 0 {
				for _, wantErr := range tt.wantErr {
					must.ErrorIs(t, err, wantErr)
				}
			}
		})
	}
}

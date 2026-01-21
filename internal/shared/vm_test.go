// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package vm

import (
	"testing"

	"github.com/hashicorp/go-multierror"
)

func TestConfig_Validate(t *testing.T) {
	allowedPath := "/allowed/path/"

	validConfig := Config{
		Name:            "test-vm",
		CPUs:            2,
		Memory:          600,
		PrimaryDiskSize: 26000,
		BaseImage:       allowedPath + "image.qcow2",
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
			name: "Image_path_not_alloweds",
			config: Config{
				Name:            validConfig.Name,
				Memory:          validConfig.Memory,
				PrimaryDiskSize: validConfig.PrimaryDiskSize,
				CPUs:            validConfig.CPUs,
				BaseImage:       "/path/not/allowed/image.qcow2",
				OsVariant:       validConfig.OsVariant,
			},
			wantErr: multierror.Append(nil, ErrPathNotAllowed),
		},
		{
			name: "Missing_vm_name",
			config: Config{
				Memory:          validConfig.Memory,
				PrimaryDiskSize: validConfig.PrimaryDiskSize,
				CPUs:            validConfig.CPUs,
				BaseImage:       validConfig.BaseImage,
				OsVariant:       validConfig.OsVariant,
			},
			wantErr: multierror.Append(nil, ErrEmptyName),
		},
		{
			name: "Missing_base_image",
			config: Config{
				Name:            validConfig.Name,
				Memory:          validConfig.Memory,
				PrimaryDiskSize: validConfig.PrimaryDiskSize,
				CPUs:            validConfig.CPUs,
				OsVariant:       validConfig.OsVariant,
			},
			wantErr: multierror.Append(nil, ErrMissingImage),
		},
		{
			name: "Not_enough_memory",
			config: Config{
				Name:            validConfig.Name,
				Memory:          2,
				PrimaryDiskSize: validConfig.PrimaryDiskSize,
				CPUs:            validConfig.CPUs,
				BaseImage:       validConfig.BaseImage,
				OsVariant:       validConfig.OsVariant,
			},
			wantErr: multierror.Append(nil, ErrNotEnoughMemory),
		},
		{
			name: "Not_enough_disk_space",
			config: Config{
				Name:            validConfig.Name,
				Memory:          validConfig.Memory,
				PrimaryDiskSize: 2,
				CPUs:            validConfig.CPUs,
				BaseImage:       validConfig.BaseImage,
				OsVariant:       validConfig.OsVariant,
			},
			wantErr: multierror.Append(nil, ErrNotEnoughDisk),
		},
		{
			name: "No_cpus_assigned",
			config: Config{
				Name:            validConfig.Name,
				Memory:          validConfig.Memory,
				PrimaryDiskSize: validConfig.PrimaryDiskSize,
				CPUs:            0,
				BaseImage:       validConfig.BaseImage,
				OsVariant:       validConfig.OsVariant,
			},
			wantErr: multierror.Append(nil, ErrNoCPUS),
		},
		{
			name: "Incomplete_OS_variant",
			config: Config{
				Name:            validConfig.Name,
				Memory:          validConfig.Memory,
				PrimaryDiskSize: validConfig.PrimaryDiskSize,
				CPUs:            validConfig.CPUs,
				BaseImage:       validConfig.BaseImage,
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
			wantErr: multierror.Append(nil, ErrEmptyName, ErrMissingImage,
				ErrNotEnoughDisk, ErrNotEnoughMemory, ErrIncompleteOSVariant,
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

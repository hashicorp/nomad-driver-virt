package domain

import (
	"testing"

	"github.com/hashicorp/go-multierror"
)

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr error
	}{
		{
			name: "Valid configuration",
			config: Config{
				Name:      "test-domain",
				Memory:    26000,
				BaseImage: "/path/to/image.qcow2",
				OsVariant: &OSVariant{
					Arch:    "x86_64",
					Machine: "pc-i440fx-2.9",
				},
			},
			wantErr: nil,
		},
		{
			name: "Missing domain name",
			config: Config{
				Memory:    26000,
				BaseImage: "/path/to/image.qcow2",
				OsVariant: &OSVariant{
					Arch:    "x86_64",
					Machine: "pc-i440fx-2.9",
				},
			},
			wantErr: multierror.Append(nil, ErrEmptyName),
		},
		{
			name: "Missing base image",
			config: Config{
				Name:   "test-domain",
				Memory: 26000,
				OsVariant: &OSVariant{
					Arch:    "x86_64",
					Machine: "pc-i440fx-2.9",
				},
			},
			wantErr: multierror.Append(nil, ErrMissingImage),
		},
		{
			name: "Not enough memory",
			config: Config{
				Name:      "test-domain",
				Memory:    25000,
				BaseImage: "/path/to/image.qcow2",
				OsVariant: &OSVariant{
					Arch:    "x86_64",
					Machine: "pc-i440fx-2.9",
				},
			},
			wantErr: multierror.Append(nil, ErrNotEnoughDisk),
		},
		{
			name: "Incomplete OS variant",
			config: Config{
				Name:      "test-domain",
				Memory:    26000,
				BaseImage: "/path/to/image.qcow2",
				OsVariant: &OSVariant{
					Arch:    "",
					Machine: "",
				},
			},
			wantErr: multierror.Append(nil, ErrIncompleteOSVariant),
		},
		{
			name: "All errors",
			config: Config{
				Name:   "",
				Memory: 25000,
				OsVariant: &OSVariant{
					Arch:    "",
					Machine: "",
				},
			},
			wantErr: multierror.Append(nil, ErrEmptyName, ErrMissingImage, ErrNotEnoughDisk, ErrIncompleteOSVariant),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
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

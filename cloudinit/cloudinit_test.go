package cloudinit

import (
	"bytes"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	domain "github/hashicorp/nomad-driver-virt/internal/shared"

	"github.com/hashicorp/go-hclog"
)

func TestWriteConfigToISO(t *testing.T) {
	tests := []struct {
		name        string
		cloudInit   *domain.CloudInit
		path        string
		wantErr     bool
		setupFunc   func() string
		cleanupFunc func(string)
	}{
		{
			name: "Empty CloudInit config",
			cloudInit: &domain.CloudInit{
				UserData:     nil,
				UserDataPath: "",
				MetaData:     nil,
				VendorData:   nil,
			},
			path:    "test_empty_iso",
			wantErr: false,
		},
		{
			name: "Valid CloudInit config with inline userdata",
			cloudInit: &domain.CloudInit{
				UserData: map[string]interface{}{
					"example": "data",
				},
				UserDataPath: "",
				MetaData: map[string]interface{}{
					"local-hostname": "test-vm",
				},
				VendorData: map[string]interface{}{
					"example": "vendor",
				},
			},
			path:    "test_inline_userdata_iso",
			wantErr: false,
			setupFunc: func() string {
				dir, err := ioutil.TempDir("", "cloudinit_test")
				if err != nil {
					t.Fatalf("failed to create temp dir: %v", err)
				}
				return dir
			},
			cleanupFunc: func(dir string) {
				os.RemoveAll(dir)
			},
		},
		{
			name: "Valid CloudInit config with userdata file",
			cloudInit: &domain.CloudInit{
				UserData:     nil,
				UserDataPath: "testdata/user-data",
				MetaData: map[string]interface{}{
					"local-hostname": "test-vm",
				},
				VendorData: map[string]interface{}{
					"example": "vendor",
				},
			},
			path:    "test_userdata_file_iso",
			wantErr: false,
			setupFunc: func() string {
				dir, err := ioutil.TempDir("", "cloudinit_test")
				if err != nil {
					t.Fatalf("failed to create temp dir: %v", err)
				}
				return dir
			},
			cleanupFunc: func(dir string) {
				os.RemoveAll(dir)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := hclog.New(&hclog.LoggerOptions{
				Name:  "test",
				Level: hclog.LevelFromString("DEBUG"),
			})
			controller, err := NewController(logger)
			if err != nil {
				t.Fatalf("failed to create controller: %v", err)
			}

			var dir string
			if tt.setupFunc != nil {
				dir = tt.setupFunc()
			}

			isoPath, err := controller.WriteConfigToISO(tt.cloudInit, dir)
			if (err != nil) != tt.wantErr {
				t.Errorf("WriteConfigToISO() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if _, err := os.Stat(isoPath); os.IsNotExist(err) {
					t.Errorf("expected iso file to be created at %s, but it does not exist", isoPath)
				}
			}

			if tt.cleanupFunc != nil {
				tt.cleanupFunc(dir)
			}
		})
	}
}

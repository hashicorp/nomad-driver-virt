package cloudinit

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	domain "github/hashicorp/nomad-driver-virt/internal/shared"

	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

func TestWriteConfigToISO(t *testing.T) {
	vendorDataTemplate = "/vendor-data.tmpl"
	userDataTemplate = "/user-data.tmpl"
	metaDataTemplate = "/meta-data.tmpl"

	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "cloudinit_test")
	assert.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test userdata file
	userDataFile := filepath.Join(tempDir, "test_userdata")
	tus := []byte("#cloud-config\n")

	err = os.WriteFile(userDataFile, tus, 0644)
	assert.NoError(t, err)

	tests := []struct {
		name         string
		cloudInit    *domain.CloudInit
		userDataPath string
		expectError  bool
	}{
		{
			name: "Valid CloudInit without UserDataPath",
			cloudInit: &domain.CloudInit{
				MetaData: domain.MetaData{
					LocalHostname: "test-localhost",
				},
				VendorData: domain.VendorData{
					Password: "password",
				},
				UserDataPath: "",
			},
			expectError: false,
		},
		{
			name: "Valid CloudInit with UserDataPath",
			cloudInit: &domain.CloudInit{
				MetaData: domain.MetaData{
					LocalHostname: "test-localhost",
				},
				VendorData: domain.VendorData{
					Password: "password",
				},
				UserDataPath: userDataFile,
			},
			expectError: false,
		},
		{
			name: "Invalid CloudInit template path",
			cloudInit: &domain.CloudInit{
				MetaData: domain.MetaData{
					LocalHostname: "test-localhost",
				},
				VendorData: domain.VendorData{
					Password: "password",
				},
				UserDataPath: "invalid_userdata",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := hclog.New(&hclog.LoggerOptions{
				Name:  "cloud-init",
				Level: hclog.Debug,
			})

			controller, err := NewController(logger)
			assert.NoError(t, err)

			isoPath, err := controller.WriteConfigToISO(tt.cloudInit, tempDir)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.FileExists(t, isoPath)

			}
		})
	}

	os.RemoveAll(tempDir)
}

func TestExecuteTemplate(t *testing.T) {
	tests := []struct {
		name            string
		config          *domain.CloudInit
		templatePath    string
		expectError     bool
		expectedContent string
	}{
		{
			name: "no_host_and_no_user_data",
			config: &domain.CloudInit{
				MetaData: domain.MetaData{
					InstanceID: "test-instanceID",
				},
			},
			templatePath:    "/meta-data.tmpl",
			expectError:     false,
			expectedContent: "instance-id: test-instanceID",
		},
		{
			name: "host_and_no_user_data",
			config: &domain.CloudInit{
				MetaData: domain.MetaData{
					InstanceID:    "test-instanceID",
					LocalHostname: "test-localhostname",
				},
			},
			templatePath:    "/meta-data.tmpl",
			expectError:     false,
			expectedContent: "instance-id: test-instanceID",
		},
		{
			name: "host_and_user_data",
			config: &domain.CloudInit{
				MetaData: domain.MetaData{
					InstanceID:    "test-instanceID",
					LocalHostname: "test-localhostname",
				},
				UserDataPath: "/path/to/some/file",
			},
			templatePath:    "/meta-data.tmpl",
			expectError:     false,
			expectedContent: "instance-id: test-instanceID\nlocal-hostname: test-localhostname",
		},
		{
			name: "no_user_data",
			config: &domain.CloudInit{
				MetaData: domain.MetaData{
					LocalHostname: "test-localhostname",
				},
			},
			templatePath:    "/user-data.tmpl",
			expectError:     false,
			expectedContent: "#cloud-config\nlocal-hostname: test-localhostname",
		},
		{
			name: "no_user_data",
			config: &domain.CloudInit{
				MetaData: domain.MetaData{
					LocalHostname: "test-localhostname",
				},
				UserDataPath: "some/path/to/file",
			},
			templatePath:    "/user-data.tmpl",
			expectError:     false,
			expectedContent: "#cloud-config",
		},
		{
			name: "vendor_data",
			config: &domain.CloudInit{
				VendorData: domain.VendorData{},
			},
			templatePath:    "/user-data.tmpl",
			expectError:     false,
			expectedContent: "#cloud-config",
		},
		{
			name: "invalid_template_path",
			config: &domain.CloudInit{
				MetaData: domain.MetaData{
					LocalHostname: "test-localhost",
				},
			},
			templatePath: "invalid_meta-data.tmpl",
			expectError:  true,
		},
	}

	// Create test template file

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := executeTemplate(tt.config, tt.templatePath, &out)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Contains(t, out.String(), tt.expectedContent)
			}
		})
	}
}

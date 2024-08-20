package cloudinit

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

<<<<<<< HEAD
	domain "nomad-driver-virt/internal/shared"

=======
>>>>>>> bdaf2bf (style: add function comments and fix failing tests)
	"github.com/docker/distribution/uuid"
	"github.com/hashicorp/go-hclog"
	"github.com/shoenig/test/must"
)

func TestWriteConfigToISO(t *testing.T) {

	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "cloudinit_test")
	must.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test userdata file
	userDataFile := filepath.Join(tempDir, "test_userdata")
	tus := []byte("#cloud-config\n")

	err = os.WriteFile(userDataFile, tus, 0644)
	must.NoError(t, err)

	tests := []struct {
		name         string
		cloudInit    *Config
		userDataPath string
		expectError  bool
	}{
		{
			name: "Valid CloudInit without UserDataPath",
			cloudInit: &Config{
				MetaData: MetaData{
					LocalHostname: "test-localhost",
				},
				VendorData: VendorData{
					Password: "password",
				},
				UserDataPath: "",
			},
			expectError: false,
		},
		{
			name: "Valid CloudInit with UserDataPath",
			cloudInit: &Config{
				MetaData: MetaData{
					LocalHostname: "test-localhost",
				},
				VendorData: VendorData{
					Password: "password",
				},
				UserDataPath: userDataFile,
			},
			expectError: false,
		},
		{
			name: "Invalid CloudInit template path",
			cloudInit: &Config{
				MetaData: MetaData{
					LocalHostname: "test-localhost",
				},
				VendorData: VendorData{
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
			allocID := uuid.Generate().String()

			controller, err := NewController(logger)
			must.NoError(t, err)

			isoPath := tempDir + "/" + allocID[0:8] + ".iso"

			err = controller.Apply(tt.cloudInit, isoPath)
			if tt.expectError {
				must.Error(t, err)
			} else {
				must.NoError(t, err)
				must.FileExists(t, isoPath)

			}
		})
	}
}

func TestExecuteTemplate(t *testing.T) {
	tests := []struct {
		name            string
		config          *Config
		templatePath    string
		expectError     bool
		expectedContent string
	}{
		{
			name: "no_host_and_no_user_data",
			config: &Config{
				MetaData: MetaData{
					InstanceID: "test-instanceID",
				},
			},
			templatePath:    "meta-data.tmpl",
			expectError:     false,
			expectedContent: "instance-id: test-instanceID\n",
		},
		{
			name: "host_and_no_user_data",
			config: &Config{
				MetaData: MetaData{
					InstanceID:    "test-instanceID",
					LocalHostname: "test-localhostname",
				},
			},
			templatePath:    "meta-data.tmpl",
			expectError:     false,
			expectedContent: "instance-id: test-instanceID\nlocal-hostname: test-localhostname\n",
		},
		{
			name: "host_and_user_data",
			config: &Config{
				MetaData: MetaData{
					InstanceID:    "test-instanceID",
					LocalHostname: "test-localhostname",
				},
				UserDataPath: "/path/to/some/file",
			},
			templatePath:    "meta-data.tmpl",
			expectError:     false,
			expectedContent: "instance-id: test-instanceID\nlocal-hostname: test-localhostname\n",
		},
		{
			name: "no_user_data",
			config: &Config{
				MetaData: MetaData{
					LocalHostname: "test-localhostname",
				},
			},
			templatePath:    "user-data.tmpl",
			expectError:     false,
			expectedContent: "#cloud-config",
		},
		{
			name: "no_user_data",
			config: &Config{
				MetaData: MetaData{
					LocalHostname: "test-localhostname",
				},
				UserDataPath: "some/path/to/file",
			},
			templatePath:    "user-data.tmpl",
			expectError:     false,
			expectedContent: "#cloud-config",
		},
		{
			name: "empty_vendor_data",
			config: &Config{
				VendorData: VendorData{},
			},
			templatePath:    "vendor-data.tmpl",
			expectError:     false,
			expectedContent: "#cloud-config\n",
		},
		{
			name: "vendor_data_without_user_data",
			config: &Config{
				VendorData: VendorData{
					Password: "test-password",
					SSHKey:   "test-sshkey",
					Files: []File{
						{
							Path:        "/here",
							Content:     "test content",
							Permissions: "0707",
						},
					},
					Mounts: []MountFileConfig{
						{
							Tag:         "tag",
							Destination: "/destination",
						},
					},
					BootCMD: []string{"bootcmd1 arg arg", "bootcmd2 arg arg"},
					RunCMD:  []string{"cmd1 arg arg", "cmd2 arg arg"},
				},
			},
			templatePath: "vendor-data.tmpl",
			expectError:  false,
			expectedContent: `#cloud-config
password: test-password
users:
  - ssh-authorized-keys: test-sshkey
mounts:  
  - [ tag, /destination, "ext4", "defaults", "0", "2" ]
write_files:  
  - path: /here
    content: test content
    permissions: '0707'
    owner: root:root
runcmd:  
  - cmd1 arg arg  
  - cmd2 arg arg
bootcmd:  
  - bootcmd1 arg arg  
  - bootcmd2 arg arg
`,
		},
		{
			name: "vendor_data_with_user_data",
			config: &Config{
				VendorData: VendorData{
					Password: "test-password",
					SSHKey:   "test-sshkey",
					Files: []File{
						{
							Path:        "/here",
							Content:     "\"test content\"",
							Permissions: "0707",
						},
					},
					Mounts: []MountFileConfig{
						{
							Tag:         "tag",
							Destination: "/destination",
						},
					},
					BootCMD: []string{"bootcmd1 arg arg", "bootcmd2 arg arg"},
					RunCMD:  []string{"cmd1 arg arg", "cmd2 arg arg"},
				},
				UserDataPath: "/some/path/to/file",
			},
			templatePath: "vendor-data.tmpl",
			expectError:  false,
			expectedContent: `#cloud-config
merge_how:
 - name: list
   settings: [prepend]
 - name: dict
   settings: [no_replace, recurse_dict]
password: test-password
users:
  - ssh-authorized-keys: test-sshkey
mounts:  
  - [ tag, /destination, "ext4", "defaults", "0", "2" ]
write_files:  
  - path: /here
    content: "test content"
    permissions: '0707'
    owner: root:root
runcmd:  
  - cmd1 arg arg  
  - cmd2 arg arg
bootcmd:  
  - bootcmd1 arg arg  
  - bootcmd2 arg arg
`,
		},
		{
			name: "invalid_template_path",
			config: &Config{
				MetaData: MetaData{
					LocalHostname: "test-localhost",
				},
			},
			templatePath: "invalid_meta-data.tmpl",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			err := executeTemplate(tt.config, tt.templatePath, &out)
			if tt.expectError {
				must.Error(t, err)
			} else {
				must.NoError(t, err)
				must.StrContains(t, tt.expectedContent, out.String())
			}
		})
	}
}

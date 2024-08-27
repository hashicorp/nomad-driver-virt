package libvirt

import (
	"context"
	"os"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/cloudinit"
	domain "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/shoenig/test/must"
)

type cloudInitMock struct {
	passedConfig *cloudinit.Config
	passedPath   string
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
	tempDataDir, err := os.MkdirTemp("", "testdir_*")
	must.NoError(t, err)

	defer os.RemoveAll(tempDataDir)

	ld, err := New(context.Background(), hclog.NewNullLogger(),
		WithConnectionURI("test:///default"),
		WithDataDirectory(tempDataDir))

	must.NoError(t, err)
	i, err := ld.GetInfo()
	must.NoError(t, err)

	must.NonZero(t, i.LibvirtVersion)
	must.NonZero(t, i.EmulatorVersion)
	must.NonZero(t, i.StoragePools)

	ld.Close()
}

func fileExists(t *testing.T, filePath string) bool {
	info, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return false
	}
	must.NoError(t, err)

	return !info.IsDir()
}

func TestStartDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		domainName        string
		removeConfigFiles bool
		ciUserDataPath    string
		expectError       bool
		expectedCIConfig  *cloudinit.Config
		expectedPath      string
	}{
		{
			name:              "domain_created_successfully_dont_remove_files_with_userdata",
			domainName:        "domain-1",
			removeConfigFiles: false,
			ciUserDataPath:    "/path/to/user/data",
			expectedCIConfig: &cloudinit.Config{
				VendorData: cloudinit.VendorData{
					Password: "test-password",
					SSHKey:   "sshkey lkbfubwfu...",
					RunCMD:   []string{"cmd arg arg", "cmd arg arg"},
					BootCMD:  []string{"cmd arg arg", "cmd arg arg"},
					Mounts:   []cloudinit.MountFileConfig{},
					Files:    []cloudinit.File{},
				},
				MetaData:     cloudinit.MetaData{InstanceID: "domain-1", LocalHostname: "test-hostname"},
				UserDataPath: "/path/to/user/data",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDataDir, err := os.MkdirTemp("", "testdir_*")
			must.NoError(t, err)

			defer os.RemoveAll(tempDataDir)

			cim := &cloudInitMock{
				err: nil,
			}

			ld, err := New(context.Background(), hclog.NewNullLogger(),
				WithConnectionURI("test:///default"), WithCIController(cim),
				WithDataDirectory(tempDataDir))
			must.NoError(t, err)

			i, err := ld.GetInfo()
			must.NoError(t, err)

			runningDomains := i.RunningDomains
			domConfig := &domain.Config{
				RemoveConfigFiles: tt.removeConfigFiles,
				Name:              tt.domainName,
				Memory:            66600,
				Cores:             2,
				CPUs:              2,
				BaseImage:         "/path/to/test/image",
				HostName:          "test-hostname",
				SSHKey:            "sshkey lkbfubwfu...",
				Password:          "test-password",
				CMDs:              []string{"cmd arg arg", "cmd arg arg"},
				BOOTCMDs:          []string{"cmd arg arg", "cmd arg arg"},
				CIUserData:        tt.ciUserDataPath,
			}

			err = ld.CreateDomain(domConfig)

			must.NoError(t, err)

			i, err = ld.GetInfo()
			must.NoError(t, err)

			isoPath := ld.dataDir + "/" + domConfig.Name + ".iso"
			must.Eq(t, tt.removeConfigFiles, !fileExists(t, isoPath))
			must.One(t, i.RunningDomains-runningDomains)
			must.Eq(t, tt.expectedCIConfig, cim.passedConfig)
		})
	}
}

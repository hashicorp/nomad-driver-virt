package libvirt

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad-driver-virt/cloudinit"
	domain "github.com/hashicorp/nomad-driver-virt/internal/shared"
	"github.com/shoenig/test/must"
)

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
	// The test driver has one running  machine.
	must.One(t, i.RunningDomains)
	must.Zero(t, i.InactiveDomains)

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

	testError := errors.New("oh no! there is an error")

	tests := []struct {
		name              string
		domainName        string
		removeConfigFiles bool
		ciUserDataPath    string
		expectError       error
		expectedCIConfig  *cloudinit.Config
		ciError           error
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
					BootCMD: []string{
						"cmd arg arg",
						"cmd arg arg",
						"mkdir -p /path/to/file/one",
						"mountpoint -q /path/to/file/one || mount -t 9p -o trans=virtio tagOne /path/to/file/one",
						"mkdir -p /path/to/file/two",
						"mountpoint -q /path/to/file/two || mount -t 9p -o trans=virtio tagTwo /path/to/file/two",
					},
					Mounts: []cloudinit.MountFileConfig{
						{
							Destination: "/path/to/file/one",
							Tag:         "tagOne",
						},
						{
							Destination: "/path/to/file/two",
							Tag:         "tagTwo",
						},
					},
					Files: []cloudinit.File{
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
				},
				MetaData:     cloudinit.MetaData{InstanceID: "domain-1", LocalHostname: "test-hostname"},
				UserDataPath: "/path/to/user/data",
			},
		},
		{
			name:              "domain_created_successfully_remove_files_with_userdata",
			domainName:        "domain-2",
			removeConfigFiles: true,
			ciUserDataPath:    "/path/to/user/data",
			expectedCIConfig: &cloudinit.Config{
				VendorData: cloudinit.VendorData{
					Password: "test-password",
					SSHKey:   "sshkey lkbfubwfu...",
					RunCMD:   []string{"cmd arg arg", "cmd arg arg"},
					BootCMD: []string{
						"cmd arg arg",
						"cmd arg arg",
						"mkdir -p /path/to/file/one",
						"mountpoint -q /path/to/file/one || mount -t 9p -o trans=virtio tagOne /path/to/file/one",
						"mkdir -p /path/to/file/two",
						"mountpoint -q /path/to/file/two || mount -t 9p -o trans=virtio tagTwo /path/to/file/two",
					},
					Mounts: []cloudinit.MountFileConfig{
						{
							Destination: "/path/to/file/one",
							Tag:         "tagOne",
						},
						{
							Destination: "/path/to/file/two",
							Tag:         "tagTwo",
						},
					},
					Files: []cloudinit.File{
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
				},
				MetaData: cloudinit.MetaData{
					InstanceID:    "domain-2",
					LocalHostname: "test-hostname"},
				UserDataPath: "/path/to/user/data",
			},
		},
		{
			name:        "duplicated_domain_error",
			domainName:  "domain-2",
			expectError: ErrDomainExists,
		},
		{
			name:        "cloud_init_error_propagation",
			domainName:  "domain-3",
			expectError: testError,
			ciError:     testError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDataDir, err := os.MkdirTemp("", "testdir_*")
			must.NoError(t, err)

			defer os.RemoveAll(tempDataDir)

			cim := &cloudInitMock{
				err: tt.ciError,
			}

			ld, err := New(context.Background(), hclog.NewNullLogger(),
				WithConnectionURI("test:///default"), WithCIController(cim),
				WithDataDirectory(tempDataDir))
			must.NoError(t, err)
			defer ld.Close()

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

			err = ld.CreateDomain(domConfig)
			must.Eq(t, tt.expectError, errors.Unwrap(err))
			if err == nil {
				i, err = ld.GetInfo()
				must.NoError(t, err)

				isoPath := ld.dataDir + "/" + domConfig.Name + ".iso"
				must.Eq(t, tt.removeConfigFiles, !fileExists(t, isoPath))
				must.One(t, i.RunningDomains-runningDomains)
				must.Eq(t, tt.expectedCIConfig, cim.passedConfig)
			}
		})
	}
}

func Test_CreateStopAndDestroyDomain(t *testing.T) {
	tempDataDir, err := os.MkdirTemp("", "testdir_*")
	must.NoError(t, err)

	defer os.RemoveAll(tempDataDir)

	cim := &cloudInitMock{}

	ld, err := New(context.Background(), hclog.NewNullLogger(),
		WithConnectionURI("test:///default"), WithCIController(cim),
		WithDataDirectory(tempDataDir))
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
	err = ld.CreateDomain(&domain.Config{
		RemoveConfigFiles: true,
		Name:              domainName,
		Memory:            66600,
		CPUs:              6,
		BaseImage:         "/path/to/test/image",
	})
	must.NoError(t, err)

	doms, err = ld.GetAllDomains()
	must.NoError(t, err)

	// The initial test hypervisor has one plus the one that was just started.
	must.Len(t, 2, doms)

	err = ld.StopDomain(domainName)
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

	err = ld.DestroyDomain(domainName)
	must.NoError(t, err)

	info, err = ld.GetInfo()
	must.NoError(t, err)

	// The domain is present as inactive anymore.
	must.Zero(t, info.InactiveDomains)
}

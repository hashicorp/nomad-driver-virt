package domain

type CloudInit struct {
	VendorData   VendorData
	MetaData     MetaData
	UserDataPath string
}

type MetaData struct {
	InstanceID    string `yaml:"instance-id"`
	LocalHostname string `yaml:""`
}

type VendorData struct {
	Password string
	SSHKey   string
	RunCMD   []string
	BootCMD  []string
	Mounts   []MountFileConfig
	Files    []File
}

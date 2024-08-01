package domain

type CloudInit struct {
	UserData     UserData
	MetaData     MetaData
	UserDataPath string
	MetaDataPath string
}

type MetaData struct {
	InstanceID    string `yaml:"instance-id"`
	LocalHostname string `yaml:""`
}

type UserData struct {
	Users  Users
	RunCMD []string
	Mounts []MountFileConfig
}

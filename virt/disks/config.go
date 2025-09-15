package disks

import (
	"errors"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
	"path/filepath"
	"slices"
	"strings"
)

var (
	ErrDisallowedPath     = errors.New("disallowed path")
	ErrDisallowedAuthUUID = errors.New("disallowed authentication UUID")
)

type DisksConfig struct {
	*FileDisksConfig `codec:"file"`
	*RbdDisksConfig  `codec:"rbd"`
}

func (cfg *DisksConfig) Validate(mErr *multierror.Error, allowedPaths []string, allowedCephUUIDs []string) {
	if cfg == nil {
		return
	}
	cfg.RbdDisksConfig.Validate(mErr, allowedPaths, allowedCephUUIDs)
	cfg.FileDisksConfig.Validate(mErr, allowedPaths)
}

func (cfg *DisksConfig) Copy() *DisksConfig {
	if cfg == nil {
		return nil
	}

	return &DisksConfig{
		FileDisksConfig: cfg.FileDisksConfig.Copy(),
		RbdDisksConfig:  cfg.RbdDisksConfig.Copy(),
	}
}

type FileDiskConfig struct {
	Fmt  string
	Path string
}

func (cfg *FileDiskConfig) Validate(mErr *multierror.Error, allowedPaths []string) {
	if cfg == nil {
		return
	}
	if !IsAllowedPath(allowedPaths, cfg.Path) {
		mErr.Errors = append(mErr.Errors, ErrDisallowedPath)
	}
}

func (cfg *FileDiskConfig) Copy() *FileDiskConfig {
	if cfg == nil {
		return nil
	}
	return &FileDiskConfig{
		Fmt:  cfg.Fmt,
		Path: cfg.Path,
	}
}

type FileDisksConfig map[string]*FileDiskConfig

func (cfg *FileDisksConfig) Validate(mErr *multierror.Error, allowedPaths []string) {
	if cfg == nil {
		return
	}

	for _, v := range *cfg {
		v.Validate(mErr, allowedPaths)
	}
}

func (cfg *FileDisksConfig) Copy() *FileDisksConfig {
	if cfg == nil {
		return nil
	}
	result := FileDisksConfig{}
	for k, v := range *cfg {
		result[k] = v.Copy()
	}
	return &result
}

type NetworkHostConfig struct {
	Port      int
	Socket    string
	Transport string
}

func (cfg *NetworkHostConfig) Validate(mErr *multierror.Error, allowedPaths []string) {
	if cfg == nil {
		return
	}
	if !IsAllowedPath(allowedPaths, cfg.Socket) {
		mErr.Errors = append(mErr.Errors, ErrDisallowedPath)
	}
}
func (cfg *NetworkHostConfig) Copy() *NetworkHostConfig {
	if cfg == nil {
		return nil
	}
	return &NetworkHostConfig{
		Port:      cfg.Port,
		Socket:    cfg.Socket,
		Transport: cfg.Transport,
	}
}

type NetworkHostsConfig map[string]*NetworkHostConfig

func (cfg *NetworkHostsConfig) Validate(mErr *multierror.Error, allowedPaths []string) {
	if cfg == nil {
		return
	}
	for _, v := range *cfg {
		v.Validate(mErr, allowedPaths)
	}
}
func (cfg *NetworkHostsConfig) Copy() *NetworkHostsConfig {
	if cfg == nil {
		return nil
	}
	result := NetworkHostsConfig{}
	for k, v := range *cfg {
		result[k] = v.Copy()
	}
	return &result
}

type RbdAuthConfig struct {
	Username string
	Uuid     string
}

func (cfg *RbdAuthConfig) Validate(mErr *multierror.Error, allowedSecretUUID []string) {
	if cfg == nil {
		return
	}
	if !slices.Contains(allowedSecretUUID, cfg.Uuid) {
		mErr.Errors = append(mErr.Errors, ErrDisallowedAuthUUID)
	}
}
func (cfg *RbdAuthConfig) Copy() *RbdAuthConfig {
	if cfg == nil {
		return nil
	}
	return &RbdAuthConfig{
		Username: cfg.Username,
		Uuid:     cfg.Uuid,
	}
}

type RbdDiskConfig struct {
	Name     string
	Fmt      string
	Config   string
	Snapshot string

	*RbdAuthConfig      `codec:"auth"`
	*NetworkHostsConfig `codec:"host"`
}

func (cfg *RbdDiskConfig) Validate(
	mErr *multierror.Error,
	allowedPaths []string,
	allowedSecretUUID []string) {
	if cfg == nil {
		return
	}
	if !IsAllowedPath(allowedPaths, cfg.Config) {
		mErr.Errors = append(mErr.Errors, ErrDisallowedPath)
	}
	cfg.RbdAuthConfig.Validate(mErr, allowedSecretUUID)
}
func (cfg *RbdDiskConfig) Copy() *RbdDiskConfig {
	if cfg == nil {
		return nil
	}
	return &RbdDiskConfig{
		Name:               cfg.Name,
		Fmt:                cfg.Fmt,
		Config:             cfg.Config,
		Snapshot:           cfg.Snapshot,
		RbdAuthConfig:      cfg.RbdAuthConfig.Copy(),
		NetworkHostsConfig: cfg.NetworkHostsConfig.Copy(),
	}
}

type RbdDisksConfig map[string]*RbdDiskConfig

func (cfg *RbdDisksConfig) Validate(
	mErr *multierror.Error,
	allowedPaths []string,
	allowedSecretUUID []string) {
	if cfg == nil {
		return
	}
	for _, v := range *cfg {
		v.Validate(mErr, allowedPaths, allowedSecretUUID)
	}
}

func (cfg *RbdDisksConfig) Copy() *RbdDisksConfig {
	if cfg == nil {
		return nil
	}
	result := RbdDisksConfig{}
	for k, v := range *cfg {
		result[k] = v.Copy()
	}
	return &result
}

func HclSpec() *hclspec.Spec {
	hostSpec := hclspec.NewBlockMap("host", []string{"name"}, hclspec.NewObject(map[string]*hclspec.Spec{
		"port":      hclspec.NewAttr("port", "number", false),
		"socket":    hclspec.NewAttr("socket", "string", false),
		"transport": hclspec.NewAttr("transport", "string", false),
	}))
	return hclspec.NewBlock("disks", false, hclspec.NewObject(map[string]*hclspec.Spec{
		"file": hclspec.NewBlockMap("file", []string{"label"}, hclspec.NewObject(map[string]*hclspec.Spec{
			"path": hclspec.NewAttr("path", "string", true),
			"fmt":  hclspec.NewAttr("fmt", "string", false),
		})),
		"rbd": hclspec.NewBlockMap("rbd", []string{"label"}, hclspec.NewObject(map[string]*hclspec.Spec{
			"name":     hclspec.NewAttr("name", "string", true),
			"fmt":      hclspec.NewAttr("fmt", "string", false),
			"config":   hclspec.NewAttr("config", "string", false),
			"snapshot": hclspec.NewAttr("snap", "string", false),
			"auth": hclspec.NewBlock("auth", false, hclspec.NewObject(map[string]*hclspec.Spec{
				"username": hclspec.NewAttr("username", "string", false),
				"uuid":     hclspec.NewAttr("uuid", "string", false),
			})),
			"host": hostSpec,
		})),
	}))
}

func isParent(parent, path string) bool {
	rel, err := filepath.Rel(parent, path)
	return err == nil && !strings.HasPrefix(rel, "..")
}

func IsAllowedPath(allowedPaths []string, path string) bool {
	for _, ap := range allowedPaths {
		if isParent(ap, path) {
			return true
		}
	}

	return false
}

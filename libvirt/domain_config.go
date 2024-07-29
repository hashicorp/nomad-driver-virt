// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"fmt"
	"html/template"
	"os"
	"strings"

	domain "github/hashicorp/nomad-driver-virt/internal/shared"
)

type cloudinitConfig struct {
	domainDir    string
	metadataPath string
	userdataPath string
}

func dirExists(dirname string) bool {
	info, err := os.Stat(dirname)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}

func deleteDir(name string) error {
	if dirExists(name) {
		err := os.RemoveAll(name)
		if err != nil {
			return fmt.Errorf("libvirt: failed to delete directory: %w", err)
		}
	}

	return nil
}

func (cic *cloudinitConfig) createAndpopulateFiles(config *domain.Config) error {
	mdf, err := os.Create(cic.domainDir + "/meta-data")
	defer mdf.Close()
	if err != nil {
		return fmt.Errorf("libvirt: create file: %w", err)
	}

	err = executeTemplate(config, metaDataTemplate, mdf)
	if err != nil {
		return err
	}

	udf, err := os.Create(cic.domainDir + "/user-data")
	defer udf.Close()
	if err != nil {
		return fmt.Errorf("libvirt: create file: %w", err)
	}

	err = executeTemplate(config, userDataTemplate, udf)
	if err != nil {
		return err
	}

	return nil
}

func executeTemplate(config *domain.Config, in string, out *os.File) error {
	pwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("libvirt: unable to get path: %w", err)
	}

	tmpl, err := template.ParseFiles(pwd + in)
	if err != nil {
		return fmt.Errorf("libvirt: unable to parse template: %w", err)
	}

	err = tmpl.Execute(out, config)
	if err != nil {
		return fmt.Errorf("libvirt: unable to execute template: %w", err)
	}
	return nil
}

func createDomainFolder(path string) error {
	err := os.MkdirAll(path, userDataDirPermissions)
	if err != nil {
		return err
	}

	return nil
}

func cleanUpDomainFolder(path string) error {
	return deleteDir(path)
}

func (d *driver) parceConfiguration(dc *domain.Config, ci *cloudinitConfig) []string {
	args := []string{
		"--debug",
		fmt.Sprintf("--connect=%s", d.uri),
		fmt.Sprintf("--name=%s", dc.Name),
		fmt.Sprintf("--ram=%d", dc.Memory),
		fmt.Sprintf("--vcpus=%d,cores=%d", dc.CPUs, dc.Cores),

		"--noautoconsole",
	}

	if dc.OsVariant != "" {
		args = append(args, fmt.Sprintf("--os-variant=%s", dc.OsVariant))
	}

	if dc.CloudInit.Enable {
		args = append(args, "--import", "--disk", fmt.Sprintf("path=%s,format=%s,size=%d", dc.BaseImage, dc.DiskFmt, dc.DiskSize))
		args = append(args, "--cloud-init", fmt.Sprintf("user-data=%s,meta-data=%s", ci.userdataPath, ci.metadataPath))
	} else {
		args = append(args, fmt.Sprintf("location=%s", dc.BaseImage))
		args = append(args, "--disk", fmt.Sprintf("path=%s,format=%s,size=%d", dc.BaseImage, dc.DiskFmt, dc.DiskSize))
	}

	for _, ni := range dc.NetworkInterfaces {
		args = append(args, "--network", fmt.Sprintf("bridge=%s,model=virtio", ni))
	}

	if len(dc.Mounts) > 0 {
		args = append(args, "--memorybacking=source.type=memfd,access.mode=shared")

		for _, m := range dc.Mounts {
			mArgs := []string{
				m.Source,
				m.Tag,
				"driver.type=virtiofs",
			}

			if m.AccessMode != "" {
				mArgs = append(mArgs, fmt.Sprintf("accessmode=%s", m.AccessMode))
			}

			args = append(args, fmt.Sprintf("--filesystem=%s", strings.Join(mArgs, ",")))
		}
	}

	return args
}

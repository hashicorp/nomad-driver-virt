// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"

	domain "github/hashicorp/nomad-driver-virt/internal/shared"
	"github/hashicorp/nomad-driver-virt/libvirt"
	"github/hashicorp/nomad-driver-virt/virt"
	"time"

	"github.com/hashicorp/go-hclog"
)

func main() {

	name := "j9"
	appLogger := hclog.New(&hclog.LoggerOptions{
		Name:  "my-app",
		Level: hclog.Debug,
	})

	ctx, cancel := context.WithCancel(context.Background())

	conn, err := libvirt.New(ctx, appLogger, libvirt.WithAuth("juana", "juana"))
	if err != nil {
		fmt.Printf("error: %+v\n %+v\n", conn, err)
		return
	}

	tz, err := time.LoadLocation("America/Denver")
	if err != nil {
		fmt.Println("Timezone is not valid")
		return
	}

	users := domain.Users{
		IncludeDefault: true,
		Users: []domain.UserConfig{
			{
				Name:     "juana",
				Password: "password",
				SSHKeys:  []string{"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIC31v1/cUhjyA8aznoy9FlwU4d6p/zfxP5RqRxhCWzGK juanita.delacuestamorales@hashicorp.com"},
				Sudo:     "ALL=(ALL) NOPASSWD:ALL",
				Groups:   []string{"sudo"},
				Shell:    "/bin/bash",
			},
		},
	}

	ci := domain.CloudInit{
		Enable: false,
	}

	mounts := []domain.MountFileConfig{
		{
			Source:      "/home/ubuntu/test/alloc",
			Destination: "/home/juana/alloc",
			ReadOnly:    false,
			Tag:         "blah",
		},
	}

	config := &domain.Config{
		RemoveConfigFiles: false,
		CloudInit:         ci,
		Timezone:          tz,
		Name:              name,
		Memory:            2048000,
		CPUs:              4,
		Cores:             2,
		OsVariant:         "ubuntufocal",
		BaseImage:         "/home/ubuntu/test/" + name + ".img",
		DiskFmt:           "qcow2",
		DiskSize:          1,
		NetworkInterfaces: []string{"virbr0"},
		HostName:          name + "-host",
		UsersConfig:       users,
		EnvVariables: map[string]string{
			"IDENTITY": "identity",
			"BLAH":     "identity",
			"DEMO":     "please dont fail",
		},
		Files: []domain.File{
			{
				Path:        "/home/juana/text.txt",
				Content:     ` this is the text we will be putting`,
				Permissions: "0777",
				Owner:       "root",
				Group:       "root",
			},
		},
		Mounts: mounts,
	}

	err = conn.CreateDomain(config)
	if err != nil {
		fmt.Println("\n no vm this time :(", err)
		return
	}

	fmt.Println("\n we have a vm")
	//conn.GetVms()

	info, err := conn.GetInfo()
	if err != nil {
		fmt.Println("ups no info", err)
	}

	fmt.Printf("%+v\n", info, "\n")
	cancel()
	time.Sleep(1 * time.Second)

	// Serve the plugin
	//plugins.Serve(factory)
}

// factory returns a new instance of a nomad driver plugin
func factory(log hclog.Logger) interface{} {
	return virt.NewPlugin(log)
}

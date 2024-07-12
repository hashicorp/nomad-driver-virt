// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"
	"time"

	domain "github/hashicorp/nomad-driver-virt/internal/shared"
	"github/hashicorp/nomad-driver-virt/libvirt"
	"github/hashicorp/nomad-driver-virt/virt"

	"github.com/hashicorp/go-hclog"
)

func main() {

	name := "juana-14"
	appLogger := hclog.New(&hclog.LoggerOptions{
		Name:  "my-app",
		Level: hclog.Debug,
	})

	ctx, cancel := context.WithCancel(context.Background())

	conn, err := libvirt.New(ctx, appLogger)
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
				SSHKeys:  []string{"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQCg6O020792w3AzhdV27gcH0zn6oCGF6JUSzrhRrqdXqFSwkw7kPeArw9uI61ebC2qNjUKOuiU8lkyYFYAMeirG4BPt5yMUzl+tjjdiI1J2IoeDxknMO83eA+4ebJJZ670vUpWYuzwEakPkj2IBiXg5UbIfTIdO0N2NtrXV9nTu7xKrfPXCbnIxPaSUJRVTHlg5hlZIT6tVl+ZsbJhzhVYgvPnIkqFqPj1Owo2XFtgE72A5KZQbYtnBEz+AwAjLmU7GL5JE14fsihD6z5QCD2z0wOHyhdCSP53/n5Tuo9oelDv2hkkTWf5QlAi6i5Z8UcEXuog7HWgqiiZblVIuCw9EGaFgBpMQbI1Q8WOrysOWcsPmoX9a4OjFGOfbE9R7tbwMRI10/nbvpapO1oBOKztF6bS9rHIXBS/9VHQ53GRTQhGGd6Zyk4eZfoEO8QMq2cT4/FA887L84QSvkJ9jCdNdmF8eYK9Z+pRVUBh4qrP8744rMH4fX5NJqnU1+UYTiy0= ubuntu@ip-10-0-1-235"},
				Sudo:     "ALL=(ALL) NOPASSWD:ALL",
				Groups:   []string{"sudo"},
				Shell:    "/bin/bash",
			},
		},
	}

	ci := domain.CloudInit{
		Enable:          true,
		ProvideUserData: false,
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
		CloudInit:         ci,
		Timezone:          tz,
		RemoveConfigFiles: false,
		Name:              name,
		Memory:            2048,
		CPUs:              4,
		Cores:             2,
		OsVariant:         "ubuntufocal",
		CloudImgPath:      fmt.Sprintf("/home/ubuntu/test/%s.img", name),
		DiskFmt:           "qcow2",
		NetworkInterfaces: []string{"virbr0"},
		HostName:          name,
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
		fmt.Println(" no vm this time", err)
	}

	conn.GetVms()

	info, err := conn.GetInfo()
	if err != nil {
		fmt.Println("ups no info", err)
	}

	fmt.Printf("%+v\n", info)
	cancel()
	time.Sleep(1 * time.Second)
	// Serve the plugin
	//plugins.Serve(factory)
}

// factory returns a new instance of a nomad driver plugin
func factory(log hclog.Logger) interface{} {
	return virt.NewPlugin(log)
}

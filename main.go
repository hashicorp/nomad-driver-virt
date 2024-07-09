// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"
	"time"

	"github/hashicorp/nomad-driver-virt/libvirt"
	"github/hashicorp/nomad-driver-virt/virt"

	"github.com/hashicorp/go-hclog"
)

func main() {

	name := "juana-4"
	appLogger := hclog.New(&hclog.LoggerOptions{
		Name: "my-app",
	})

	ctx, cancel := context.WithCancel(context.Background())

	conn, err := libvirt.New(ctx, appLogger)
	if err != nil {
		fmt.Printf("error: %+v\n %+v\n", conn, err)
		return
	}
	users := libvirt.Users{
		IncludeDefault: true,
		Users: []libvirt.UserConfig{
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

	tz, err := time.LoadLocation("America/Denver")
	if err != nil {
		fmt.Println("Timezone is not valid")
		return
	}

	config := &libvirt.DomainConfig{
		Timezone:          tz,
		RemoveConfigFiles: false,
		Name:              name,
		Memory:            2048,
		CPUs:              4,
		Cores:             2,
		OsVariant:         "ubuntufocal",
		CloudImgPath:      "/home/ubuntu/test/" + name + ".img",
		DiskFmt:           "qcow2",
		NetworkInterface:  "virbr0",
		HostName:          name,
		UsersConfig:       users,
		EnvVariables: map[string]string{
			"IDENTITY": "identity",
			"BLAH":     "identity",
		},
		Files: []libvirt.File{
			{
				Path:        "/home/juana/text.txt",
				Content:     ` this is the text we will be putting`,
				Permissions: "0777",
				Owner:       "root",
				Group:       "root",
			},
			{
				Path:        "/home/ubuntu/text.txt",
				Content:     ` this is the text we will be putting`,
				Permissions: "0777",
				Owner:       "ubuntu",
				Group:       "ubuntu",
			},
		},
	}

	err = conn.CreateDomain(config)
	if err != nil {
		fmt.Println(" no vm this time", err)
	}

	conn.GetVms()
	cancel()

	time.Sleep(2 * time.Second)
	info, err := conn.GetInfo()
	if err != nil {
		fmt.Println("ups no info", err)
	}

	fmt.Printf("%+v\n", info)
	// Serve the plugin
	//plugins.Serve(factory)
}

// factory returns a new instance of a nomad driver plugin
func factory(log hclog.Logger) interface{} {
	return virt.NewPlugin(log)
}

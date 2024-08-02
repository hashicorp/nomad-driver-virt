// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"fmt"

	"github/hashicorp/nomad-driver-virt/cloudinit"
	domain "github/hashicorp/nomad-driver-virt/internal/shared"
	"github/hashicorp/nomad-driver-virt/libvirt"
	"github/hashicorp/nomad-driver-virt/virt"
	"time"

	"github.com/hashicorp/go-hclog"
)

func main() {
	name := "j10"
	appLogger := hclog.New(&hclog.LoggerOptions{
		Name:  "my-app",
		Level: hclog.Debug,
	})

	//fmt.Println(ci(appLogger, name))
	fmt.Println(createVM(appLogger, name))

	// Serve the plugin
	//plugins.Serve(factory)
}

func createVM(appLogger hclog.Logger, name string) error {

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		cancel()
		time.Sleep(1 * time.Second)
	}()

	conn, err := libvirt.New(ctx, appLogger, libvirt.WithAuth("juana", "juana"))
	if err != nil {
		fmt.Printf("error: %+v\n %+v\n", conn, err)
		return err
	}

	tz, err := time.LoadLocation("America/Denver")
	if err != nil {
		fmt.Println("Timezone is not valid")
		return err
	}

	config := &domain.Config{
		CIUserData:        "/home/ubuntu/test/user-data",
		Password:          "password",
		RemoveConfigFiles: false,
		Timezone:          tz,
		Name:              name,
		Memory:            2048000,
		CPUs:              4,
		Cores:             2,
		BaseImage:         "/home/ubuntu/test/" + name + ".img",
		DiskFmt:           "qcow2",
		DiskSize:          1,
		NetworkInterfaces: []string{"virbr0"},
		HostName:          name + "-host",
		SSHKey:            "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIC31v1/cUhjyA8aznoy9FlwU4d6p/zfxP5RqRxhCWzGK juanita.delacuestamorales@hashicorp.com",
		Files: []domain.File{
			{
				Path:        "/home/juana/text.txt",
				Content:     ` this is the text we will be putting`,
				Permissions: "0777",
			},
		},
	}

	err = conn.CreateDomain(config)
	if err != nil {
		fmt.Println("\n no vm this time :(", err)
		return err
	}

	fmt.Println("\n we have a vm")
	//conn.GetVms()

	info, err := conn.GetInfo()
	if err != nil {
		fmt.Println("ups no info", err)
	}

	fmt.Printf("%+v\n", info, "\n")

	return nil
}

// factory returns a new instance of a nomad driver plugin
func factory(log hclog.Logger) interface{} {
	return virt.NewPlugin(log)
}

func ci(appLogger hclog.Logger, name string) error {
	cic, err := cloudinit.NewController(appLogger)
	if err != nil {
		fmt.Printf("error: %+v\n %+v\n", err)
	}

	mounts := []domain.MountFileConfig{
		{
			Source:      "/home/ubuntu/test/alloc",
			Destination: "/alloc",
			Tag:         "allocDir",
		},
		{
			Source:      "/home/ubuntu/test/logs",
			Destination: "/run/cloud-init",
			Tag:         "logs",
		},
	}

	ci := &domain.CloudInit{
		MetaData: domain.MetaData{
			InstanceID:    name,
			LocalHostname: name,
		},
		VendorData: domain.VendorData{
			Mounts: mounts,
			RunCMD: []string{
				fmt.Sprintf("mkdir -p %s", "/alloc"),
				fmt.Sprintf("mount -t virtiofs %s %s", "allocDir", "/alloc"),
				fmt.Sprintf("mount -t virtiofs %s %s", "logs", "/run/cloud-init"),
			},
		},
	}

	path, err := cic.WriteConfigToISO(ci, "/usr/local/virt")
	fmt.Println(" the iso is located at: ", path)
	return err

}

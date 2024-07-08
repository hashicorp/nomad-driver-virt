package main

import (
	"context"
	"fmt"
	"github/juanadelacuesta/heraclitus/libvirt"
	"github/juanadelacuesta/heraclitus/virt"
	"time"

	"github.com/hashicorp/go-hclog"
)

func main() {

	name := "juana-44"
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
				SSHKeys:  []string{"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABgQCy0vGcocd3IkW0iVnO5UlmjmPZjtydg4n31UhwB0Z/TuqkN/mEQFd/jEcgrX76R2Wn/f3FAT5o++yrwaWrpf7C2z6kDv+ntCkHjy6qKy2W14R8glGIPkQUyA7Pm6SgpFCua6MLddZSGld705QHMNu6a89MH3cvS1S9AoPWEnz/bJL1akb4YhY/NN4S7gM3VSSNmlRGCyjWAh1nVjya4S+2UYQIq23KIMif3SuEX0duyXdvAD+asXI3zHoRAOps1CNfp+qIqAJisFBcmfwplT6IFwUFI29S2ff9uUx5RQjl0ZiUTil25Bdgbm0fq20KuET9lMAHE2Bwbg4DVXK8oq3djTREu6oF089Usnzri1glpbjSC/b0/KRV3KJeTm+X1YaIJyjSMN0VHFG/IXRj+gV3PVQeANKDoJl5b2sgl0J+5INKUu0JeFfZcoSiRfdlgrFPChA/ktEc/h9lCbXsIcqBLYfj9RcybfhiIZFbgjR/xR9XwrU1F4WesBc1Zie5UU8= ubuntu@ip-10-0-1-184"},
				Sudo:     "ALL=(ALL) NOPASSWD:ALL",
				Groups:   []string{"sudo"},
				Shell:    "/bin/bash",
			},
		},
	}

	config := &libvirt.DomainConfig{
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

	dom, err := conn.CreateDomain(config)
	if err != nil {
		fmt.Println(" no vm this time", err)
		cancel()
		return
	}

	fmt.Println(dom.GetID())

	conn.GetVms()
	cancel()

	time.Sleep(2 * time.Second)

	// Serve the plugin
	//plugins.Serve(factory)
}

// factory returns a new instance of a nomad driver plugin
func factory(log hclog.Logger) interface{} {
	return virt.NewPlugin(log)
}

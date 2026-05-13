// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

job "python-server" {

  group "virt-group" {
    count = 1

    network {
      mode = "host"
      port "http" {
        to = 8000
      }
    }

    volume "virt-primary" {
      type            = "host"
      source          = "virt-primary-volume"
      attachment_mode = "block-device"
    }

    task "virt-task" {

      volume_mount {
        volume      = "virt-primary"
        destination = "/dev/vda"
      }

      driver = "virt"

      config {
        cmds = ["python3 -m http.server 8000 -d /"]

        disk {
          volume = "/dev/vda"
        }

        network_interface {
          bridge {
            name  = "virbr0"
            ports = ["http"]
          }
        }
      }

      resources {
        cores  = 1
        memory = 1024
      }
    }
  }
}

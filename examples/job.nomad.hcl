// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

job "virt-example" {

  group "virt-group" {
    count = 1

    task "virt-task" {

      identity {
        env  = true
        file = true
      }

      template {
        data        = <<EOH
        Guest System
        EOH
        destination = "local/index.html"
      }

      driver = "nomad-driver-virt"

      artifact {
        source      = "http://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
        destination = "local/focal-server-cloudimg-amd64.img"
        mode        = "file"
      } 

      config {
        image                           = "local/focal-server-cloudimg-amd64.img"
        use_thin_copy                   = true
        default_user_password           = "password"
        cmds                            = ["touch /home/ubuntu/file.txt"]
        default_user_authorized_ssh_key = "ssh-ed25519 AAAAC3NzaC1lZDI..."
      }

      resources {
        cores  = 4
        memory = 4000
      }
    }
  }
}

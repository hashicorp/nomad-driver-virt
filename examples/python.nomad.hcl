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

    task "virt-task" {

      driver = "nomad-driver-virt"

      artifact {
        source      = "http://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
        destination = "local/focal-server-cloudimg-amd64.img"
        mode        = "file"
      } 

      config {
        image                 = "local/focal-server-cloudimg-amd64.img"
        primary_disk_size     = 10000
        use_thin_copy         = true
        default_user_password = "password"
        cmds                  = ["python -m http.server 8000"]
      }

      resources {
        cpu    = 40
        memory = 4000
      }
    }
  }
}

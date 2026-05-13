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

    volume "virt-data" {
      type   = "host"
      source = "virt-data-dir"
    }

    task "virt-task" {

      volume_mount {
        volume      = "virt-data"
        destination = "/mnt/data"
      }

      template {
        data        = <<EOH
<pre>
Guest System

\o/
</pre>
        EOH
        destination = "local/index.html"
      }

      artifact {
        source      = "http://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
        destination = "local/focal-server-cloudimg-amd64.img"
        mode        = "file"
      }

      driver = "virt"

      config {
        default_user_password = "password"
        cmds = [
          "cp /local/index.html /mnt/data",
          "python3 -m http.server 8000 -d /mnt/data",
        ]

        disk {
          source {
            image = "local/focal-server-cloudimg-amd64.img"
          }
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

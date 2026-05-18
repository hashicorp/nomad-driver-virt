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

      template {
        data        = <<EOH
<pre>
Guest System

\o/
</pre>
        EOH
        destination = "local/index.html"
      }

      driver = "virt"

      config {
        default_user_password = "password"
        cmds                  = ["python3 -m http.server 8000 -d /local"]

        disk {
          pool = "remote"
          source {
            volume = "focal.img"
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

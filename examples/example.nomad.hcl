job "virt-example" {
  datacenters = ["dc1"]

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
        destination = "focal-server-cloudimg-amd64.img"
        mode        = "file"
      }

      config {
        image                           = "focal-server-cloudimg-amd64.img"
        use_thin_copy                   = true
        default_user_password           = "password"
        cmds                            = ["touch /home/ubuntu/file.txt"]
        default_user_authorized_ssh_key = "ssh-ed25519 AAAAC3Nza..."
      }

      resources {
        cpu    = 128
        memory = 25600
      }
    }
  }
}

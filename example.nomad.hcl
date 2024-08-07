job "e" {
  datacenters = ["dc1"]

  group "virt-group" {
    count = 1

    task "virt-task" {
      driver = "nomad-driver-virt"

      config {
        image = "/home/ubuntu/test/test-1.img"
        password = "password"
        authorized_ssh_key = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIC31v1/cUhjyA8aznoy9FlwU4d6p/zfxP5RqRxhCWzGK juanita.delacuestamorales@hashicorp.com"
      }

      resources {
        cpu    = 128
        memory = 64
      }
    }
  }
}
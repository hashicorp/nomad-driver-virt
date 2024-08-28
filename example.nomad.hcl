job "e" {
  datacenters = ["dc1"]

  group "virt-group" {
    count = 1

    task "virt-task" {

      identity {
        env = true
        file = true
      }

      template {
        data = <<EOH
        Guest System
        EOH
        destination = "local/index.html"
      } 

      driver = "nomad-driver-virt"
    
/*       artifact {
        source =  "http://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
        destination = "tinycore.qcow2"
        mode = "file"
      }  */ 


      config {
        //image = "tinycore.qcow2"
        image ="/home/ubuntu/juana13.img"
        password = "password"
        cmds = [" touch /home/ubuntu/fede.txt"]
        //user_data = "/home/ubuntu/cc/user-data" //TODO: verify user data!!!
        authorized_ssh_key = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIC31v1/cUhjyA8aznoy9FlwU4d6p/zfxP5RqRxhCWzGK juanita.delacuestamorales@hashicorp.com"
      }

      resources {
        cpu    = 128
        memory = 25600
      }
    }
  }
}
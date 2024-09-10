job "python-server" {
  datacenters = ["dc1"]

  group "virt-group" {
    count = 1

    task "virt-task" {

      driver = "nomad-driver-virt"

    artifact {
        source      = "http://cloud-images.ubuntu.com/focal/current/focal-server-cloudimg-amd64.img"
        destination = "focal-server-cloudimg-amd64.img"
        mode        = "file"
      } 

      config {
        image                           = "focal-server-cloudimg-amd64.img"
        primary_disk_size               = 10000
        use_thin_copy                   = true
        default_user_password           = "password"
        cmds                            = ["python -m http.server 8000"]
      }

      resources {
        cpu    = 4
        memory = 4000
      }
    }
  }
}
client {
  enabled = true
  servers = ["${NOMAD_SERVER}:4647"]
}

plugin "nomad-driver-virt" {
  data_dir    = "/opt/ubuntu/virt_temp"
  image_paths = ["/var/local/statics/images/"]
}

plugin_dir = "/opt/nomad/plugins"
data_dir  = "/opt/ubuntu/nomad_tmp"
log_level = "INFO"

# Enable the client
client {
  enabled = true
  servers = ["${NOMAD_SERVER}:4647"]
}

plugin "nomad-driver-virt" {
  data_dir    = "/opt/ubuntu/virt_temp"
  image_paths = ["/var/local/statics/images/"]
}

# Increase log verbosity
log_level = "INFO"

# Setup data dir
data_dir  = "/opt/ubuntu/nomad_tmp"

# Set up the plugin dir
plugin_dir = "/opt/nomad/plugins"



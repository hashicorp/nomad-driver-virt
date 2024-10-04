// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

# Enable the client
client {
  enabled = true
  servers = ["${NOMAD_SERVER}:4647"]
}

plugin "nomad-driver-virt" {
  config {
    data_dir    = "/opt/ubuntu/virt_temp"
    image_paths = ["/var/local/statics/images/"]
  }
}

# Increase log verbosity
log_level = "DEBUG"

# Setup data dir
data_dir  = "/opt/nomad/client"

# Set up the plugin dir
plugin_dir = "/opt/nomad/plugins"

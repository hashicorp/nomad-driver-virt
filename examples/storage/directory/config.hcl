// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

# Increase log verbosity
log_level = "DEBUG"

# Setup data dir
data_dir = "/opt/nomad/data"

# Set up the plugin dir
plugin_dir = "/opt/nomad/plugins"

# Give the agent a unique name. Defaults to hostname
name = "virt-test"

plugin "nomad-driver-virt" {
  config {
    storage_pools {
      directory "local" {
        path = "/opt/nomad/virt/storage"
      }
    }
  }
}

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

# Configure host volumes
client {
  host_volume_plugin_dir = "/opt/nomad/volume-plugins"
  host_volumes_dir       = "/opt/nomad/virt/host-volumes"
}

plugin "nomad-driver-virt" {
  config {
    storage_pools {
      ceph "remote" {
        pool  = "nomad-pool"
        hosts = ["UPDATE_ME"]
        authentication {
          username = "nomad"
          secret   = "UPDATE_ME"
        }
      }

      directory "local" {
        path = "/opt/nomad/virt/storage"
      }

      default = "local"
    }
  }
}


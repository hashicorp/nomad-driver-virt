// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

type      = "host"
name      = "virt-data-dir"
plugin_id = "mkdir"

parameters = {
  mode = "0755"
  uid  = 1000
  gid  = 1000
}

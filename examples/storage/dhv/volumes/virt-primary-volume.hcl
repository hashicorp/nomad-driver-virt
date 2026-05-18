// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

name = "virt-primary-volume"
type = "host"

plugin_id    = "mkblock"
capacity_min = "10mib"
capacity_max = "5gib"

capability {
  access_mode     = "single-node-writer"
  attachment_mode = "block-device"
}

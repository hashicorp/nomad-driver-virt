// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:build !linux

package net

import (
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

func (c *Controller) Fingerprint(_ map[string]*structs.Attribute) {}

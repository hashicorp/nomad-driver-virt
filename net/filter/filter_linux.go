// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package filter

import "github.com/hashicorp/nomad-driver-virt/net/filter/linux"

// newFilter returns a new instance of the linux filter.
func newFilter() (Filter, error) {
	return linux.New()
}

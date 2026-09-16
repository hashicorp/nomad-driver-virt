// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

//go:build !linux

package filter

import (
	"fmt"

	"github.com/hashicorp/nomad-driver-virt/internal/errs"
)

// newFilter returns a not implemented error.
func newFilter() (Filter, error) {
	return nil, fmt.Errorf("network filter %w for this platform", errs.ErrNotImplemented)
}

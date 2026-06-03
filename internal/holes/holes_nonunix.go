// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

//go:build !unix

package holes

import "io"

// MakeReader is not supported, so will always return false.
func MakeReader(r io.Reader) (Reader, bool) {
	return nil, false
}

// Collect is not supported, so will always return an empty collection.
func Collect(f Reader) (Collection, error) {
	return &collection{}, nil
}

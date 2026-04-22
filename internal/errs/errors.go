// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package errs

import (
	"errors"
	"fmt"
)

var (
	// Base error types
	ErrNotFound             = errors.New("not found")
	ErrNotImplemented       = errors.New("not implemented")
	ErrNotSupported         = errors.New("not supported")
	ErrInvalidConfiguration = errors.New("invalid configuration")

	ErrMissingAttribute = fmt.Errorf("%w - missing required attribute", ErrInvalidConfiguration)
)

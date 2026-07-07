// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package errs

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

var (
	// Base error types
	ErrNotFound             = errors.New("not found")
	ErrNotImplemented       = errors.New("not implemented")
	ErrNotSupported         = errors.New("not supported")
	ErrInvalidConfiguration = errors.New("invalid configuration")
	ErrNetworking           = errors.New("networking error")
	ErrPacketFilter         = errors.New("packet filter error")

	// Configuration errors
	ErrMissingAttribute = fmt.Errorf("%w: missing required attribute", ErrInvalidConfiguration)

	// Networking errors
	ErrLoopbackNotEnabled = fmt.Errorf("%w: loopback port forwarding not enabled", ErrNetworking)

	// Packet filter errors
	ErrPacketFilterConfigure     = fmt.Errorf("%w: configure failure", ErrPacketFilter)
	ErrPacketFilterTeardown      = fmt.Errorf("%w: teardown failure", ErrPacketFilter)
	ErrPacketFilterCreateTable   = fmt.Errorf("%w: create table failure", ErrPacketFilter)
	ErrPacketFilterCreateChain   = fmt.Errorf("%w: create chain failure", ErrPacketFilter)
	ErrPacketFilterCreateRule    = fmt.Errorf("%w: create rule failure", ErrPacketFilter)
	ErrPacketFilterDeleteTable   = fmt.Errorf("%w: delete table failure", ErrPacketFilter)
	ErrPacketFilterDeleteChain   = fmt.Errorf("%w: delete chain failure", ErrPacketFilter)
	ErrPacketFilterDeleteRule    = fmt.Errorf("%w: delete rule failure", ErrPacketFilter)
	ErrPacketFilterConfiguration = fmt.Errorf("%w %w", ErrPacketFilter, ErrInvalidConfiguration)
)

// options are optional arguments used by helper functions.
type options struct {
	prefix string // prefix for error message
	suffix string // suffix for error message
}

type optionFn func(*options)

// WithPrefix adds the provided prefix to generated error messages.
func WithPrefix(prefix string) optionFn {
	return func(o *options) { o.prefix = prefix }
}

// WithSuffix adds the provided suffix to generated error messages.
func WithSuffix(suffix string) optionFn {
	return func(o *options) { o.suffix = suffix }
}

// MissingAttribute returns a wrapped ErrMissingAttribute error which includes
// the attribute name if the value is empty.
func MissingAttribute(attrName string, value any, opts ...optionFn) error {
	// Inspect the value and return if it was provided.
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Pointer:
		if !rv.IsNil() {
			return nil
		}
	case reflect.String, reflect.Slice, reflect.Map:
		if rv.Len() != 0 {
			return nil
		}
	default:
		return fmt.Errorf("%w - uncheckable attribute %s = %v (%T)", ErrNotSupported, attrName, value, value)
	}

	// The attribute is missing so set any passed options.
	o := &options{}
	for _, fn := range opts {
		fn(o)
	}

	// Construct the error message.
	tmpl := []string{"%w:"}
	args := []any{ErrMissingAttribute}
	if o.prefix != "" {
		tmpl = append([]string{"%s"}, tmpl...)
		args = append([]any{o.prefix}, args...)
	}
	tmpl = append(tmpl, "%s")
	args = append(args, attrName)
	if o.suffix != "" {
		tmpl = append(tmpl, "%s")
		args = append(args, o.suffix)
	}

	// Create and return the error.
	return fmt.Errorf(strings.Join(tmpl, " "), args...)
}

// Error wraps a base error and a runtime error into a new error.
func Error(baseErr, reportErr error) error {
	return fmt.Errorf("%w - %w", baseErr, reportErr)
}

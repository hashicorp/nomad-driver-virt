// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package convert

import (
	"testing"

	"github.com/shoenig/test/must"
)

func TestValidBytesString(t *testing.T) {
	t.Parallel()

	cases := []struct {
		value string
		valid bool
	}{
		// size patterns
		{value: "1", valid: true},
		{value: "1kb", valid: true},
		{value: "1KB", valid: true},
		{value: "1kib", valid: true},
		{value: "1KiB", valid: true},
		{value: "1mb", valid: true},
		{value: "1MB", valid: true},
		{value: "1mib", valid: true},
		{value: "1MiB", valid: true},
		{value: "1gb", valid: true},
		{value: "1GB", valid: true},
		{value: "1gib", valid: true},
		{value: "1GiB", valid: true},
		{value: "1tb", valid: true},
		{value: "1TB", valid: true},
		{value: "1tib", valid: true},
		{value: "1TiB", valid: true},
		{value: "1pb", valid: true},
		{value: "1PB", valid: true},
		{value: "1pib", valid: true},
		{value: "1PiB", valid: true},
		{value: "1eb", valid: true},
		{value: "1EB", valid: true},
		{value: "1eib", valid: true},
		{value: "1EiB", valid: true},
		// handles space
		{value: "1 GB", valid: true},
		{value: " 1 GB", valid: true},
		{value: " 1 GB ", valid: true},
		{value: "1 GB ", valid: true},
		// invalids
		{value: "ten"},
		{value: "1G"},
		{value: "1gig"},
		{value: "unknown"},
		{value: "1KaB"},
	}

	for _, tc := range cases {
		if tc.valid {
			must.True(t, ValidBytesString(tc.value),
				must.Sprintf("expected %q to be valid", tc.value))
		} else {
			must.False(t, ValidBytesString(tc.value),
				must.Sprintf("expected %q to be invalid", tc.value))
		}
	}
}

func TestToBytes(t *testing.T) {
	t.Run("valid bytes", func(t *testing.T) {
		result, err := ToBytes("20")
		must.NoError(t, err)
		must.Eq(t, 20, result)
	})

	t.Run("valid gigabytes", func(t *testing.T) {
		result, err := ToBytes("2GB")
		must.NoError(t, err)
		must.Eq(t, 2000000000, result)
	})

	t.Run("valid gibibytes", func(t *testing.T) {
		result, err := ToBytes("2GiB")
		must.NoError(t, err)
		must.Eq(t, 2147483648, result)
	})

	t.Run("invalid pattern", func(t *testing.T) {
		result, err := ToBytes("twenty bytes")
		must.ErrorContains(t, err, "cannot parse")
		must.Eq(t, 0, result)
	})
}

func TestMustToBytes(t *testing.T) {
	t.Run("valid value", func(t *testing.T) {
		must.Eq(t, 2147483648, MustToBytes("2GiB"))
	})

	t.Run("invalid value panics", func(t *testing.T) {
		must.Panic(t, func() { MustToBytes("invalid value") })
	})
}

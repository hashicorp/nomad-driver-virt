// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package errs

import (
	"testing"

	"github.com/shoenig/test/must"
)

func TestMissingAttribute(t *testing.T) {
	testCases := []struct {
		desc        string
		value       any
		attrName    string
		options     []optionFn
		errContains string // checks for substring in error message
		errContent  string // checks exact error message
		errIs       error  // target error for matching
		noErr       bool   // no error is expected
	}{
		{
			desc:     "ok - string",
			value:    "1",
			attrName: "test.value",
			noErr:    true,
		},
		{
			desc:     "ok - pointer",
			value:    &struct{}{},
			attrName: "test.value",
			noErr:    true,
		},
		{
			desc:     "ok - slice",
			value:    []string{"test"},
			attrName: "test.value",
			noErr:    true,
		},
		{
			desc:     "ok - map",
			value:    map[string]string{"test": "test"},
			attrName: "test.value",
			noErr:    true,
		},
		{
			desc:     "missing - string",
			value:    "",
			attrName: "test.value",
		},
		{
			desc:     "missing - pointer",
			value:    (*struct{})(nil),
			attrName: "test.value",
		},
		{
			desc:     "missing - slice",
			value:    []string{},
			attrName: "test.value",
		},
		{
			desc:     "missing - map",
			value:    make(map[string]string, 0),
			attrName: "test.value",
		},
		{
			desc:        "unsupported",
			value:       1,
			attrName:    "test.value",
			errIs:       ErrNotSupported,
			errContains: "uncheckable attribute test.value = 1 (int)",
		},
		{
			desc:       "missing - message",
			value:      "",
			attrName:   "test.value",
			errContent: "invalid configuration: missing required attribute: test.value",
		},
		{
			desc:       "missing - prefix",
			value:      "",
			attrName:   "test.value",
			options:    []optionFn{WithPrefix("test-prefix -")},
			errContent: "test-prefix - invalid configuration: missing required attribute: test.value",
		},
		{
			desc:       "missing - suffix",
			value:      "",
			attrName:   "test.value",
			options:    []optionFn{WithSuffix("- test-suffix")},
			errContent: "invalid configuration: missing required attribute: test.value - test-suffix",
		},
		{
			desc:       "missing - prefix and suffix",
			value:      "",
			attrName:   "test.value",
			options:    []optionFn{WithPrefix("test-prefix -"), WithSuffix("- test-suffix")},
			errContent: "test-prefix - invalid configuration: missing required attribute: test.value - test-suffix",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := MissingAttribute(tc.attrName, tc.value, tc.options...)
			if tc.noErr {
				must.NoError(t, err)
				return
			}

			chkErr := tc.errIs
			if chkErr == nil {
				chkErr = ErrMissingAttribute
			}

			must.ErrorIs(t, err, chkErr)

			if tc.errContains != "" {
				must.ErrorContains(t, err, tc.errContains)
			}

			if tc.errContent != "" {
				must.Eq(t, err.Error(), tc.errContent)
			}
		})
	}
}

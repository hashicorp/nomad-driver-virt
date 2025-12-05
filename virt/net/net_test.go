// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package net

import (
	"testing"

	"github.com/shoenig/test/must"
)

func Test_NetworkActiveString(t *testing.T) {
	testCases := []struct {
		name           string
		input          bool
		expectedOutput string
	}{
		{
			name:           "active",
			input:          true,
			expectedOutput: "active",
		},
		{
			name:           "inactive",
			input:          false,
			expectedOutput: "inactive",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualOutput := IsActiveString(tc.input)
			must.Eq(t, tc.expectedOutput, actualOutput)
		})
	}
}

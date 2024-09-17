// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package idset

import (
	"testing"

	"github.com/shoenig/test/must"
)

func Test_Parse(t *testing.T) {
	cases := []struct {
		input string
		exp   []uint16
		size  int
	}{
		{
			input: "0",
			exp:   []uint16{0},
			size:  1,
		},
		{
			input: "1,3,5,9",
			exp:   []uint16{1, 3, 5, 9},
			size:  4,
		},
		{
			input: "1-2",
			exp:   []uint16{1, 2},
			size:  2,
		},
		{
			input: "3-6",
			exp:   []uint16{3, 4, 5, 6},
			size:  4,
		},
		{
			input: "1,3-5,9,11-14",
			exp:   []uint16{1, 3, 4, 5, 9, 11, 12, 13, 14},
			size:  9,
		},
		{
			input: " 4-2 , 9-9 , 11-7\n",
			exp:   []uint16{2, 3, 4, 7, 8, 9, 10, 11},
			size:  8,
		},
	}

	for _, tc := range cases {
		t.Run("("+tc.input+")", func(t *testing.T) {
			result := Parse[uint16](tc.input)
			must.SliceContainsAll(t, tc.exp, result.Slice(), must.Sprint("got", result))
			must.Eq(t, tc.size, result.Size())
		})
	}
}

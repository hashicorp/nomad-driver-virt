// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package holes

import (
	"testing"

	"github.com/shoenig/test/must"
)

func TestHole(t *testing.T) {
	testHole := &hole{start: 10, end: 20}
	var nilHole *hole

	t.Run("StartsBefore", func(t *testing.T) {
		t.Run("position is before hole", func(t *testing.T) {
			must.False(t, testHole.StartsBefore(9))
		})

		t.Run("position is within hole", func(t *testing.T) {
			must.True(t, testHole.StartsBefore(12))
		})

		t.Run("position is after hole", func(t *testing.T) {
			must.True(t, testHole.StartsBefore(30))
		})

		t.Run("hole is nil", func(t *testing.T) {
			must.False(t, nilHole.StartsBefore(15))
		})
	})

	t.Run("Overflow", func(t *testing.T) {
		t.Run("position is before hole", func(t *testing.T) {
			must.Zero(t, testHole.Overflow(5))
		})

		t.Run("position is within hole", func(t *testing.T) {
			must.Eq(t, 5, testHole.Overflow(15))
		})

		t.Run("position is after hole", func(t *testing.T) {
			must.Eq(t, 20, testHole.Overflow(30))
		})

		t.Run("hole is nil", func(t *testing.T) {
			must.Zero(t, nilHole.Overflow(15))
		})
	})

	t.Run("Length", func(t *testing.T) {
		t.Run("hole is real", func(t *testing.T) {
			must.Eq(t, 10, testHole.Length())
		})

		t.Run("hole is nil", func(t *testing.T) {
			must.Zero(t, nilHole.Length())
		})
	})

	t.Run("Start", func(t *testing.T) {
		t.Run("hole is real", func(t *testing.T) {
			must.Eq(t, 10, testHole.Start())
		})

		t.Run("hole is nil", func(t *testing.T) {
			must.Eq(t, -1, nilHole.Start())
		})
	})

	t.Run("End", func(t *testing.T) {
		t.Run("hole is real", func(t *testing.T) {
			must.Eq(t, 20, testHole.End())
		})

		t.Run("hole is nil", func(t *testing.T) {
			must.Eq(t, -1, nilHole.End())
		})
	})
}

var _ Hole = (*hole)(nil)

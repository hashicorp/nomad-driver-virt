// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package holes

import (
	"testing"

	"github.com/shoenig/test/must"
)

func TestCollection(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		c := new(collection)

		must.Zero(t, c.Count(), must.Sprint("empty collection should have no count"))
		must.Nil(t, c.Next(), must.Sprint("next on empty collection should be nil"))
	})

	t.Run("populate", func(t *testing.T) {
		c := new(collection)

		for i := range int64(5) {
			c.Add(&hole{start: i, end: i})
		}

		must.Eq(t, 5, c.Count(), must.Sprint("expected five holes in collection"))
		// Next must return in FIFO order
		for i := range int64(5) {
			must.Eq(t, int(5-i), c.Count(), must.Sprint("unexpected count for collection"))
			h := c.Next()
			must.Eq(t, i, h.Start(), must.Sprint("unexpected start position for hole"))
		}

		// Collection should be empty now
		must.Zero(t, c.Count(), must.Sprint("expected collection to be empty"))
		// Next returns nil
		must.Nil(t, c.Next(), must.Sprint("next hole from empty collection should be nil"))
	})
}

var _ Collection = (*collection)(nil)

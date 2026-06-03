// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

//go:build unix

package holes

import (
	"os"
	"testing"

	"github.com/shoenig/test/must"
)

func TestHoles(t *testing.T) {
	// The sparse image produced includes two holes with known positions
	// in the file (4096-12288 and 20480-40960)
	sparseImg, fullImg := TestFiles(t, t.TempDir())

	t.Run("MakeReader", func(t *testing.T) {
		t.Run("with holes", func(t *testing.T) {
			f, err := os.Open(sparseImg)
			must.NoError(t, err)

			_, ok := MakeReader(f)
			must.True(t, ok)
		})

		t.Run("without holes", func(t *testing.T) {
			f, err := os.Open(fullImg)
			must.NoError(t, err)

			_, ok := MakeReader(f)
			must.False(t, ok)
		})
	})

	t.Run("Collect", func(t *testing.T) {
		t.Run("with holes", func(t *testing.T) {
			var f Reader
			f, err := os.Open(sparseImg)
			must.NoError(t, err)
			c, err := Collect(f)
			must.NoError(t, err)
			must.Eq(t, 2, c.Count(), must.Sprint("expected two holes in file"))
			h := c.Next()
			must.NotNil(t, h)
			must.Eq(t, 4096, h.Start(), must.Sprint("incorrect start position for first hole"))
			must.Eq(t, 12288, h.End(), must.Sprint("incorrect end position for first hole"))
			h = c.Next()
			must.NotNil(t, h)
			must.Eq(t, 20480, h.Start(), must.Sprint("incorrect start position for second hole"))
			must.Eq(t, 40960, h.End(), must.Sprint("incorrect end position for second hole"))
			h = c.Next()
			must.Nil(t, h)
		})

		t.Run("without holes", func(t *testing.T) {
			var f Reader
			f, err := os.Open(fullImg)
			must.NoError(t, err)
			c, err := Collect(f)
			must.NoError(t, err)
			must.Zero(t, c.Count(), must.Sprint("expected no holes in file"))
		})
	})
}

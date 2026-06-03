// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package holes

import (
	"os"
)

// Reader provides an interface with needed file-like functions.
type Reader interface {
	ReadAt([]byte, int64) (int, error)
	Read([]byte) (int, error)
	Seek(int64, int) (int64, error)
	Stat() (os.FileInfo, error)
}

// Collection contains a list of holes from a file.
type Collection interface {
	// Add adds a new hole to the collection.
	Add(Hole)

	// Count returns the number of holes in the collection.
	Count() int

	// Next removes the next hole from the collection and returns
	// it, or nil if no holes remain.
	Next() Hole
}

// Hole represents a hole within a sparse file.
type Hole interface {
	// End returns the end position of the hole.
	End() int64

	// StartsBefore returns if the start of the hole is before
	// or at the provided position.
	StartsBefore(pos int64) bool

	// Length returns the length of the hole.
	Length() int64

	// Overflow returns the distance of the provided position
	// past the start of the hole.
	Overflow(pos int64) int

	// Start returns the start position of the hole.
	Start() int64
}

// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package holes

// hole represents a hole within a sparse file.
type hole struct {
	start int64 // start position of hole.
	end   int64 // end position of hole.
}

// StartsBefore returns if the start of the hole is before
// or at the provided position.
func (h *hole) StartsBefore(pos int64) bool {
	if h == nil {
		return false
	}

	return pos >= h.start
}

// Overflow returns how far the provided position is past the start of the hole.
func (h *hole) Overflow(pos int64) int {
	if h == nil {
		return 0
	}

	over := int(pos - h.start)

	if over < 0 {
		return 0
	}

	return over
}

// Length returns the length of the hole.
func (h *hole) Length() int64 {
	if h == nil {
		return 0
	}

	return h.end - h.start
}

// Start returns the start position of the hole.
func (h *hole) Start() int64 {
	if h == nil {
		return -1
	}

	return h.start
}

// End returns the end position of the hole.
func (h *hole) End() int64 {
	if h == nil {
		return -1
	}

	return h.end
}

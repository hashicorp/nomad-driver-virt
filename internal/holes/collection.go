// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package holes

// collection contains a list of holes from a file.
type collection struct {
	holes []Hole
}

// Add adds a new hole to the collection.
func (c *collection) Add(h Hole) {
	c.holes = append(c.holes, h)
}

// Count returns the number of holes in the collection.
func (c *collection) Count() int {
	return len(c.holes)
}

// Next removes the next hole from the collection and returns
// it, or nil if no holes remain.
func (c *collection) Next() Hole {
	if len(c.holes) < 1 {
		return nil
	}

	h := c.holes[0]
	c.holes = c.holes[1:]
	return h
}

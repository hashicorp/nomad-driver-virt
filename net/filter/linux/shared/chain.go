// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package shared

// Chain represents a filter chain.
type Chain struct {
	Table string // table name
	Chain string // chain name
	stamp uint   // sorting value for collections
}

// Equal returns if the provided Chain is equal. Equality
// is based off the hash value.
func (c Chain) Equal(rhs Chain) bool {
	return c.Hash() == rhs.Hash()
}

// Hash returns a unique string for the chain.
func (c Chain) Hash() string {
	return c.Table + "-" + c.Chain
}

// setStamp sets the stamp value on the chain.
func (c Chain) setStamp(stamp uint) stampable {
	c.stamp = stamp
	return c
}

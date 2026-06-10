// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/go-set/v3"
)

type Rules [][]string

// Empty returns if collection is empty.
func (r Rules) Empty() bool {
	return len(r) == 0
}

// rules converts the raw slice into a collection of rules.
func (r Rules) rules() set.Collection[*rule] {
	result := set.NewHashSet[*rule](0)
	for _, entry := range r {
		if len(entry) < 3 {
			// invalid rule so skip.
			continue
		}
		result.Insert(&rule{
			table: entry[0],
			chain: entry[1],
			spec:  entry[2:],
		})
	}

	return result
}

// stampFn is the signature for a stamping function.
type stampFn func(stampable)

// stampable describes a type that can be stamped.
type stampable interface {
	setStamp(uint)
}

// newChains creates a new chains collection with a stamping
// function to stamp chains.
func newChains(fn stampFn) *chains {
	return &chains{
		HashSet: set.NewHashSet[*chain](0),
		stampFn: fn,
	}
}

// chains is a collection of chain pointers.
type chains struct {
	*set.HashSet[*chain, string]
	stampFn
}

// Insert inserts a chain into the collection.
func (c *chains) Insert(item *chain) bool {
	if c.stampFn != nil {
		c.stampFn(item)
	}

	return c.HashSet.Insert(item)
}

// InsertSlice inserts a chain slice into the collection.
func (c *chains) InsertSlice(items []*chain) bool {
	if c.stampFn != nil {
		for _, i := range items {
			c.stampFn(i)
		}
	}

	return c.HashSet.InsertSlice(items)
}

// InsertSet inserts a chain set into the collection.
func (c *chains) InsertSet(items set.Collection[*chain]) bool {
	if c.stampFn != nil {
		for _, i := range items.Slice() {
			c.stampFn(i)
		}
	}

	return c.HashSet.InsertSet(items)
}

// Slice returns the chain slice of the collection, sorted by stamp.
func (c *chains) Slice() []*chain {
	s := c.HashSet.Slice()
	slices.SortFunc(s, func(a, b *chain) int { return cmp.Compare(a.stamp, b.stamp) })
	return s
}

// chain represents an iptables chain.
type chain struct {
	table string // table name
	chain string // chain name
	stamp uint   // sorting value for collections
}

// setStamp sets the stamp value on the chain.
func (c *chain) setStamp(stamp uint) {
	c.stamp = stamp
}

// Equal returns if the chains are equal.
func (c *chain) Equal(rhs *chain) bool {
	if c == nil || rhs == nil {
		return c == rhs
	}

	return c.table == rhs.table &&
		c.chain == rhs.chain
}

// Hash returns a unique string for the chain.
func (c *chain) Hash() string {
	return c.table + c.chain
}

// newRules creates a new rules collection with a stamping
// function to stamp rules.
func newRules(fn stampFn) *rules {
	return &rules{
		HashSet: set.NewHashSet[*rule](0),
		stampFn: fn,
	}
}

// rules is a collection of rule pointers.
type rules struct {
	*set.HashSet[*rule, string]
	stampFn
}

// Insert inserts a rule into the collection.
func (r *rules) Insert(item *rule) bool {
	if r.stampFn != nil {
		r.stampFn(item)
	}

	return r.HashSet.Insert(item)
}

// InsertSlice inserts a rule slice into the collection.
func (r *rules) InsertSlice(items []*rule) bool {
	if r.stampFn != nil {
		for _, i := range items {
			r.stampFn(i)
		}
	}

	return r.HashSet.InsertSlice(items)
}

// InsertSet inserts a rule set into the collection.
func (r *rules) InsertSet(items set.Collection[*rule]) bool {
	if r.stampFn != nil {
		for _, i := range items.Slice() {
			r.stampFn(i)
		}
	}

	return r.HashSet.InsertSet(items)
}

// Slice returns the rule slice of the collection, sorted by stamp.
func (r *rules) Slice() []*rule {
	s := r.HashSet.Slice()
	slices.SortFunc(s, func(a, b *rule) int { return cmp.Compare(a.stamp, b.stamp) })
	return s
}

// removables returns all rules marked as removable.
func (r *rules) removables() *rules {
	result := []*rule{}
	for _, item := range r.Slice() {
		if item.removable {
			rm := *item
			result = append(result, &rm)
		}
	}

	return &rules{
		HashSet: set.HashSetFrom(result),
		stampFn: r.stampFn,
	}
}

// rule represents an iptables rule.
type rule struct {
	table     string   // table name.
	chain     string   // chain name.
	position  int      // position of the rule if it should be inserted.
	spec      []string // rule specification.
	removable bool     // rule should be removed during teardown.
	stamp     uint     // sorting value for collections.
}

// setStamp sets the stamp value on the rule.
func (r *rule) setStamp(stamp uint) {
	r.stamp = stamp
}

// Equal returns if the rules are equal.
func (r *rule) Equal(rhs *rule) bool {
	if r == nil || rhs == nil {
		return r == rhs
	}

	return r.table == rhs.table &&
		r.chain == rhs.chain &&
		r.position == rhs.position &&
		slices.Equal(r.spec, rhs.spec)
}

// Hash returns a unique string for the rule.
func (r *rule) Hash() string {
	return fmt.Sprintf("%s%s%s", r.table, r.chain, strings.Join(r.spec, ""))
}

// String returns a string representation of the rule.
func (r *rule) String() string {
	return fmt.Sprintf("-A %s %s", r.chain, strings.Join(r.spec, " "))
}

// slice converts the rule into a string slice.
func (r *rule) slice() []string {
	return append([]string{r.table, r.chain}, r.spec...)
}

// mkchain builds a chain from the rule.
func (r *rule) mkchain() *chain {
	return &chain{table: r.table, chain: r.chain}
}

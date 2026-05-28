// Copyright IBM Corp. 2024, 2025
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

// chains is a slice of chain pointers.
type chains []*chain

// sort sorts the chain slice by the stamp value.
func (c chains) sort() chains {
	slices.SortFunc(c, func(a, b *chain) int { return cmp.Compare(a.stamp, b.stamp) })
	return c
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

// rules is a slice of rule pointers.
type rules []*rule

// sort sorts the rule slice by the stamp value.
func (r rules) sort() rules {
	slices.SortFunc(r, func(a, b *rule) int { return cmp.Compare(a.stamp, b.stamp) })
	return r
}

// removables returns all rules marked as removable.
func (r rules) removables() rules {
	return slices.Collect(func(yield func(*rule) bool) {
		for _, rule := range r {
			if rule.removable && !yield(rule) {
				return
			}
		}
	})
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

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"fmt"
	"slices"
	"strings"

	"github.com/hashicorp/go-set/v3"
)

// Rules is a raw collection of rules.
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
	if c == nil && rhs == nil {
		return true
	}

	if c == nil || rhs == nil {
		return false
	}

	return c.table == rhs.table && c.chain == rhs.chain
}

// Hash returns a unique string for the chain.
func (c *chain) Hash() string {
	return c.table + c.chain
}

// rule represents an iptables rule.
type rule struct {
	table    string   // table name
	chain    string   // chain name
	position int      // position of the rule if it should be inserted
	spec     []string // rule specification
	teardown bool     // rule should be included in teardown list
	stamp    uint     // sorting value for collections
}

// setStamp sets the stamp value on the rule.
func (r *rule) setStamp(stamp uint) {
	r.stamp = stamp
}

// Equal returns if the rules are equal.
func (r *rule) Equal(rhs *rule) bool {
	if r == nil && rhs == nil {
		return true
	}

	if r == nil || rhs == nil {
		return false
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

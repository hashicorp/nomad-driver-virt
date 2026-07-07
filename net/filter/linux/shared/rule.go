// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package shared

import "fmt"

// Rule represents a filter rule.
type Rule struct {
	Chain      Chain
	Spec       Spec
	Position   int
	Identifier string
	Removable  bool

	stamp uint
}

func (r Rule) Equal(rhs Rule) bool {
	return r.Hash() == rhs.Hash()
}

// setStamp sets the stamp value on the rule.
func (r Rule) setStamp(stamp uint) stampable {
	r.stamp = stamp

	return r
}

// setIdentifier sets the identifier value on the rule if the rule
// is removable.
func (r Rule) setIdentifier(ident string) identifiable {
	if r.Removable {
		r.Identifier = ident
	}

	return r
}

// Hash returns a unique string for the rule.
func (r Rule) Hash() string {
	return fmt.Sprintf("%s-%s-%d", r.Chain.Hash(), r.Spec.Hash(), r.Position)
}

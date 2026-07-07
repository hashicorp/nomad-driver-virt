// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package shared

import "fmt"

// Spec represents the specification of a rule.
type Spec struct {
	Destination     string
	DestinationPort int
	DestinationType string
	InInterface     string
	Jump            string
	OutInterface    string
	Protocol        string
	Source          string
	SourceType      string
	State           string
	ToDestination   string
}

// Hash returns a unique string for the spec.
func (s Spec) Hash() string {
	return fmt.Sprintf("%#v", s)
}

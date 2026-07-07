// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package shared

type Teardown struct {
	Chains      []Chain // Chains to remove (currently this is only for internal testing).
	RulesWithin []Chain // Chains that contain rules to remove.
	Identifier  string  // Identifier value to match rules.
}

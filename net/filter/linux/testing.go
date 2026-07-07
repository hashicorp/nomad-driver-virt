// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package linux

import (
	"github.com/hashicorp/nomad/helper/uuid"
)

// testingT is a minimal testing.T interface.
type testingT interface {
	Cleanup(func())
}

// WithTestNames configures the tables instance with generated
// names to prevent collisions.
func WithTestNames() option {
	return func(t *tables) {
		t.names = TestNewNames()
	}
}

// WithBackend configures the tables instance with provided
// backend implementation.
func WithBackend(b Backend) option {
	return func(t *tables) {
		t.backend = b
	}
}

// WithBackendCleanup registers a cleanup function to remove
// testing related modifications in the packet filter.
func WithBackendCleanup(in testingT) option {
	return func(t *tables) {
		in.Cleanup(t.cleanup)
	}
}

// TestNewNames creates a new instance with all nomad chain name values
// set to generated testing names.
func TestNewNames() *names {
	return &names{
		holder: genTestName(defaultHolderName),
		chains: &ChainNames{
			Forward:     defaultChainNameForward,
			Output:      defaultChainNameOutput,
			Postrouting: defaultChainNamePostrouting,
			Prerouting:  defaultChainNamePrerouting,
			Nomad: &NomadChainNames{
				Forward:     genTestName(defaultChainNameNomadForward),
				Postrouting: genTestName(defaultChainNameNomadPostrouting),
				Prerouting:  genTestName(defaultChainNameNomadPrerouting),
				Output:      genTestName(defaultChainNameNomadOutput),
			},
		},
		tables: &TableNames{
			Filter: defaultTableNameFilter,
			NAT:    defaultTableNameNAT,
		},
	}
}

// genTestName appends a short uuid to the prefix to provide a unique
// name for testing.
func genTestName(prefix string) string {
	return prefix + "_" + uuid.Short()
}

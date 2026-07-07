// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package iptables

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/coreos/go-iptables/iptables"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	"github.com/hashicorp/nomad-driver-virt/net/filter/linux/shared"
)

const (
	// name of the backend.
	name = "iptables"
)

// Interface for iptables which defines the subset of functions
// that are currently used. This allows for easily swapping out
// implementations for testing.
type IPTables interface {
	AppendUnique(table, chain string, rulespec ...string) error
	ChainExists(table, chain string) (bool, error)
	ClearAndDeleteChain(table, chain string) error
	DeleteById(table, chain string, id int) error
	InsertUnique(table, chain string, pos int, rulespec ...string) error
	List(table, chain string) ([]string, error)
	NewChain(table, chain string) error
}

// New creates a new iptables backend.
func New(logger hclog.Logger) (*ipt, error) {
	i, err := iptables.New()
	if err != nil {
		return nil, err
	}

	return &ipt{
		ipt:    i,
		logger: logger.Named("iptables"),
	}, nil
}

// ipt implements an IPTables backend for a linux packet filter.
type ipt struct {
	ipt    IPTables
	logger hclog.Logger

	m sync.Mutex
}

// Name returns the name of the backend.
func (i *ipt) Name() string {
	return name
}

// Add adds chains and rules based on the request.
func (i *ipt) Add(req *shared.Request) error {
	i.m.Lock()
	defer i.m.Unlock()

	// Start with creating any needed chains.
	for _, c := range req.Chains() {
		exists, err := i.ipt.ChainExists(c.Table, c.Chain)
		if err != nil {
			return fmt.Errorf("%w - chain exists check failure: %w", errs.ErrPacketFilter, err)
		}

		if !exists {
			if err := i.ipt.NewChain(c.Table, c.Chain); err != nil {
				return errs.Error(errs.ErrPacketFilterCreateChain, err)
			}
		}
	}

	// Now add any rules that remain. If a position is defined, then the rule
	// is inserted, otherwise it is appended.
	for _, r := range req.Rules() {
		rule := generateRule(r)
		i.logger.Trace("generated new iptables rule", "source", hclog.Fmt("%#v", r), "rule", hclog.Fmt("%#v", rule))

		var err error
		if r.Position > 0 {
			err = i.ipt.InsertUnique(rule.table, rule.chain, r.Position, rule.spec...)
		} else {
			err = i.ipt.AppendUnique(rule.table, rule.chain, rule.spec...)
		}

		if err != nil {
			return errs.Error(errs.ErrPacketFilterCreateRule, err)
		}

		i.logger.Debug("new iptables rule added", "rule", hclog.Fmt("%#v", rule))
	}

	return nil
}

// Remove removes chains and rules based on the teardown.
func (i *ipt) Remove(teardown shared.Teardown) error {
	i.m.Lock()
	defer i.m.Unlock()

	var mErr *multierror.Error

	// Delete any rules that are found with the identifier.
	for _, chain := range teardown.RulesWithin {
		rules, err := i.ipt.List(chain.Table, chain.Chain)
		if err != nil {
			mErr = multierror.Append(mErr, err)
			continue
		}

		// Process the rules in reverse order so we can delete by entry.
		// NOTE: The list result include the creation of the chain as
		// the first entry. Because of this, it allows our idx value to
		// be the same for indexing the rule in the list and deleting it
		// by ID (which is really just line number).
		for idx := len(rules) - 1; idx >= 0; idx-- {
			rule := rules[idx]

			// If the identifier isn't detected within the rule it should
			// not be deleted.
			if !strings.Contains(rule, teardown.Identifier) {
				continue
			}

			if err := i.ipt.DeleteById(chain.Table, chain.Chain, idx); err != nil {
				i.logger.Error("failed to delete iptables rule", "table", chain.Table, "chain", chain.Chain,
					"rule", rule, "error", err)
				mErr = multierror.Append(mErr, errs.Error(errs.ErrPacketFilterDeleteRule, err))
				continue
			}

			i.logger.Debug("iptables rule deleted", "table", chain.Table, "chain", chain.Chain, "rule", rule)
		}
	}

	// Delete any chains requested for removal.
	for _, chain := range teardown.Chains {
		if err := i.ipt.ClearAndDeleteChain(chain.Table, chain.Chain); err != nil {
			i.logger.Error("failed to delete iptables chain", "table", chain.Table, "chain", chain.Chain,
				"error", err)
			mErr = multierror.Append(mErr, errs.Error(errs.ErrPacketFilterDeleteChain, err))
			continue
		}
		i.logger.Debug("iptables chain deleted", "table", chain.Table, "chain", chain.Chain)
	}

	return nil
}

// rule is an internal representation of a rule for iptables.
type rule struct {
	table string
	chain string
	spec  []string
}

// generateRule converts a rule to an iptables specific rule.
func generateRule(src shared.Rule) rule {
	r := rule{table: src.Chain.Table, chain: src.Chain.Chain}

	if src.Spec.Protocol != "" {
		r.spec = append(r.spec, "--protocol", src.Spec.Protocol)
	}
	if src.Spec.Source != "" {
		r.spec = append(r.spec, "--source", src.Spec.Source)
	}
	if src.Spec.Destination != "" {
		r.spec = append(r.spec, "--destination", src.Spec.Destination)
	}
	if src.Spec.InInterface != "" {
		r.spec = append(r.spec, "--in-interface", src.Spec.InInterface)
	}
	if src.Spec.OutInterface != "" {
		r.spec = append(r.spec, "--out-interface", src.Spec.OutInterface)
	}

	if src.Spec.DestinationPort > 0 {
		protoMatch := "tcp"
		if src.Spec.Protocol != "" {
			protoMatch = src.Spec.Protocol
		}
		r.spec = append(r.spec, "--match", protoMatch,
			"--dport", strconv.Itoa(src.Spec.DestinationPort))
	}

	if src.Spec.SourceType != "" || src.Spec.DestinationType != "" {
		r.spec = append(r.spec, "--match", "addrtype")
		if src.Spec.SourceType != "" {
			r.spec = append(r.spec, "--src-type", src.Spec.SourceType)
		}
		if src.Spec.DestinationType != "" {
			r.spec = append(r.spec, "--dst-type", src.Spec.DestinationType)
		}
	}

	if src.Spec.State != "" {
		r.spec = append(r.spec, "--match", "state",
			"--state", src.Spec.State)
	}

	if src.Spec.Jump != "" {
		r.spec = append(r.spec, "--jump", src.Spec.Jump)
	}

	if src.Spec.ToDestination != "" {
		r.spec = append(r.spec, "--to-destination", src.Spec.ToDestination)
	}

	// If an identifier is defined, add it as a comment so the rule
	// can be detected for deletion.
	if src.Identifier != "" {
		r.spec = append(r.spec, "--match", "comment", "--comment", src.Identifier)
	}

	return r
}

// isNotExistErr is a helper to check if the error was caused
// by a rule or chain not existing.
func isNotExistErr(err error) bool {
	iptErr, ok := err.(*iptables.Error)
	if !ok {
		return false
	}

	return iptErr.IsNotExist()
}

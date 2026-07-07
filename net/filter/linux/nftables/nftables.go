// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

//go:build linux

// An important note about how this backend is implemented.
// While rules that are added and removed using this backend
// may define a chain on a specific table, all chains are
// contained within a single isolated table. Doing this makes
// it much easier to manage, and cleanup, our rules. The names
// of the chains within our table will be a combination of the
// table name and chain name making it easy to map back to the
// requests that created them.
package nftables

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/google/nftables"
	"github.com/google/nftables/binaryutil"
	"github.com/google/nftables/expr"
	"github.com/google/nftables/userdata"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-multierror"
	"github.com/hashicorp/go-set/v3"
	"github.com/hashicorp/nomad-driver-virt/internal/errs"
	"github.com/hashicorp/nomad-driver-virt/net/filter/linux/shared"
	"golang.org/x/sys/unix"
)

const (
	// name of the backend.
	name = "nftables"

	// null byte for terminating strings.
	nullByte = "\x00"
)

var (
	baseTypes = map[string]nftables.ChainType{
		"filter": nftables.ChainTypeFilter,
		"nat":    nftables.ChainTypeNAT,
		"route":  nftables.ChainTypeRoute,
	}
	baseHooks = map[string]*nftables.ChainHook{
		"input":       nftables.ChainHookInput,
		"forward":     nftables.ChainHookForward,
		"output":      nftables.ChainHookOutput,
		"postrouting": nftables.ChainHookPostrouting,
		"prerouting":  nftables.ChainHookPrerouting,
	}
)

// NFTables is the interface defining the functions that
// are actually used from the nftables library.
type NFTables interface {
	AddChain(*nftables.Chain) *nftables.Chain
	AddRule(*nftables.Rule) *nftables.Rule
	CreateTable(*nftables.Table) *nftables.Table
	DelChain(*nftables.Chain)
	DelRule(*nftables.Rule) error
	DelTable(*nftables.Table)
	Flush() error
	GetRules(*nftables.Table, *nftables.Chain) ([]*nftables.Rule, error)
	InsertRule(*nftables.Rule) *nftables.Rule
	ListChain(*nftables.Table, string) (*nftables.Chain, error)
	ListTableOfFamily(string, nftables.TableFamily) (*nftables.Table, error)
}

// New creates a new nftables backend.
func New(logger hclog.Logger, name string) (*nft, error) {
	n, err := nftables.New()
	if err != nil {
		return nil, err
	}

	nt := &nft{
		nft:    n,
		logger: logger.Named("nftables"),
		table:  name,
	}

	if err := nt.setup(); err != nil {
		return nil, err
	}

	return nt, nil
}

// nft implements an NFTables backed for a linux packet filter.
type nft struct {
	nft    NFTables
	logger hclog.Logger
	table  string

	m sync.Mutex
}

// SetTableName sets the name of the table containing all the
// nomad virt chains. Used for testing.
func (n *nft) SetTableName(name string) {
	n.table = name
}

// setup will create an isolated table for nomad virt chains.
func (n *nft) setup() error {
	// Check if our custom table exists.
	if _, err := n.getTable(); err == nil {
		return nil
	}

	// Create our isolated table.
	_ = n.nft.CreateTable(&nftables.Table{
		Name:   n.table,
		Family: nftables.TableFamilyINet,
	})

	// Flush to actually create the table.
	if err := n.nft.Flush(); err != nil {
		n.logger.Error("failure during setup", "error", err)
		return err
	}

	return nil
}

// Name returns the name of the backend.
func (n *nft) Name() string {
	return name
}

// Add adds chains and rules based on the request.
func (n *nft) Add(req *shared.Request) error {
	n.m.Lock()
	defer n.m.Unlock()

	// Start with grabbing our isolated table.
	table, err := n.getTable()
	if err != nil {
		return errs.Error(errs.ErrPacketFilter, err)
	}

	// We create base chains on demand (these are the chains
	// with hooks defined to actually receive packets). Check
	// the list of rules for any base chains and add them to
	// the list of chains to create if found.
	chains := set.NewHashSet[shared.Chain](0)
	chains.InsertSlice(req.Chains())
	for _, r := range req.Rules() {
		if needsHook(normalizedChain(r.Chain)) {
			chains.Insert(r.Chain)
		}
	}

	// Define a holder for the save function that we'll set only
	// if modifications have been staged.
	var saveFn func() error

	for _, c := range chains.Slice() {
		// Create a normalized version of the chain.
		c = normalizedChain(c)

		// If the chain can be loaded then it doesn't
		// need to be created.
		_, err := n.getChain(c)
		if err == nil {
			continue
		}

		// Create the content for a regular chain.
		chain := &nftables.Chain{
			Name:  chainName(c),
			Table: table,
		}

		// If the chain needs a hook then it is a base chain. Add
		// the configuration to properly hook the chain to receive
		// packets.
		if needsHook(c) {
			chain.Type = baseTypes[c.Table]
			chain.Hooknum = baseHooks[c.Chain]
			chain.Priority = chainPriority(c)
			chain.Policy = new(nftables.ChainPolicyAccept)
		}

		// Add the new chain.
		n.logger.Debug("adding new chain", "table", n.table, "chain", chainName(c))
		n.nft.AddChain(chain)

		// We've modified things so set the save function.
		saveFn = n.nft.Flush
	}

	// If new chains were added, persist the changes.
	if saveFn != nil {
		if err := saveFn(); err != nil {
			n.logger.Error("failure saving new chains", "error", err)
			return errs.Error(errs.ErrPacketFilterCreateChain, err)
		}
	}

	// Reset the save function holder.
	saveFn = nil

	// Positioned rules will need to be added in a final pass.
	positionedRules := make([]*nftables.Rule, 0)

	// Now create any rules that are defined.
RULES_LOOP:
	for _, r := range req.Rules() {
		// Convert the rule request into an nftables rule.
		rule, err := n.generateRule(r)
		if err != nil {
			n.logger.Error("failure generating rule", "error", err)
			return errs.Error(errs.ErrPacketFilterCreateRule, err)
		}

		// If the logger is outputting trace lines, log the rule expressions.
		if n.logger.GetLevel() == hclog.Trace {
			exprs := []any{}
			for _, exp := range rule.Exprs {
				val := reflect.ValueOf(exp)
				if val.Kind() != reflect.Pointer {
					exprs = append(exprs, val.Interface())
				} else {
					exprs = append(exprs, val.Elem().Interface())
				}
			}
			n.logger.Trace("adding rule", "rule", hclog.Fmt("%#v", r), "exprs", hclog.Fmt("%#v", exprs))
		}

		// If this rule defines a position, add it to the positioned rules
		// collection to be processed after non-positioned rules. Allow processing
		// of the rule to continue so we can determine if an existing equivalent
		// rule should be removed.
		if rule.Position != 0 {
			positionedRules = append(positionedRules, rule)
		}

		// Grab the existing rules on the chain.
		existingRules, err := n.nft.GetRules(rule.Table, rule.Chain)
		if err != nil {
			n.logger.Error("cannot list existing rules", "table", rule.Table.Name, "chain", rule.Chain.Name)
			return fmt.Errorf("%w - cannot list rules: %w", errs.ErrPacketFilter, err)
		}

		// Check if the rule already exists.
		for _, existingRule := range existingRules {
			// If the rule exists and no position is set, skip.
			if rule.Position == 0 && equalRules(existingRule, rule) {
				continue RULES_LOOP
			}

			// If the existing rule is equivalent to the new rule, and it's just making
			// changes to flags or user data, then it can be replaced. We can do that by
			// setting the handle from the existing rule into the new rule and adding it
			// below. If the new rule includes a position, then the existing rule must
			// be deleted so it can be inserted at the correct location on the next pass.
			if equivalentRules(existingRule, rule) {
				if rule.Position != 0 {
					if err := n.nft.DelRule(existingRule); err != nil {
						n.logger.Error("failure deleting existing rule for replacement", "error", err)
						return fmt.Errorf("%w - rule replacement failure: %w", errs.ErrPacketFilterCreateRule, err)
					}
					continue RULES_LOOP
				}
				rule.Handle = existingRule.Handle
			}
		}

		// Don't add any rules with a position defined.
		if rule.Position != 0 {
			continue
		}

		// Add the new rule.
		n.nft.AddRule(rule)

		// We've modified things so set the save function.
		saveFn = n.nft.Flush
	}

	// If new rules were added, persist the changes.
	if saveFn != nil {
		if err := saveFn(); err != nil {
			n.logger.Error("failed saving new rules", "error", err)
			return errs.Error(errs.ErrPacketFilterCreateRule, err)
		}
	}

	// If there are not positioned rules, then we're done here.
	if len(positionedRules) == 0 {
		return nil
	}

	// A position provided in a request is the position within the chain. We
	// need the handle of an existing rule for relative positioning of the new
	// rule when adding it. Reverse the order of positionedRules so the most
	// recently added items have precedence for the position.
	slices.Reverse(positionedRules)

	for _, rule := range positionedRules {
		// Grab the existing rules on the chain.
		existingRules, err := n.nft.GetRules(rule.Table, rule.Chain)
		if err != nil {
			n.logger.Error("cannot list existing rules", "table", rule.Table.Name, "chain", rule.Chain.Name)
			return fmt.Errorf("%w - cannot list rules: %w", errs.ErrPacketFilter, err)
		}

		// If there are no existing rules, remove the position and just add
		// the new rule.
		if len(existingRules) == 0 {
			rule.Position = 0
			n.nft.AddRule(rule)
			continue
		}

		// The actual position in the collection will zero indexed so adjust.
		desiredPosition := int(rule.Position - 1)

		// The index of the rule to reference will be either the rule at the
		// desired position, or the last rule in the collection.
		positionIndex := min(desiredPosition, len(existingRules)-1)

		// Grab the reference rule.
		referenceRule := existingRules[positionIndex]

		// Set the position of the new rule to the handle of the reference rule.
		rule.Position = referenceRule.Handle

		// And insert the rule so it is placed above the reference rule.
		n.nft.InsertRule(rule)
	}

	// Persist the added rules.
	if err := n.nft.Flush(); err != nil {
		n.logger.Error("failed saving positioned rules", "error", err)
		return errs.Error(errs.ErrPacketFilterCreateRule, err)
	}

	return nil
}

// Remove removes chains and rules based on the teardown.
func (n *nft) Remove(teardown shared.Teardown) error {
	n.m.Lock()
	defer n.m.Unlock()

	var mErr *multierror.Error

	// Define a holder for the save function that we'll set only
	// if modifications have been staged.
	var saveFn func() error

	for _, c := range teardown.RulesWithin {
		c = normalizedChain(c)
		chain, err := n.getChain(c)
		if err != nil {
			// If the error is due to the chain not existing, ignore.
			if errors.Is(err, errs.ErrNotFound) {
				continue
			}

			n.logger.Warn("failed to load chain for rule removal", "table", n.table, "chain", chainName(c))
			mErr = multierror.Append(mErr, errs.Error(errs.ErrPacketFilterDeleteRule, err))
			continue
		}

		rules, err := n.nft.GetRules(chain.Table, chain)
		if err != nil {
			n.logger.Warn("failed to get chain rules for removal", "table", chain.Table.Name, "chain", chain.Name)
			mErr = multierror.Append(mErr, fmt.Errorf("%w - cannot list rules: %w", errs.ErrPacketFilterDeleteRule, err))
			continue
		}

		for _, rule := range rules {
			content, ok := userdata.GetString(rule.UserData, userdata.TypeComment)
			if !ok {
				continue
			}
			if !strings.Contains(content, teardown.Identifier) {
				continue
			}

			if err := n.nft.DelRule(rule); err != nil {
				n.logger.Warn("failed to delete rule", "table", chain.Table.Name, "chain", chain.Name)
				mErr = multierror.Append(mErr, errs.Error(errs.ErrPacketFilterDeleteRule, err))
			}

			// A rule was deleted so a save is required.
			saveFn = n.nft.Flush
		}
	}

	// If rules were deleted, persist the changes.
	if saveFn != nil {
		if err := saveFn(); err != nil {
			n.logger.Warn("failed to persist rule deletion changes", "error", err)
			mErr = multierror.Append(mErr, errs.Error(errs.ErrPacketFilterDeleteRule, err))
		}
	}

	// Reset the save function holder.
	saveFn = nil

	for _, c := range teardown.Chains {
		c = normalizedChain(c)
		chain, err := n.getChain(c)
		if err != nil {
			// If the error is due to the chain not existing, ignore.
			if errors.Is(err, errs.ErrNotFound) {
				continue
			}

			n.logger.Warn("failed to delete chain", "table", n.table, "chain", chainName(c))
			mErr = multierror.Append(mErr, errs.Error(errs.ErrPacketFilterDeleteChain, err))
			continue
		}

		n.nft.DelChain(chain)

		// A chain was deleted so a save is required.
		saveFn = n.nft.Flush
	}

	// If chains were deleted, persist the changes.
	if saveFn != nil {
		if err := saveFn(); err != nil {
			n.logger.Warn("failed to persist chain deletion changes", "error", err)
			mErr = multierror.Append(mErr, errs.Error(errs.ErrPacketFilterDeleteChain, err))
		}
	}

	return mErr.ErrorOrNil()
}

// Destroy destroys the table containing all the nomad virt
// filtering rules.
func (n *nft) Destroy() error {
	t, err := n.getTable()
	if err != nil {
		return err
	}
	n.nft.DelTable(t)
	return n.nft.Flush()
}

// getTable returns the isolated nftables table for chains rules.
func (n *nft) getTable() (*nftables.Table, error) {
	t, err := n.nft.ListTableOfFamily(n.table, nftables.TableFamilyINet)
	if err != nil {
		n.logger.Debug("failed to get table", "name", n.table, "error", err)
		// the error messages are generally cryptic and terse, so note that
		// it was encountered while attempting to load the table.
		return nil, fmt.Errorf("failed to load table: %w", netlinkErr(err))
	}

	return t, nil
}

// getChain loads the requested chain for the isolated table.
func (n *nft) getChain(chain shared.Chain) (*nftables.Chain, error) {
	t, err := n.getTable()
	if err != nil {
		return nil, err
	}

	c, err := n.nft.ListChain(t, chainName(chain))
	if err != nil {
		n.logger.Debug("failed to get chain", "chain", chainName(chain), "error", err)
		// the error messages are generally cryptic and terse, so note that
		// it was encountered while attempting to load the chain.
		return nil, fmt.Errorf("failed to load chain: %w", netlinkErr(err))
	}

	return c, nil
}

// netlinkErr wraps an error that was returned by netlink if appropriate.
func netlinkErr(err error) error {
	if strings.Contains(err.Error(), "no such file or directory") {
		return fmt.Errorf("%w (%w)", errs.ErrNotFound, err)
	}

	return err
}

// normalizedChain returns a chain with the table and
// chain names in lowercase.
func normalizedChain(c shared.Chain) shared.Chain {
	return shared.Chain{
		Table: strings.ToLower(c.Table),
		Chain: strings.ToLower(c.Chain),
	}
}

// chainName constructs the chain name by combining the requested
// table name and chain name.
func chainName(c shared.Chain) string {
	return strings.ToLower(fmt.Sprintf("%s.%s", c.Table, c.Chain))
}

// needsHook returns if the chain is considered a base chain and
// should have a hook configured.
func needsHook(c shared.Chain) bool {
	return slices.Contains(slices.Collect(maps.Keys(baseTypes)), c.Table) &&
		slices.Contains(slices.Collect(maps.Keys(baseHooks)), c.Chain)
}

// generateRule converts a rule to an nftables specific rule.
func (n *nft) generateRule(r shared.Rule) (*nftables.Rule, error) {
	c, err := n.getChain(normalizedChain(r.Chain))
	if err != nil {
		n.logger.Warn("failed to get chain for rule generation", "error", err)
		return nil, err
	}
	rule := &nftables.Rule{
		Table:    c.Table,
		Chain:    c,
		Position: uint64(r.Position),
	}

	exprs := []expr.Any{}

	// Always specify the protocol of the rule.

	// Usage of the initial meta match is to prevent loading garbage data
	// as described here:
	// https://wiki.nftables.org/wiki-nftables/index.php/Ruleset_debug/VM_code_analysis#VM_bytecode_in_action
	exprs = append(exprs,
		&expr.Meta{
			Key:      expr.MetaKeyL4PROTO,
			Register: 1,
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     dataProtocol(r.Spec.Protocol),
		},
	)

	//	nft --debug=netlink 'add rule ip t c ip protocol tcp'
	//   [ payload load 1b @ network header + 9 => reg 1 ]
	//   [ cmp eq reg 1 0x00000006 ]
	exprs = append(exprs,
		&expr.Payload{
			Base:         expr.PayloadBaseNetworkHeader,
			Offset:       9,
			Len:          1,
			DestRegister: 1,
		},
		&expr.Cmp{
			Op:       expr.CmpOpEq,
			Register: 1,
			Data:     dataProtocol(r.Spec.Protocol),
		},
	)

	if r.Spec.OutInterface != "" {
		// nft --debug=netlink 'add rule ip t c oifname "eth1"'
		//   [ meta load oifname => reg 1 ]
		//   [ cmp eq reg 1 0x31687465 0x00000000 0x00000000 0x00000000 ]
		exprs = append(exprs,
			&expr.Meta{
				Key:      expr.MetaKeyOIFNAME,
				Register: 1,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     dataIfname(r.Spec.OutInterface),
			},
		)
	}
	if r.Spec.InInterface != "" {
		// 	nft --debug=netlink 'add rule ip t c iifname "eth1"'
		//   [ meta load iifname => reg 1 ]
		//   [ cmp eq reg 1 0x31687465 0x00000000 0x00000000 0x00000000 ]
		exprs = append(exprs,
			&expr.Meta{
				Key:      expr.MetaKeyIIFNAME,
				Register: 1,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     dataIfname(r.Spec.InInterface),
			},
		)
	}
	if r.Spec.Source != "" {
		//	nft --debug=netlink 'add rule ip t c ip saddr 127.0.3.3'
		//   [ payload load 4b @ network header + 12 => reg 1 ]
		//   [ cmp eq reg 1 0x0303007f ]
		addr, err := netip.ParseAddr(r.Spec.Source)
		if err != nil {
			return nil, err
		}

		exprs = append(exprs,
			&expr.Payload{
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       12,
				Len:          4,
				DestRegister: 1,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     addr.AsSlice(),
			},
		)
	}
	if r.Spec.Destination != "" {
		//	nft --debug=netlink 'add rule ip t c ip daddr 127.0.3.3'
		//   [ payload load 4b @ network header + 16 => reg 1 ]
		//   [ cmp eq reg 1 0x0303007f ]
		addr, err := netip.ParseAddr(r.Spec.Destination)
		if err != nil {
			return nil, err
		}

		exprs = append(exprs,
			&expr.Payload{
				Base:         expr.PayloadBaseNetworkHeader,
				Offset:       16,
				Len:          4,
				DestRegister: 1,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     addr.AsSlice(),
			},
		)
	}

	if r.Spec.DestinationPort > 0 {
		// 	nft --debug=netlink 'add rule ip t c tcp dport 20'
		//   [ meta load l4proto => reg 1 ]
		//   [ cmp eq reg 1 0x00000006 ]
		//   [ payload load 2b @ transport header + 2 => reg 1 ]
		//   [ cmp eq reg 1 0x00001400 ]
		exprs = append(exprs,
			&expr.Meta{
				Key:      expr.MetaKeyL4PROTO,
				Register: 1,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     dataProtocol(r.Spec.Protocol),
			},
			&expr.Payload{
				Base:         expr.PayloadBaseTransportHeader,
				Offset:       2,
				Len:          2,
				DestRegister: 1,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     binaryutil.BigEndian.PutUint16(uint16(r.Spec.DestinationPort)),
			},
		)
	}
	if r.Spec.SourceType != "" {
		// 	nft --debug=netlink 'add rule ip t c fib saddr type local'
		//   [ fib saddr type => reg 1 ]
		//   [ cmp eq reg 1 0x00000002 ]
		exprs = append(exprs,
			&expr.Fib{
				FlagSADDR:      true,
				ResultADDRTYPE: true,
				Register:       1,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     dataAddrType(r.Spec.SourceType),
			},
		)
	}
	if r.Spec.DestinationType != "" {
		//  nft --debug=netlink 'add rule ip nat OUTPUT fib daddr type unicast'
		//   [ fib daddr type => reg 1 ]
		//   [ cmp eq reg 1 0x00000001 ]
		exprs = append(exprs,
			&expr.Fib{
				FlagDADDR:      true,
				ResultADDRTYPE: true,
				Register:       1,
			},
			&expr.Cmp{
				Op:       expr.CmpOpEq,
				Register: 1,
				Data:     dataAddrType(r.Spec.DestinationType),
			},
		)
	}
	if r.Spec.State != "" {
		// 	nft --debug=netlink 'add rule inet t c ct state new'
		//   [ ct load state => reg 1 ]
		//   [ bitwise reg 1 = ( reg 1 & 0x00000008 ) ^ 0x00000000 ]
		//   [ cmp neq reg 1 0x00000000 ]
		exprs = append(exprs,
			&expr.Ct{
				Key:      expr.CtKeySTATE,
				Register: 1,
			},
			&expr.Bitwise{
				SourceRegister: 1,
				DestRegister:   1,
				Len:            4,
				Mask:           binaryutil.NativeEndian.PutUint32(stateBit(r.Spec.State)),
				Xor:            binaryutil.NativeEndian.PutUint32(0),
			},
			&expr.Cmp{
				Op:       expr.CmpOpNeq,
				Register: 1,
				Data:     []byte{0, 0, 0, 0},
			},
		)
	}

	if r.Spec.ToDestination != "" {
		// 	nft --debug=netlink 'add rule inet t c ip protocol tcp dnat to 127.4.4.3:8888'
		//   ... <clipped>
		//   [ immediate reg 1 0x0304047f ]
		//   [ immediate reg 2 0x0000b822 ]
		//   [ nat dnat ip addr_min reg 1 proto_min reg 2 flags 0x2 ]
		addr, err := netip.ParseAddrPort(r.Spec.ToDestination)
		if err != nil {
			return nil, err
		}

		exprs = append(exprs,
			&expr.Immediate{
				Register: 1,
				Data:     addr.Addr().AsSlice(),
			},
			&expr.Immediate{
				Register: 2,
				Data:     binaryutil.BigEndian.PutUint16(addr.Port()),
			},
			&expr.NAT{
				Type:        expr.NATTypeDestNAT,
				Family:      unix.NFPROTO_IPV4,
				RegAddrMin:  1,
				RegProtoMin: 2,
				Specified:   true,
			},
		)
	}

	if r.Spec.Jump != "" {
		// Some jumps are just implying a verdict, so
		// adjudicate if we need to.
		switch strings.ToLower(r.Spec.Jump) {
		case "accept":
			exprs = append(exprs, &expr.Verdict{Kind: expr.VerdictAccept})
		case "reject":
			exprs = append(exprs, &expr.Verdict{Kind: expr.VerdictDrop})
		case "masquerade":
			exprs = append(exprs, &expr.Masq{})
		case "dnat", "snat":
			// these are ignored
		default:
			// Do an actual jump. Be sure to normalize
			// the chain as it's being constructed from
			// the jump value.
			jumpChain := normalizedChain(
				shared.Chain{
					Table: r.Chain.Table,
					Chain: r.Spec.Jump,
				},
			)

			exprs = append(exprs,
				&expr.Verdict{
					Kind:  expr.VerdictJump,
					Chain: chainName(jumpChain),
				},
			)
		}

	}

	// Set the expressions into the rule.
	rule.Exprs = exprs

	// If an identifier is specified set it on the rule.
	if r.Identifier != "" {
		rule.UserData = userdata.AppendString(
			rule.UserData, userdata.TypeComment, r.Identifier)
	}

	return rule, nil
}

// dataIfname returns the interface name in the format required
// by nftables for comparison.
func dataIfname(name string) []byte {
	data := make([]byte, 16)
	copy(data, []byte(name+nullByte))

	return data
}

// dataProtocol returns the protocol value used in comparison
// expressions.
func dataProtocol(name string) []byte {
	switch name {
	case "tcp":
		return []byte{unix.IPPROTO_TCP}
	case "udp":
		return []byte{unix.IPPROTO_UDP}
	default:
		return []byte{unix.IPPROTO_TCP}
	}
}

// dataAddrType returns the addrtype value used in comparison
// expressions.
func dataAddrType(name string) []byte {
	switch strings.ToLower(name) {
	case "unicast":
		return []byte{unix.RTN_UNICAST}
	case "local":
		return []byte{unix.RTN_LOCAL}
	case "broadcast":
		return []byte{unix.RTN_BROADCAST}
	case "anycast":
		return []byte{unix.RTN_ANYCAST}
	case "multicast":
		return []byte{unix.RTN_MULTICAST}
	case "blackhole":
		return []byte{unix.RTN_BLACKHOLE}
	case "unreachable":
		return []byte{unix.RTN_UNREACHABLE}
	case "prohibit":
		return []byte{unix.RTN_PROHIBIT}
	default:
		return []byte{unix.RTN_UNSPEC}
	}
}

// stateBit returns the value with requested state bit set.
func stateBit(name string) uint32 {
	switch strings.ToLower(name) {
	case "invalid":
		return expr.CtStateBitINVALID
	case "established":
		return expr.CtStateBitESTABLISHED
	case "related":
		return expr.CtStateBitRELATED
	case "new":
		return expr.CtStateBitNEW
	case "untracked":
		return expr.CtStateBitUNTRACKED
	default:
		return 0
	}
}

// chainPriority returns the priority value for a chain.
func chainPriority(c shared.Chain) *nftables.ChainPriority {
	if c.Table == "nat" {
		switch c.Chain {
		case "prerouting":
			return nftables.ChainPriorityNATDest
		case "postrouting":
			return nftables.ChainPriorityNATSource
		}
	}

	return nftables.ChainPriorityFilter
}

// equivalentRules compares two rules to determine if they
// are equivalent.
// NOTE: This is an equivalency check, not an equality check. If
// both rules defined the same expressions within the same chain
// then they are considered equivalent.
func equivalentRules(lhs, rhs *nftables.Rule) bool {
	// Check for missing or mismatched table/chain names.
	if lhs.Table == nil || rhs.Table == nil || lhs.Table.Name != rhs.Table.Name {
		return false
	}
	if lhs.Chain == nil || rhs.Chain == nil || lhs.Chain.Name != rhs.Chain.Name {
		return false
	}

	// If the number of expressions aren't the same they
	// cannot be equivalent.
	if len(lhs.Exprs) != len(rhs.Exprs) {
		return false
	}

	// Compare all the expressions.
	for i := range len(lhs.Exprs) {
		if !reflect.DeepEqual(getElement(lhs.Exprs[i]), getElement(rhs.Exprs[i])) {
			return false
		}
	}

	// Rules are equivalent.
	return true
}

// equalRules compares two rules to determine if they are equal.
// NOTE: This equality check ignores the Position values since it is only
// used when placing a rule, referencing a handle.
func equalRules(rhs, lhs *nftables.Rule) bool {
	if !equivalentRules(rhs, lhs) {
		return false
	}

	return rhs.Flags == lhs.Flags &&
		bytes.Equal(rhs.UserData, lhs.UserData)
}

// getElement returns the current value of v.
func getElement(v any) any {
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Pointer {
		return val.Elem().Interface()
	}
	return val.Interface()
}

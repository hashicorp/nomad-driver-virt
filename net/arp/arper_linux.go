// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

//go:build linux

package arp

import (
	"bytes"
	"context"
	"net"
	"slices"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-set/v3"
	"github.com/jsimonetti/rtnetlink/v2"
	"golang.org/x/sys/unix"
)

const (
	// defaultPollingInterval is the polling interval.
	defaultPollingInterval = 1 * time.Second
)

var (
	// loaderMu is used to synchronize singleton creation.
	loaderMu sync.Mutex

	// singleton is the single instance of the ARP interface.
	singleton *arper
)

// IsAvailable returns if ARP functionality is available.
func IsAvailable() bool {
	return true
}

// New returns the ARP interface, creating the singleton if
// it does not already exist.
func New() *arper {
	loaderMu.Lock()
	defer loaderMu.Unlock()

	if singleton != nil {
		return singleton
	}

	singleton = &arper{
		ctx:              context.Background(),
		logger:           hclog.Default().Named("arp"),
		pollingInterval:  defaultPollingInterval,
		notifyCh:         make(chan struct{}),
		neighborsFn:      getNeighbors,
		interfaceByIndex: net.InterfaceByIndex,
	}

	return singleton
}

// neighborsFn is the function that returns the list of
// neighbor table entries.
type neighborsFn func() ([]rtnetlink.NeighMessage, error)

// interfaceByIndex returns the network interface by the
// index provided.
type interfaceByIndex func(int) (*net.Interface, error)

// request is an incoming request for address discovery.
type request struct {
	hwaddr net.HardwareAddr // MAC address to match.
	iface  *net.Interface   // interface to match.
	ch     chan net.IP      // channel to send results.
}

// matches returns if entry is a match for the request.
func (r *request) matches(entry *entry) bool {
	return !entry.addr.IsUnspecified() &&
		slices.Equal(entry.hwaddr, r.hwaddr) &&
		r.iface.Name == entry.device
}

// entry is an ARP entry.
type entry struct {
	addr   net.IP           // IP address.
	hwaddr net.HardwareAddr // MAC address.
	device string           // interface name.
}

// arper implements the ARP interface.
type arper struct {
	ctx             context.Context    // controls life of instance. used for testing, background context in actual use.
	activeReqs      uint               // count of currently active requests
	entries         []*entry           // ARP table entries.
	polling         bool               // polling goroutine is running.
	pollingDone     chan struct{}      // channel that is closed when polling goroutine is complete.
	pollingCtx      context.Context    // context used by the polling goroutine.
	pollingCancel   context.CancelFunc // cancel func used to stop the polling goroutine.
	pollingInterval time.Duration      // polling interval.
	notifyCh        chan struct{}      // channel that is closed when updated entries are available.

	logger     hclog.Logger
	notifyChMu sync.Mutex
	requestsMu sync.Mutex
	entriesMu  sync.RWMutex

	// Items below are defined to enable testing.
	neighborsFn      // function to provide neighbor list
	interfaceByIndex // function to provide network interface by index
}

// Discover will poll the neighbor entry table (ARP entries) for a matching
// MAC address and send found IP addresses to the channel until the provided
// context has been canceled.
func (a *arper) Discover(ctx context.Context, iface *net.Interface, hwaddr net.HardwareAddr) (<-chan net.IP, error) {
	return a.addRequest(ctx, &request{
		hwaddr: hwaddr,
		iface:  iface,
		ch:     make(chan net.IP),
	})
}

// SetContext sets a custom context.
func (a *arper) SetContext(ctx context.Context) {
	a.ctx = ctx
}

// SetLogger sets a custom logger.
func (a *arper) SetLogger(logger hclog.Logger) {
	a.logger = logger
}

// addRequest adds a new discovery request.
func (a *arper) addRequest(ctx context.Context, req *request) (<-chan net.IP, error) {
	a.requestsMu.Lock()
	defer a.requestsMu.Unlock()

	// The polling goroutine will only run if there are active
	// requests. If it's not running, start it.
	if !a.polling {
		// If the completeCh is available then wait on it in case a
		// previous polling goroutine is still cleaning up.
		if a.pollingDone != nil {
			select {
			case <-a.pollingDone:
			case <-a.ctx.Done():
				return nil, a.ctx.Err()
			}
		}

		// Set a new completion channel.
		a.pollingDone = make(chan struct{})
		// Create a cancelable context for the polling goroutine.
		a.pollingCtx, a.pollingCancel = context.WithCancel(a.ctx)
		// Flag that we are actively polling.
		a.polling = true
		// Start the polling goroutine.
		go a.poll()
	}

	// Handle the request
	go a.handleRequest(ctx, req)

	// And send the channel back.
	return req.ch, nil
}

// requestStarted increments the active requests count.
func (a *arper) requestStarted() {
	a.requestsMu.Lock()
	defer a.requestsMu.Unlock()

	// Increment the active requests.
	a.activeReqs++
}

// requestComplete decrements the active requests count and stops the
// polling goroutine if there are no remaining active requests.
func (a *arper) requestComplete() {
	a.requestsMu.Lock()
	defer a.requestsMu.Unlock()

	// Decrement the active requests.
	a.activeReqs--

	// If there are no active requests, stop polling.
	if a.activeReqs == 0 && a.pollingCancel != nil {
		// Mark as no longer polling.
		a.polling = false
		// Cancel the polling context to stop the polling goroutine.
		a.pollingCancel()
	}
}

// handleRequest will attempt to find IP addresses matching the
// request until the context (request context or polling context)
// have completed.
func (a *arper) handleRequest(ctx context.Context, req *request) {
	// Increment the active requests count.
	a.requestStarted()

	// Decrement the active requests count on the way out.
	defer a.requestComplete()

	// Close the request channel when done handling the request.
	defer close(req.ch)

	// Remember seen addresses so we only send them once.
	seenAddrs := set.NewTreeSet(bytes.Compare)

	for {
		// Find any matches and send them.
		entries := a.find(req)
		for _, entry := range entries {
			// If we have seen this address already, skip.
			if !seenAddrs.Insert(entry.addr) {
				continue
			}

			select {
			case req.ch <- entry.addr:
			case <-ctx.Done():
				// Request context is done so we can bail.
				return
			case <-a.pollingCtx.Done():
				// polling context (or its parent) is done so it's time to go.
				return
			}
		}

		// Wait for new entries to be available or context
		// to be done.
		select {
		case <-a.notificationCh():
			// New entries available so loop.
		case <-ctx.Done():
			// Request context is done so we can bail.
			return
		case <-a.pollingCtx.Done():
			// polling context (or its parent) is done so it's time to go.
			return
		}
	}
}

// notificationCh returns a channel to wait on for new ARP
// entries to be available.
func (a *arper) notificationCh() <-chan struct{} {
	a.notifyChMu.Lock()
	defer a.notifyChMu.Unlock()

	return a.notifyCh
}

// notifyRequests closes the existing readyCh to broadcast
// notification of new ARP entries and creates a new readyCh
// for requests to wait on.
func (a *arper) notifyRequests() {
	a.notifyChMu.Lock()
	defer a.notifyChMu.Unlock()

	close(a.notifyCh)
	a.notifyCh = make(chan struct{})
}

// poll will load any available ARP entries, notify any
// waiting requests of new entries, and wait the polling
// interval before running again until the polling context
// is canceled.
func (a *arper) poll() {
	defer func() {
		// Ensure the polling context is canceled when we stop polling.
		a.pollingCancel()
		// Clear the current list of ARP entries.
		a.clearEntries()
		// Notify that this poller is complete.
		close(a.pollingDone)
	}()

	for {
		// Load the currently available ARP entries.
		a.loadARPs()

		// Notify of new entries.
		a.notifyRequests()

		select {
		// If the polling context is done then there are no active requests
		// or the parent context is done. Either way, we are done here.
		case <-a.pollingCtx.Done():
			a.logger.Debug("stopping polling due to context completion")
			return
		// Wait for the polling interval before reloading the ARP entries.
		case <-time.After(a.pollingInterval):
		}
	}
}

// clearEntries cleans up clears any existing ARP table entries.
func (a *arper) clearEntries() {
	a.entriesMu.Lock()
	defer a.entriesMu.Unlock()

	// Clear the current entries list and any requests.
	a.entries = make([]*entry, 0)
}

// loadARPs loads all the ARP entries which are currently reachable
// and are not stale.
func (a *arper) loadARPs() {
	a.logger.Trace("loading available ARP entries")

	a.entriesMu.Lock()
	defer a.entriesMu.Unlock()

	// Start with fetching the neighbors list.
	neighbors, err := a.neighborsFn()
	if err != nil {
		a.logger.Error("failed to get neighbor list", "error", err)
		return
	}

	// Collect all the current neighbor entries.
	results := []*entry{}
	for _, n := range neighbors {
		iface, err := a.interfaceByIndex(int(n.Index))
		if err != nil {
			a.logger.Warn("invalid interface index for neighbor, ignoring", "error", err, "index", n.Index, "neighbor", n)
			continue
		}

		entry := entry{
			device: iface.Name,
			hwaddr: n.Attributes.LLAddress,
			addr:   n.Attributes.Address,
		}

		// If the entry is not reachable, ignore it.
		if n.State&unix.NUD_REACHABLE != unix.NUD_REACHABLE {
			a.logger.Trace("ignoring unreachable entry", "entry", entry)
			continue
		}

		// If the entry is stale, ignore it.
		if n.State&unix.NUD_STALE == unix.NUD_STALE {
			a.logger.Trace("ignore stale entry", "entry", entry)
			continue
		}

		results = append(results, &entry)
	}

	// Now finish up by updating the entries.
	a.entries = results

	a.logger.Trace("completed loading ARP entries", "count", len(a.entries))
}

// find attempts to find any ARP entry matches for the provided
// request and returns all found matches.
func (a *arper) find(req *request) []*entry {
	a.entriesMu.RLock()
	defer a.entriesMu.RUnlock()

	matches := make([]*entry, 0)

	for _, entry := range a.entries {
		if req.matches(entry) {
			matches = append(matches, entry)
		}
	}

	return matches
}

// getNeighbors returns the neighbor list from netlink.
func getNeighbors() ([]rtnetlink.NeighMessage, error) {
	// Establish a netlink connection to fetch the neighbors.
	conn, err := rtnetlink.Dial(nil)
	if err != nil {
		return nil, err
	}

	return conn.Neigh.List()
}

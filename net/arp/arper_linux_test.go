// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package arp

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/go-hclog"
	"github.com/jsimonetti/rtnetlink/v2"
	"github.com/shoenig/test/must"
	"golang.org/x/sys/unix"
)

const (
	testReqMAC    = "10:66:6a:f2:04:dd"
	testReqIP     = "10.162.122.209"
	testReqDevice = "eth0"

	testPollingInterval = 50 * time.Millisecond
)

// setEntries sets a custom list of entries.
func (a *arper) setEntries(entries []*entry) {
	a.entriesMu.Lock()
	defer a.entriesMu.Unlock()

	a.entries = entries
}

// setNeighbors sets a custom neighbors function.
func (a *arper) setNeighbors(fn neighborsFn) {
	a.entriesMu.Lock()
	defer a.entriesMu.Unlock()

	a.neighborsFn = fn
}

// isPolling returns currently polling
func (a *arper) isPolling() bool {
	a.requestsMu.Lock()
	defer a.requestsMu.Unlock()

	return a.polling
}

// activeRequests returns the current number of active requests.
func (a *arper) activeRequests() uint {
	a.requestsMu.Lock()
	defer a.requestsMu.Unlock()

	return a.activeReqs
}

func mkTestReq(t *testing.T) *request {
	mac, err := net.ParseMAC(testReqMAC)
	must.NoError(t, err)
	return &request{
		hwaddr: mac,
		iface:  &net.Interface{Name: testReqDevice},
		ch:     make(chan net.IP, 1),
	}
}

func mkTestEntry(t *testing.T) *entry {
	mac, err := net.ParseMAC(testReqMAC)
	must.NoError(t, err)
	return &entry{
		hwaddr: mac,
		addr:   net.ParseIP(testReqIP),
		device: testReqDevice,
	}
}

func mkTestNeighMessage(t *testing.T) rtnetlink.NeighMessage {
	mac, err := net.ParseMAC(testReqMAC)
	must.NoError(t, err)

	return rtnetlink.NeighMessage{
		State: unix.NUD_REACHABLE,
		Attributes: &rtnetlink.NeighAttributes{
			Address:   net.ParseIP(testReqIP),
			LLAddress: mac,
		},
	}
}

func testArper(t *testing.T) *arper {
	ctx, cancel := context.WithCancel(t.Context())
	return &arper{
		ctx:             t.Context(),
		logger:          hclog.NewNullLogger(),
		notifyCh:        make(chan struct{}),
		pollingCtx:      ctx,
		pollingCancel:   cancel,
		pollingInterval: testPollingInterval,
		neighborsFn:     func() (_ []rtnetlink.NeighMessage, _ error) { return },
		interfaceByIndex: func(int) (*net.Interface, error) {
			return &net.Interface{Name: "eth0"}, nil
		},
	}
}

func Test_arper_Discover(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		a := testArper(t)
		a.setNeighbors(func() ([]rtnetlink.NeighMessage, error) {
			return []rtnetlink.NeighMessage{mkTestNeighMessage(t)}, nil
		})
		ctx, cancel := context.WithCancel(t.Context())
		stubReq := mkTestReq(t)

		ch, err := a.Discover(ctx, stubReq.iface, stubReq.hwaddr)
		must.NoError(t, err)

		// Result should be received.
		select {
		case result := <-ch:
			must.Eq(t, testReqIP, result.String())
		case <-time.After(100 * time.Millisecond):
			t.Fatal("expected result, received none")
		}

		must.One(t, a.activeRequests(), must.Sprint("expected on active request"))

		// Polling should be active.
		must.True(t, a.isPolling(), must.Sprint("expected polling to be active"))

		// Cancel the request context
		cancel()

		// Wait for the request to complete.
		select {
		case <-ch:
		case <-time.After(50 * time.Millisecond):
			t.Fatal("expected request channel to be closed")
		}

		// With no active requests, the poller should stop.
		select {
		case <-a.pollingDone:
		case <-time.After(50 * time.Millisecond):
			t.Fatal("expected polling to be stopped")
		}

		must.False(t, a.isPolling(), must.Sprint("expected polling to be inactive"))
		must.Zero(t, a.activeRequests(), must.Sprint("expected no active requests"))
	})

	t.Run("ok multiple requests", func(t *testing.T) {
		count := 8
		reqs := make([]*request, count)
		messages := make([]rtnetlink.NeighMessage, count)
		for i := range count {
			r := mkTestReq(t)
			mac, _ := net.ParseMAC(strings.ReplaceAll(testReqMAC, ":dd", fmt.Sprintf(":0%d", i)))
			r.hwaddr = mac
			reqs[i] = r

			m := mkTestNeighMessage(t)
			m.Attributes.LLAddress = mac
			m.Attributes.Address = net.ParseIP(strings.ReplaceAll(testReqIP, "209", fmt.Sprintf("%d", i)))
			messages[i] = m
		}

		ctx, cancel := context.WithCancel(t.Context())
		a := testArper(t)
		a.ctx = ctx
		reqChs := make([]<-chan net.IP, count)
		// Load in all the requests.
		for i, r := range reqs {
			ch, err := a.Discover(t.Context(), r.iface, r.hwaddr)
			must.NoError(t, err)
			reqChs[i] = ch
		}

		// Polling should be active now.
		must.True(t, a.isPolling(), must.Sprint("expected polling to be active"))

		// Check that all requests are waiting.
		for _, ch := range reqChs {
			select {
			case result := <-ch:
				t.Fatalf("received unexpected result: %v", result)
			case <-time.After(10 * time.Millisecond):
			}
		}

		// Add half the messages to the neighbor list.
		a.setNeighbors(func() ([]rtnetlink.NeighMessage, error) {
			return messages[0 : count/2], nil
		})

		// Wait for results on first half of requests.
		for i := range count / 2 {
			select {
			case result := <-reqChs[i]:
				must.Eq(t, messages[i].Attributes.Address, result)
			case <-time.After(100 * time.Millisecond):
				t.Fatal("failed to receive expected result")
			}
		}

		// All requests should still be waiting.
		for i := range count {
			select {
			case result := <-reqChs[i]:
				t.Fatalf("received unexpected result: %v", result)
			case <-time.After(10 * time.Millisecond):
			}
		}

		// Add all messages to neighbor list.
		a.setNeighbors(func() ([]rtnetlink.NeighMessage, error) {
			return messages, nil
		})

		// Wait for results on last half of requests.
		for i := range count - (count / 2) {
			i = (count / 2) + i

			select {
			case result := <-reqChs[i]:
				must.Eq(t, messages[i].Attributes.Address, result)
			case <-time.After(100 * time.Millisecond):
				t.Fatal("failed to receive expected result")
			}
		}

		// All requests should now be waiting.
		for _, ch := range reqChs {
			select {
			case result := <-ch:
				t.Fatalf("received unexpected result: %v", result)
			case <-time.After(10 * time.Millisecond):
			}
		}

		// Cancel the parent context to shut the arper down completely.
		cancel()

		// Wait for it to stop polling.
		select {
		case <-a.pollingDone:
		case <-time.After(100 * time.Millisecond):
			t.Fatal("expected polling to stop")
		}

		// All request channels should now be closed.
		for _, ch := range reqChs {
			select {
			case <-ch:
			case <-time.After(10 * time.Millisecond):
				t.Fatal("request channel should be closed")
			}
		}
	})
}

func Test_arper_handleRequest(t *testing.T) {
	t.Run("no match request context cancel", func(t *testing.T) {
		a := testArper(t)
		req := mkTestReq(t)
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		go a.handleRequest(ctx, req)

		// No address should be sent since no entries are loaded.
		select {
		case <-req.ch:
			t.Fatal("received unexpected result")
		case <-time.After(10 * time.Millisecond):
		}

		// There should be a single active request.
		must.One(t, a.activeRequests(), must.Sprint("expected one active request"))

		// Cancel the request context.
		cancel()

		// Wait on the channel for it to be closed.
		select {
		case <-req.ch:
		case <-time.After(50 * time.Millisecond):
			t.Fatal("request channel did not close")
		}

		// There should be no active requests.
		must.Zero(t, a.activeRequests(), must.Sprint("expected no active requests"))
	})

	t.Run("no match parent context cancel", func(t *testing.T) {
		a := testArper(t)
		req := mkTestReq(t)

		go a.handleRequest(t.Context(), req)

		// No address should be sent since no entries are loaded.
		select {
		case <-req.ch:
			t.Fatal("received unexpected result")
		case <-time.After(10 * time.Millisecond):
		}

		// There should be a single active request.
		must.One(t, a.activeRequests(), must.Sprint("expected one active request"))

		// Cancel the parent context context.
		a.pollingCancel()

		// Wait on the channel for it to be closed.
		select {
		case <-req.ch:
		case <-time.After(50 * time.Millisecond):
			t.Fatal("request channel did not close")
		}

		// There should be no active requests.
		must.Zero(t, a.activeRequests(), must.Sprint("expected no active requests"))
	})

	t.Run("match", func(t *testing.T) {
		a := testArper(t)
		req := mkTestReq(t)
		go a.handleRequest(t.Context(), req)

		// Make sure nothing has been sent.
		select {
		case <-req.ch:
			t.Fatal("received unexpected result")
		case <-time.After(10 * time.Millisecond):
		}

		// Add a matching entry.
		a.setEntries([]*entry{mkTestEntry(t)})

		// Notify of new entries.
		a.notifyRequests()

		// And we should get a match.
		select {
		case result := <-req.ch:
			must.Eq(t, testReqIP, result.String())
		case <-time.After(50 * time.Millisecond):
			t.Fatal("expected result but none received")
		}

		// Notify again so the entries are reprocessed.
		a.notifyRequests()

		// Match was already sent so it should not be
		// received again.
		select {
		case <-req.ch:
			t.Fatal("match was delivered a second time")
		case <-time.After(50 * time.Millisecond):
		}
	})

	t.Run("multiple matches", func(t *testing.T) {
		a := testArper(t)
		req := mkTestReq(t)
		go a.handleRequest(t.Context(), req)

		// Make sure nothing has been sent.
		select {
		case <-req.ch:
			t.Fatal("received unexpected result")
		case <-time.After(10 * time.Millisecond):
		}

		entries := []*entry{
			mkTestEntry(t),
			mkTestEntry(t),
			mkTestEntry(t),
		}
		entries[0].addr = net.ParseIP("10.2.3.2")
		mac, _ := net.ParseMAC("10:77:6a:00:09:cc")
		entries[2].hwaddr = mac

		// Set the entries.
		a.setEntries(entries)

		// Notify of new entries.
		a.notifyRequests()

		// And we should get both matches.
		idx := 0
		var complete bool
		for !complete {
			select {
			case result := <-req.ch:
				must.Less(t, 2, idx, must.Sprint("too many matches received"))
				must.Eq(t, entries[idx].addr, result)
				idx++
			case <-time.After(50 * time.Millisecond):
				complete = true
			}
		}

		must.True(t, complete)
	})
}

func Test_arper_loadARPs(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		a := testArper(t)
		a.loadARPs()

		must.SliceEmpty(t, a.entries)
	})

	t.Run("reachable neighbor", func(t *testing.T) {
		a := testArper(t)
		a.setNeighbors(func() ([]rtnetlink.NeighMessage, error) {
			return []rtnetlink.NeighMessage{mkTestNeighMessage(t)}, nil
		})

		a.loadARPs()
		must.SliceLen(t, 1, a.entries)
		must.SliceContains(t, a.entries, mkTestEntry(t),
			must.Cmp(cmp.AllowUnexported(entry{})))
	})

	t.Run("stale neighbor", func(t *testing.T) {
		a := testArper(t)
		staleNeighbor := mkTestNeighMessage(t)
		staleNeighbor.Attributes.Address = net.ParseIP("10.0.0.2")
		staleNeighbor.State |= unix.NUD_STALE
		a.setNeighbors(func() ([]rtnetlink.NeighMessage, error) {
			return []rtnetlink.NeighMessage{
				mkTestNeighMessage(t),
				staleNeighbor,
			}, nil
		})

		a.loadARPs()
		must.SliceLen(t, 1, a.entries)
		must.SliceContains(t, a.entries, mkTestEntry(t),
			must.Cmp(cmp.AllowUnexported(entry{})))
	})
}

func Test_arper_find(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		a := testArper(t)
		must.SliceEmpty(t, a.find(mkTestReq(t)),
			must.Sprint("expected no matches"))
	})

	t.Run("match", func(t *testing.T) {
		a := testArper(t)
		testEntry := mkTestEntry(t)
		a.setEntries([]*entry{testEntry})

		result := a.find(mkTestReq(t))
		must.Eq(t, []*entry{testEntry}, result)
	})

	t.Run("multiple matches", func(t *testing.T) {
		a := testArper(t)
		firstMatch := mkTestEntry(t)
		secondMatch := mkTestEntry(t)
		secondMatch.addr = net.ParseIP("10.162.122.111")
		a.setEntries([]*entry{
			firstMatch,
			secondMatch,
		})

		result := a.find(mkTestReq(t))
		must.Eq(t, []*entry{firstMatch, secondMatch}, result)
	})
}

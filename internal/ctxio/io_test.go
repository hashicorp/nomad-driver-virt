// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package ctxio

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/shoenig/test/must"
)

func TestReader(t *testing.T) {
	r, err := os.Open("/dev/urandom")
	must.NoError(t, err)
	defer r.Close()
	w, err := os.Open("/dev/null")
	must.NoError(t, err)
	defer w.Close()

	ctx, cancel := context.WithCancel(t.Context())
	completeCh := make(chan struct{}, 1)

	go func() {
		io.Copy(w, NewReader(ctx, r))
		completeCh <- struct{}{}
	}()

	// Make sure the copy starts
	<-time.After(5 * time.Millisecond)
	// Cancel the context
	cancel()

	// Wait for completion.
	select {
	case <-completeCh:
	case <-time.After(10 * time.Millisecond):
		t.Fatal("reader was not interrupted")
	}
}

func TestReaderAt(t *testing.T) {
	r, err := os.Open("/dev/urandom")
	must.NoError(t, err)
	defer r.Close()
	w, err := os.Open("/dev/null")
	must.NoError(t, err)
	defer w.Close()

	ctx, cancel := context.WithCancel(t.Context())
	completeCh := make(chan struct{}, 1)

	go func() {
		io.Copy(w, NewReaderFrom(ctx, r))
		completeCh <- struct{}{}
	}()

	// Make sure the copy starts
	<-time.After(5 * time.Millisecond)
	// Cancel the context
	cancel()

	// Wait for completion.
	select {
	case <-completeCh:
	case <-time.After(10 * time.Millisecond):
		t.Fatal("reader was not interrupted")
	}
}

func TestWriter(t *testing.T) {
	r, err := os.Open("/dev/urandom")
	must.NoError(t, err)
	defer r.Close()
	w, err := os.Open("/dev/null")
	must.NoError(t, err)
	defer w.Close()

	ctx, cancel := context.WithCancel(t.Context())
	completeCh := make(chan struct{}, 1)

	go func() {
		io.Copy(NewWriter(ctx, w), r)
		completeCh <- struct{}{}
	}()

	// Make sure the copy starts
	<-time.After(5 * time.Millisecond)
	// Cancel the context
	cancel()

	// Wait for completion.
	select {
	case <-completeCh:
	case <-time.After(10 * time.Millisecond):
		t.Fatal("writer was not interrupted")
	}
}

func TestWriterAt(t *testing.T) {
	r, err := os.Open("/dev/urandom")
	must.NoError(t, err)
	defer r.Close()
	w, err := os.Open("/dev/null")
	must.NoError(t, err)
	defer w.Close()

	ctx, cancel := context.WithCancel(t.Context())
	completeCh := make(chan struct{}, 1)

	go func() {
		io.Copy(NewWriterTo(ctx, w), r)
		completeCh <- struct{}{}
	}()

	// Make sure the copy starts
	<-time.After(5 * time.Millisecond)
	// Cancel the context
	cancel()

	// Wait for completion.
	select {
	case <-completeCh:
	case <-time.After(10 * time.Millisecond):
		t.Fatal("writer was not interrupted")
	}
}

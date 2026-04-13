// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package shims

import (
	"context"
	"fmt"

	"libvirt.org/go/libvirt"
)

const libvirtNoFlags = 0

// Stream is the shim interface that wraps the libvirt Stream.
type Stream interface {
	// Abort requests data transfer be cancelled abnormally.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-stream.html#virStreamAbort
	Abort() error

	// Finish indicates no further data to be transmitted on the stream.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-stream.html#virStreamFinish
	Finish() error

	// Free frees the resources associated to this instance.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-stream.html#virStreamFree
	Free() error

	// Read reads a series of bytes from the stream.
	// NOTE: Name is modified from Recv -> Read for interface support.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-stream.html#virStreamRecv
	Read([]byte) (int, error)

	// Write writes a series of bytes to the stream.
	// NOTE: Name is modified from Send -> Write for interface support.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-stream.html#virStreamSend
	Write([]byte) (int, error)

	// RawStream will return the raw libvirt.Stream pointer if possible.
	// NOTE: THis is a custom function on the shim.
	RawStream() (*libvirt.Stream, error)

	// Sparse returns if the stream supports sparse uploads.
	// NOTE: This is a custom function on the shim.
	Sparse() bool
}

// WrapStream wraps a libvirt Stream in the shim.
func WrapStream(stream streamImpl, ctx context.Context, sparseSupport bool) *libvirtStream {
	return &libvirtStream{
		s:             stream,
		ctx:           ctx,
		sparseSupport: sparseSupport,
	}
}

// streamImpl is an internal interface to represent a *libvirt.Stream
// but allows for mocking.
type streamImpl interface {
	Abort() error
	Finish() error
	Free() error
	Recv(p []byte) (int, error)
	Send(p []byte) (int, error)
	SendHole(len int64, flags uint32) error
}

type libvirtStream struct {
	s streamImpl

	ctx           context.Context
	sparseSupport bool // flags if sparse uploads are supported
}

// Abort requests data transfer be cancelled abnormally.
func (l *libvirtStream) Abort() error {
	return l.s.Abort()
}

// Finish indicates no further data to be transmitted on the stream.
func (l *libvirtStream) Finish() error {
	return l.s.Finish()
}

// Free frees the resources associated to this instance.
func (l *libvirtStream) Free() error {
	return l.s.Free()
}

// Read reads a series of bytes from the stream.
func (l *libvirtStream) Read(data []byte) (int, error) {
	return l.s.Recv(data)
}

// Write writes a series of bytes to the stream.
func (l *libvirtStream) Write(data []byte) (int, error) {
	return l.s.Send(data)
}

// RawStream will return the raw libvirt.Stream pointer if possible.
func (l *libvirtStream) RawStream() (*libvirt.Stream, error) {
	s, ok := l.s.(*libvirt.Stream)
	if !ok {
		return nil, fmt.Errorf("invalid backing stream type, have: %T", l.s)
	}
	return s, nil
}

// Sparse returns if the stream supports sparse uploads.
func (l *libvirtStream) Sparse() bool {
	return l.sparseSupport
}

var _ Stream = (*libvirtStream)(nil)

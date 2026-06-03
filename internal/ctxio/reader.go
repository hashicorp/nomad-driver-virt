// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package ctxio

import (
	"context"
	"io"
)

type ReaderFrom interface {
	io.Reader
	io.ReaderFrom
}

type reader struct {
	ctx context.Context
	src io.Reader
}

// Read reads data unless the context is complete.
func (r *reader) Read(p []byte) (int, error) {
	if r.ctx.Err() != nil {
		return 0, r.ctx.Err()
	}

	return r.src.Read(p)
}

type readerFrom struct {
	*reader

	ctx context.Context
	src io.ReaderFrom
}

// ReadFrom wraps the provided io.Reader into a context aware Reader.
func (r *readerFrom) ReadFrom(rd io.Reader) (int64, error) {
	if _, ok := rd.(*reader); !ok {
		rd = NewReader(r.ctx, rd)
	}
	return r.src.ReadFrom(rd)
}

// Copyright IBM Corp. 2024, 2025
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

func (r *readerFrom) ReadFrom(rd io.Reader) (int64, error) {
	if r.ctx.Err() != nil {
		return 0, r.ctx.Err()
	}

	return r.src.ReadFrom(rd)
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package ctxio

import (
	"context"
	"io"
)

type ReaderAt interface {
	io.Reader
	io.ReaderAt
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

type readerAt struct {
	*reader

	ctx context.Context
	src io.ReaderAt
}

func (r *readerAt) ReadAt(p []byte, off int64) (int, error) {
	if r.ctx.Err() != nil {
		return 0, r.ctx.Err()
	}

	return r.src.ReadAt(p, off)
}

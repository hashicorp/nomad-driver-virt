// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package ctxio

import (
	"context"
	"io"
)

type WriterAt interface {
	io.Writer
	io.WriterAt
}

type writer struct {
	ctx context.Context
	dst io.Writer
}

func (w *writer) Write(p []byte) (int, error) {
	if w.ctx.Err() != nil {
		return 0, w.ctx.Err()
	}

	return w.dst.Write(p)
}

type writerAt struct {
	*writer

	ctx context.Context
	dst io.WriterAt
}

func (w *writerAt) WriteAt(p []byte, off int64) (int, error) {
	if w.ctx.Err() != nil {
		return 0, w.ctx.Err()
	}

	return w.dst.WriteAt(p, off)
}

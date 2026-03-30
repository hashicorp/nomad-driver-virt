// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package ctxio

import (
	"context"
	"io"
)

type WriterTo interface {
	io.Writer
	io.WriterTo
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

type writerTo struct {
	*writer

	ctx context.Context
	dst io.WriterTo
}

func (w *writerTo) WriteTo(wrt io.Writer) (int64, error) {
	if w.ctx.Err() != nil {
		return 0, w.ctx.Err()
	}

	return w.dst.WriteTo(wrt)
}

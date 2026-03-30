// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

// ctxio provides context aware io wrappers.
//
// NewReader, for example, returns an io.Reader that will error on Read()
// if the initial context is done. However, canceling the context *during*
// a Read() will not interrupt the ongoing read.
package ctxio

import (
	"context"
	"io"
)

// NewReader returns a context aware io.Reader.
func NewReader(ctx context.Context, r io.Reader) *reader {
	return &reader{
		ctx: ctx,
		src: r,
	}
}

// NewReaderFrom returns a context aware io.ReaderFrom.
func NewReaderFrom(ctx context.Context, r ReaderFrom) *readerFrom {
	return &readerFrom{
		reader: &reader{
			ctx: ctx,
			src: r,
		},
		ctx: ctx,
		src: r,
	}
}

// NewWriter returns a context aware io.Writer.
func NewWriter(ctx context.Context, w io.Writer) *writer {
	return &writer{
		ctx: ctx,
		dst: w,
	}
}

// NewWriterTo returns a context aware io.WriterTo.
func NewWriterTo(ctx context.Context, w WriterTo) *writerTo {
	return &writerTo{
		writer: &writer{
			ctx: ctx,
			dst: w,
		},
		ctx: ctx,
		dst: w,
	}
}

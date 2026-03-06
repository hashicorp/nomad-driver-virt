// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

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

// NewReaderAt returns a context aware io.ReaderAt.
func NewReaderAt(ctx context.Context, r ReaderAt) *readerAt {
	return &readerAt{
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

// NewWriterAt returns a context aware io.WriterAt.
func NewWriterAt(ctx context.Context, w WriterAt) *writerAt {
	return &writerAt{
		writer: &writer{
			ctx: ctx,
			dst: w,
		},
		ctx: ctx,
		dst: w,
	}
}

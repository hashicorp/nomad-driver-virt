// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"errors"
	"io"

	"github.com/ceph/go-ceph/rbd"
	"github.com/hashicorp/nomad-driver-virt/internal/ctxio"
	"github.com/hashicorp/nomad-driver-virt/internal/holes"
)

const (
	// Size of the buffer for copying (same as io.Copy).
	BUFFER_SIZE = 32768
)

// SparseWriter returns an io.Writer that implements io.ReaderFrom on an
// rbd.Image which supports writing a sparse file.
func SparseWriter(ctx context.Context, img *rbd.Image) io.Writer {
	return &sparseWriter{
		ctx:    ctx,
		img:    img,
		Writer: ctxio.NewWriter(ctx, img),
	}
}

// sparseWriter wraps an rbd.Image to support writing a sparse file
// to an rbd image.
type sparseWriter struct {
	io.Writer

	ctx context.Context
	img *rbd.Image
}

// ReadFrom reads from the io.Reader and handles if the io.Reader
// is a sparse file by only writing the content to the image and
// skipping the holes. Otherwise, a normal copy is performed.
func (s *sparseWriter) ReadFrom(r io.Reader) (int64, error) {
	// Check if the reader is a sparse file. If not, just force
	// a simple copy.
	f, ok := holes.MakeReader(r)
	if !ok {
		return io.Copy(s.Writer, r)
	}

	// Locate all the holes in the file.
	holes, err := holes.Collect(f)
	if err != nil {
		return 0, err
	}

	// Get the first hole in the file.
	hole := holes.Next()
	buf := make([]byte, BUFFER_SIZE)
	var offset int64

	for {
		// Check context if this should be interrupted.
		if s.ctx.Err() != nil {
			return offset, s.ctx.Err()
		}

		read, err := f.ReadAt(buf, offset)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				return offset, err
			}

			// If anything was read, process it and let the next
			// EOF force the return.
			if read == 0 {
				return offset, nil
			}
		}
		newOffset := offset + int64(read)

		// If the new offset is past the start of the hole, adjust the read count to
		// only send the data up to the start of the hole.
		if hole != nil && hole.StartsBefore(newOffset) {
			read -= hole.Overflow(newOffset)
		}

		// If there is data to write, send it to the stream.
		if read > 0 {
			if _, err := s.img.WriteAt(buf[0:read], offset); err != nil {
				return offset, err
			}
		}

		// Set the offset to the new offset value.
		offset = newOffset

		// If the offset is past the start of the hole, adjust the offset to
		// the end of the hole and get the next hole.
		if hole != nil && hole.StartsBefore(offset) {
			// Adjust the offset to the end of the hole so the next read starts
			// in the correct location and get the next hole.
			offset = hole.End()
			hole = holes.Next()
		}
	}
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package shims

import (
	"errors"
	"io"

	"github.com/hashicorp/nomad-driver-virt/internal/ctxio"
	"github.com/hashicorp/nomad-driver-virt/internal/holes"
)

const (
	// Size of the buffer for copying (same as io.Copy).
	BUFFER_SIZE = 32768
)

// ReadFrom copies data from the reader to the stream and supports
// sparse uploads if the reader is a sparse file.
func (l *libvirtStream) ReadFrom(r io.Reader) (int64, error) {
	// Check if the reader is a sparse file. If not, just force
	// a simple copy.
	f, ok := holes.MakeReader(r)
	if !ok {
		return io.Copy(ctxio.NewWriter(l.ctx, l), r)
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
		if l.ctx.Err() != nil {
			return offset, l.ctx.Err()
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
		offset += int64(read)

		// If the offset is past the start of the hole, adjust the read count to
		// only send the data up to the start of the hole.
		if hole != nil && hole.StartsBefore(offset) {
			read -= hole.Overflow(offset)
		}

		// If there is data to write, send it to the stream.
		if read > 0 {
			if _, err := l.s.Send(buf[0:read]); err != nil {
				return offset, err
			}
		}

		// If the offset has past the start of the hole, send the length of the
		// hole to the stream.
		if hole != nil && hole.StartsBefore(offset) {
			if err := l.s.SendHole(hole.Length(), libvirtNoFlags); err != nil {
				return offset, err
			}

			// Adjust the offset to the end of the hole so the next read starts
			// in the correct location and get the next hole.
			offset = hole.End()
			hole = holes.Next()
		}
	}
}

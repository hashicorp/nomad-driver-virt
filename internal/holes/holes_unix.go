// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package holes

import (
	"errors"
	"io"
	"syscall"
)

// NOTE: Whence values are defined originally in solaris and used across
// unix systems that implement sparse files. More information about
// whence values can be found in the lseek manual page: `man lseek`

const (
	// Whence value for seeking to next data position.
	SeekData = 3
	// Whence value for seeking to the next hole position.
	SeekHole = 4
)

// MakeReader will attempt to convert an io.Reader to a Reader and check
// if it is a sparse file, returning the Reader and true if the check was
// successful. Otherwise, will return nil and false.
func MakeReader(r io.Reader) (Reader, bool) {
	// First check if the supplied reader can even be converted
	// into the local reader.
	f, ok := r.(Reader)
	if !ok {
		return nil, false
	}

	// Get the current position. If the file is not seekable, this will error.
	// If it is, the returned position is used to reset the position when complete.
	pos, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, false
	}
	defer f.Seek(pos, io.SeekStart)

	// Get the final seekable position in the file.
	eof, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, false
	}

	// Reset back to original position
	if _, err := f.Seek(pos, io.SeekStart); err != nil {
		return nil, false
	}

	// Check if the file includes any holes.
	hole, err := f.Seek(0, SeekHole)
	if err != nil {
		return nil, false
	}

	// If the hole is the final position, there are no holes.
	if hole == eof {
		return nil, false
	}

	// It's a file with at least one hole!
	return f, true
}

// Collect returns a collection of all holes found in the Reader.
func Collect(f Reader) (Collection, error) {
	// Get the current position in the file so it can be reset when complete.
	pos, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	defer f.Seek(pos, io.SeekStart)

	holes := new(collection)
	var offset int64

	// Get the final seekable position of the file.
	eof, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return nil, err
	}

	// Reset to original position
	if _, err := f.Seek(pos, io.SeekStart); err != nil {
		return nil, err
	}

	for {
		// Seek into the file to the first hole position.
		start, err := f.Seek(offset, SeekHole)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return nil, err
		}

		// If the start position of the hole is the final seekable
		// position of the file, there is no final hole so stop.
		if start == eof {
			break
		}

		// Now seek from the start of the hole to find the end of the hole.
		end, err := f.Seek(start, SeekData)
		if err != nil {
			// An EOF error should not occur here since the seeking originated
			// from a hole. Discard this hole data.
			if errors.Is(err, io.EOF) {
				break
			}

			// If the end of the file is encountered while seeking to the end of
			// the hole, a special error is returned: ENXIO. If this error is
			// encountered, then the end of the hole is past the end of the file,
			// so set the end position of the hole to the final seekable position..
			if errors.Is(err, syscall.ENXIO) {
				holes.Add(&hole{start: start, end: eof})
				break
			}

			return nil, err
		}

		holes.Add(&hole{start: start, end: end})
		offset = end
	}

	return holes, nil
}

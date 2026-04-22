// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package holes

import (
	"crypto/rand"
	"os"
	"path/filepath"

	"github.com/shoenig/test/must"
)

const (
	testFileSize       = 50000
	testFileSparseName = "sparse.file"
	testFileFullName   = "full.file"
)

// TestFile creates 50k a sparse file named "sparse.file" within the provided
// directory which contains two holes at positions:
//   - 4096-12288
//   - 20480-40960
func TestFile(t must.T, dir string) string {
	t.Helper()

	return TestFileCreate(t, filepath.Join(dir, testFileSparseName), testFileSize, &collection{
		holes: []Hole{
			&hole{
				start: 4096,
				end:   12288,
			},
			&hole{
				start: 20480,
				end:   40960,
			},
		},
	})
}

// TestFiles creates two 50k test files: a sparse file and a full file.
func TestFiles(t must.T, dir string) (sparseFile string, fullFile string) {
	t.Helper()

	sparseFile = TestFile(t, dir)
	fullFile = filepath.Join(dir, testFileFullName)
	f, err := os.Create(fullFile)
	must.NoError(t, err, must.Sprint("failed to create full test file"))
	defer f.Close()
	buf := make([]byte, testFileSize)

	_, err = rand.Read(buf)
	must.NoError(t, err, must.Sprint("failed to generate content for full test file"))
	_, err = f.Write(buf)
	must.NoError(t, err, must.Sprint("failed to write to full test file"))

	return
}

// TestFileCreate creates a sparse file at the provided path of the given
// size with the requested holes.
func TestFileCreate(t must.T, path string, size int64, holes Collection) string {
	t.Helper()

	f, err := os.Create(path)
	must.NoError(t, err, must.Sprint("failed to create sparse test file"))
	defer f.Close()

	var pos int64
	hole := holes.Next()
	bufSize := int64(4096)
	buf := make([]byte, bufSize)

	for pos < size {
		read, err := rand.Read(buf)
		must.NoError(t, err, must.Sprint("failed to generate content for sparse test file"))

		// Check if any of the write will be within a hole and move back to
		// the beginning of the hole if so.
		finalPos := pos + bufSize
		if hole != nil && hole.StartsBefore(finalPos) {
			read -= hole.Overflow(finalPos)
		}

		// If the write will push past requested size, truncate.
		if pos+int64(read) > size {
			read = int(size - pos)
		}

		// Write the bytes to the file.
		_, err = f.WriteAt(buf[0:read], pos)
		must.NoError(t, err, must.Sprint("failed to write to sparse test file"))
		pos += int64(read)

		// If the position is at the start of the hole, jump
		// the hole and load the next hole.
		if hole != nil && hole.Start() == pos {
			pos = hole.End()
			hole = holes.Next()
		}
	}

	return path
}

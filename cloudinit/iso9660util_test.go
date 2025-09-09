// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package cloudinit

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
)

func TestWrite(t *testing.T) {
	tests := []struct {
		name      string
		isoPath   string
		label     string
		layout    []Entry
		wantError bool
	}{
		{
			name:    "Empty layout",
			isoPath: filepath.Join(t.TempDir(), "test_empty.iso"),
			label:   "EMPTY",
			layout:  []Entry{},
		},
		{
			name:    "Single file",
			isoPath: filepath.Join(t.TempDir(), "test_single_file.iso"),
			label:   "SINGLE",
			layout: []Entry{
				{
					Path:   filepath.Join(t.TempDir(), "/file.txt"),
					Reader: bytes.NewReader([]byte("Hello, World!")),
				},
			},
		},
		{
			name:    "Multiple files",
			isoPath: filepath.Join(t.TempDir(), "test_multiple_files.iso"),
			label:   "MULTIPLE",
			layout: []Entry{
				{
					Path:   filepath.Join(t.TempDir(), "/file1.txt"),
					Reader: bytes.NewReader([]byte("Hello, World 1!")),
				},
				{
					Path:   filepath.Join(t.TempDir(), "/file2.txt"),
					Reader: bytes.NewReader([]byte("Hello, World 2!")),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Write(tt.isoPath, tt.label, tt.layout)
			if (err != nil) != tt.wantError {
				t.Errorf("Write() error = %v, wantError %v", err, tt.wantError)
			}

			os.Remove(tt.isoPath)
		})
	}
}

func TestWriteFile(t *testing.T) {
	tests := []struct {
		name      string
		pathStr   string
		content   string
		wantError bool
	}{
		{
			name:    "Create new file",
			pathStr: filepath.Join(t.TempDir(), "/newfile.txt"),
			content: "This is a new file",
		},
		{
			name:    "Create file in new directory",
			pathStr: filepath.Join(t.TempDir(), "/newdir/newfile.txt"),
			content: "This is a file in a new directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workdir, err := os.MkdirTemp("", "diskfs_iso_test")
			if err != nil {
				t.Fatalf("failed to create temp dir: %v", err)
			}
			defer os.RemoveAll(workdir)

			isoFile, err := os.Create(filepath.Join(workdir, "test.iso"))
			if err != nil {
				t.Fatalf("failed to create iso file: %v", err)
			}
			defer isoFile.Close()

			backend := file.New(isoFile, false)
			fs, err := iso9660.Create(backend, 0, 0, 0, workdir)
			if err != nil {
				t.Fatalf("failed to create iso filesystem: %v", err)
			}

			_, err = writeFile(fs, tt.pathStr, bytes.NewReader([]byte(tt.content)))
			if (err != nil) != tt.wantError {
				t.Errorf("WriteFile() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

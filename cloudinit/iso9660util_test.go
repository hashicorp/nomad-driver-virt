package cloudinit

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/diskfs/go-diskfs/filesystem/iso9660"
)

func TestWriteFile(t *testing.T) {
	tests := []struct {
		name      string
		pathStr   string
		content   string
		wantError bool
	}{
		{
			name:    "Create new file",
			pathStr: "/newfile.txt",
			content: "This is a new file",
		},
		{
			name:    "Create file in new directory",
			pathStr: "/newdir/newfile.txt",
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

			fs, err := iso9660.Create(isoFile, 0, 0, 0, workdir)
			if err != nil {
				t.Fatalf("failed to create iso filesystem: %v", err)
			}

			_, err = WriteFile(fs, tt.pathStr, bytes.NewReader([]byte(tt.content)))
			if (err != nil) != tt.wantError {
				t.Errorf("WriteFile() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

package testutil

import (
	"syscall"
	"testing"
)

func RequireRoot(t *testing.T) {
	if syscall.Geteuid() != 0 {
		t.Skip("Test requires root")
	}
}

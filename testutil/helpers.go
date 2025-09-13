package testutil

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"

	"github.com/coreos/go-iptables/iptables"
)

// RequireRoot will skip the test if not running as root.
func RequireRoot(t *testing.T) {
	if syscall.Geteuid() != 0 {
		t.Skip("Test requires root")
	}
}

// RequireIPTables will skip the test if not running as
// root or if iptables is not available.
func RequireIPTables(t *testing.T) {
	RequireRoot(t)

	_, err := iptables.New()
	if errors.Is(err, exec.ErrNotFound) {
		t.Skip("Test requires iptables")
	}
}

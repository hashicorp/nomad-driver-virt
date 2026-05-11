// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package testutil

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"

	"github.com/coreos/go-iptables/iptables"
)

// RequireRoot will skip the test if not running as root. If the
// test is detected as running within CI it will error.
func RequireRoot(t *testing.T) {
	if syscall.Geteuid() != 0 {
		if isCI() {
			t.Fatal("Test requires root in CI")
			return
		}

		t.Skip("Test requires root")
	}
}

// RequireIPTables will skip the test if not running as
// root or if iptables is not available. If the test is
// detected as running within CI it will error.
func RequireIPTables(t *testing.T) {
	RequireRoot(t)

	_, err := iptables.New()
	if errors.Is(err, exec.ErrNotFound) {
		if isCI() {
			t.Fatal("Test requires iptables in CI")
			return
		}

		t.Skip("Test requires iptables")
	}
}

// RequireQemuImg will check if the qemu-img executable is
// available and error if it is not found.
func RequireQemuImg(t *testing.T) {
	_, err := exec.LookPath("qemu-img")
	if err != nil {
		t.Fatalf("Test requires qemu-img executable (%s)", err)
	}
}

// isCI checks if the process is currently running in
// CI by checking for a `CI` environment variable.
func isCI() bool {
	return os.Getenv("CI") != ""
}

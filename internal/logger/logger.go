// This is a simple package for managing the default logger use
// by the plugin. It adds basic synchronization to prevent triggering
// racy errors when testing.
package logger

import (
	"sync"

	"github.com/hashicorp/go-hclog"
)

var (
	logger hclog.Logger
	m      sync.Mutex
)

// SetDefault sets the default logger.
func SetDefault(l hclog.Logger) {
	m.Lock()
	defer m.Unlock()

	logger = l
}

// Default returns the default logger.
func Default() hclog.Logger {
	m.Lock()
	defer m.Unlock()

	if logger == nil {
		return hclog.Default()
	}

	return logger
}

package nvme

import (
	"github.com/hashicorp/go-hclog"
)

// AttributeHandler is the NVMe handler for driver attributes and should be
// used to populate the driver attribute mapping with NVMe specific detail.
type AttributeHandler struct {
	logger          hclog.Logger
	hostNQNFilePath string
}

// NewAttributeHandler returns a handler that should be used to collect all
// NVMe related driver attributes.
func NewAttributeHandler(logger hclog.Logger, hostNQNPath string) *AttributeHandler {

	// Ensure the path is set, so callers don't necessarily have to worry about
	// this.
	if hostNQNPath == "" {
		hostNQNPath = defaultHostNQNFilePath()
	}

	return &AttributeHandler{
		logger:          logger,
		hostNQNFilePath: hostNQNPath,
	}
}

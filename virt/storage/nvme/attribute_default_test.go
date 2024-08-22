//go:build !linux

package nvme

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/shoenig/test/must"
)

func Test_defaultHostNQNFilePath(t *testing.T) {
	must.Eq(t, "", defaultHostNQNFilePath())
}

func TestAttributeHandler_HostNQN(t *testing.T) {
	attributeHandler := NewAttributeHandler(hclog.NewNullLogger(), "")
	must.Eq(t, "", attributeHandler.HostNQN())
}

func TestAttributeHandler_KernelModules(t *testing.T) {
	attributeHandler := NewAttributeHandler(hclog.NewNullLogger(), "")
	must.False(t, attributeHandler.TCPKernelModule())
}

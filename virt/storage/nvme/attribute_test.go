package nvme

import (
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/shoenig/test/must"
)

func Test_NewDriverAttributes(t *testing.T) {
	testCases := []struct {
		name         string
		inputLogger  hclog.Logger
		inputPath    string
		expectedPath string
	}{
		{
			name:         "empty input nqn path",
			inputLogger:  hclog.NewNullLogger(),
			inputPath:    "",
			expectedPath: defaultHostNQNFilePath(),
		},
		{
			name:         "default input nqn path",
			inputLogger:  hclog.NewNullLogger(),
			inputPath:    defaultHostNQNFilePath(),
			expectedPath: defaultHostNQNFilePath(),
		},
		{
			name:         "custom input nqn path",
			inputLogger:  hclog.NewNullLogger(),
			inputPath:    "/opt/custom/nvme/hostnqn",
			expectedPath: "/opt/custom/nvme/hostnqn",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			attributesHandler := NewAttributeHandler(tc.inputLogger, tc.inputPath)
			must.NotNil(t, attributesHandler)
			must.Eq(t, tc.expectedPath, attributesHandler.hostNQNFilePath)
		})
	}
}

//go:build linux

package nvme

import (
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/shoenig/test/must"
)

func Test_defaultHostNQNFilePath(t *testing.T) {
	must.Eq(t, DefaultHostNQNFilePath, defaultHostNQNFilePath())
}

func TestAttributesHandler_HostNQN(t *testing.T) {

	// Test a path that is guaranteed not to exist, so we can try and ensure no
	// panic or other unknown error occurs.
	attributeHandler := NewAttributeHandler(hclog.NewNullLogger(), "/opt/nope")
	must.Eq(t, "", attributeHandler.HostNQN())

	// Write a test file that contains an example host NQN to read out.
	testFile, err := os.CreateTemp("", "driver-virt-nvme-hostnqnq-")
	must.NoError(t, err)
	must.NotNil(t, testFile)

	defer func() {
		_ = testFile.Close()
	}()

	testHostNQN := "nqn.2014-08.org.nvmexpress:uuid:8ae2b12c-3d28-4458-83e3-658e571ed4b8"

	_, err = testFile.Write([]byte(testHostNQN))
	must.NoError(t, err)

	// Create a new handler and read the newly created file. These should match
	// exactly.
	attributeHandler = NewAttributeHandler(hclog.NewNullLogger(), testFile.Name())
	must.Eq(t, testHostNQN, attributeHandler.HostNQN())

	// Test writing an NQN that includes a newline character, to ensure this is
	// removed.
	testHostNQNNewLine := "nqn.2014-08.org.nvmexpress:uuid:8ae2b12c-3d28-4458-83e3-658e571ed4b8\n"

	_, err = testFile.WriteAt([]byte(testHostNQNNewLine), 0)
	must.NoError(t, err)

	// Create a new handler and read the newly created file. These should match
	// exactly without the newline addition.
	attributeHandler = NewAttributeHandler(hclog.NewNullLogger(), testFile.Name())
	must.Eq(t, testHostNQN, attributeHandler.HostNQN())
}

func TestAttributeHandler_parseNVMECLIVersion(t *testing.T) {
	testCases := []struct {
		name           string
		inputString    string
		expectedOutput string
	}{
		{
			name:           "linux 2.8",
			inputString:    "nvme version 2.8 (git 2.8)\nlibnvme version 1.8 (git 1.8)\n",
			expectedOutput: "2.8",
		},
		{
			name:           "empty input",
			inputString:    "",
			expectedOutput: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			attributeHandler := NewAttributeHandler(hclog.Default(), "")
			must.NotNil(t, attributeHandler)

			actualOutput := attributeHandler.parseNVMECLIVersion(tc.inputString)
			must.Eq(t, tc.expectedOutput, actualOutput)
		})
	}
}

func TestAttributesHandler_searchFile(t *testing.T) {
	testCases := []struct {
		name           string
		inputFile      string
		inputRegex     *regexp.Regexp
		expectedOutput bool
	}{
		{
			name:           "non-existent file",
			inputFile:      "/opt/something/something/nope",
			inputRegex:     nil,
			expectedOutput: false,
		},
		{
			name:           "aarch64 not found proc modules",
			inputFile:      "./test-fixtures/linux-aarch64-proc-modules",
			inputRegex:     buildKernelModuleRegex(kernelModuleDynamicRegex, "my_module"),
			expectedOutput: false,
		},
		{
			name:           "aarch64 found proc modules",
			inputFile:      "./test-fixtures/linux-aarch64-proc-modules",
			inputRegex:     buildKernelModuleRegex(kernelModuleDynamicRegex, "nvme_tcp"),
			expectedOutput: true,
		},
		{
			name:           "aarch64 not found modules.builtin",
			inputFile:      "./test-fixtures/linux-aarch64-proc-modules",
			inputRegex:     buildKernelModuleRegex(kernelModuleBuiltinRegex, "my_module"),
			expectedOutput: false,
		},
		{
			name:           "aarch64 found modules.builtin",
			inputFile:      "./test-fixtures/linux-aarch64-modules.builtin",
			inputRegex:     buildKernelModuleRegex(kernelModuleBuiltinRegex, "virtio_scsi"),
			expectedOutput: true,
		},
		{
			name:           "aarch64 not found modules.dep",
			inputFile:      "./test-fixtures/linux-aarch64-modules.dep",
			inputRegex:     buildKernelModuleRegex(kernelModuleDependsRegex, "my_module"),
			expectedOutput: false,
		},
		{
			name:           "aarch64 found modules.dep",
			inputFile:      "./test-fixtures/linux-aarch64-modules.dep",
			inputRegex:     buildKernelModuleRegex(kernelModuleDependsRegex, "vsock"),
			expectedOutput: true,
		},
		{
			name:           "x86-64 not found proc modules",
			inputFile:      "./test-fixtures/linux-x86-64-proc-modules",
			inputRegex:     buildKernelModuleRegex(kernelModuleDynamicRegex, "my_module"),
			expectedOutput: false,
		},
		{
			name:           "x86-64 found proc modules",
			inputFile:      "./test-fixtures/linux-x86-64-proc-modules",
			inputRegex:     buildKernelModuleRegex(kernelModuleDynamicRegex, "nvme_tcp"),
			expectedOutput: true,
		},
		{
			name:           "x86-64 not found modules.builtin",
			inputFile:      "./test-fixtures/linux-x86-64-proc-modules",
			inputRegex:     buildKernelModuleRegex(kernelModuleBuiltinRegex, "my_module"),
			expectedOutput: false,
		},
		{
			name:           "x86-64 found modules.builtin",
			inputFile:      "./test-fixtures/linux-x86-64-modules.builtin",
			inputRegex:     buildKernelModuleRegex(kernelModuleBuiltinRegex, "scsi_mod"),
			expectedOutput: true,
		},
		{
			name:           "x86-64 not found modules.dep",
			inputFile:      "./test-fixtures/linux-x86-64-modules.dep",
			inputRegex:     buildKernelModuleRegex(kernelModuleDependsRegex, "my_module"),
			expectedOutput: false,
		},
		{
			name:           "x86-64 found modules.dep",
			inputFile:      "./test-fixtures/linux-x86-64-modules.dep",
			inputRegex:     buildKernelModuleRegex(kernelModuleDependsRegex, "vsock"),
			expectedOutput: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			attributeHandler := NewAttributeHandler(hclog.Default(), "")
			must.NotNil(t, attributeHandler)

			actualOutput := attributeHandler.searchFile(tc.inputFile, tc.inputRegex)
			must.Eq(t, tc.expectedOutput, actualOutput)
		})
	}
}

//go:build linux

package nvme

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/shirou/gopsutil/v4/host"
)

const (
	// DefaultHostNQNFilePath is the standard path to the host NQN file on
	// Linux. It is unlikely this will need overriding in running systems, but
	// is provided as an option to aid testing.
	DefaultHostNQNFilePath = "/etc/nvme/hostnqn"

	// tcpKernelModuleName is the name of the Linux kernel module needed on the
	// host in order to support NVMe-oF via TCP.
	tcpKernelModuleName = "nvme_tcp"

	// kernelModuleDynamicRegex is the regex to find whether a named module
	// (%s) is listed within the "/proc/modules" file. A line within the file
	// can look like "nvme_tcp 49152 0 - Live 0x0000000000000000".
	kernelModuleDynamicRegex = `%s\s+.*$`

	// kernelModuleBuiltinRegex is the regex to find whether a named module
	// (%s) is listed within the "/lib/modules/$(uname -r)/modules.builtin"
	// file. A line within the file can look like
	// "kernel/arch/arm64/kvm/kvm.ko".
	kernelModuleBuiltinRegex = `.+/%s.ko$`

	// kernelModuleDependsRegex is the regex to find whether a named module
	// (%s) is listed within the "/lib/modules/$(uname -r)/modules.dep" file.
	// A line within the file can look like
	// "kernel/fs/quota/quota_v2.ko.zst: kernel/fs/quota/quota_tree.ko.zst".
	kernelModuleDependsRegex = `.+/%s.ko(\.xz)?(\.zst)?:.*$`
)

var (
	// cliVersionCommand is the NVMe CLI command to execute in order to find
	// the version identifier.
	cliVersionCommand = []string{"nvme", "version"}
)

func defaultHostNQNFilePath() string { return DefaultHostNQNFilePath }

// HostNQN interrogates the host filesystem in order to identify the NVMe host
// NQN. If the host does not support NVMe-oF, this function will return an
// empty string. There is no option of an error return, indicating any errors
// are logged when needed, along with useful information for debugging.
func (a *AttributeHandler) HostNQN() string {

	// Stat the file to see if it exists. Many Nomad clients won't support
	// NVMe, so non-existence of the file is not an error.
	fileInfo, err := os.Stat(a.hostNQNFilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			a.logger.Debug("host NQN file not found", "path", a.hostNQNFilePath)
		} else {
			a.logger.Error("failed to stat host NQN file",
				"path", a.hostNQNFilePath, "error", err)
		}
		return ""
	}

	// The previous call gives us this information, so we might as well check
	// it isn't a directory.
	if fileInfo.IsDir() {
		a.logger.Error("host NQN file is a directory", "path", a.hostNQNFilePath)
		return ""
	}

	// Read the identified file and return its contents if no error occurs.
	hostNQN, err := os.ReadFile(a.hostNQNFilePath)
	if err != nil {
		a.logger.Error("failed to read host NQN file", "path", a.hostNQNFilePath,
			"error", err)
		return ""
	}
	return strings.TrimSuffix(string(hostNQN[:]), "\n")
}

// CLIVersion executes the NVMe CLI, if available, and returns the version
// identifier. If the host does not support NVMe-oF, this function will return
// an empty string. There is no option of an error return, indicating any
// errors are logged when needed, along with useful information for debugging.
func (a *AttributeHandler) CLIVersion() string {

	var out bytes.Buffer

	cmd := exec.Command(cliVersionCommand[0], cliVersionCommand[1:]...)
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		a.logger.Debug("unable to find nvme-cli version", "error", err)
		return ""
	}

	if s := out.String(); s != "" {
		return a.parseNVMECLIVersion(s)
	}
	return ""
}

// parseNVMECLIVersion is responsible for taking the string output from the
// NVMe version command and extracting the version identifier.
func (a *AttributeHandler) parseNVMECLIVersion(s string) string {

	// Grab each line of the output which includes the trailing newline at the
	// end. We do not currently care about the libnvme version, but if we do in
	// the future, this is available at index 1 in the split.
	splitLines := strings.Split(s, "\n")
	if len(splitLines) != 3 {
		a.logger.Debug("unable to process NVMe CLI version output", "content", s)
		return ""
	}

	// Split the string based on the spaces in the line. It would also be
	// possible to use a regex such as "([.\d]+)" but the output is unlikely to
	// change and is consistent across operating systems, so use the simplest
	// method.
	nvmeVersionSplit := strings.Split(splitLines[0], " ")
	if len(nvmeVersionSplit) != 5 {
		a.logger.Debug("unable to process NVMe CLI version line", "content", splitLines[0])
		return ""
	}

	return nvmeVersionSplit[2]
}

// TCPKernelModule interrogates the host to identify if the kernel module
// "nvme_tcp" is available. There is no option of an error return, indicating
// any errors are logged when needed, along with useful information for
// debugging.
func (a *AttributeHandler) TCPKernelModule() bool {

	// The "/sys/module/(module_name)/" directory contains individual files
	// that are each individual parameters of the module that can be
	// changed at runtime. It is the simplest place to start when looking
	// for loaded kernel modules.
	if _, err := os.Stat("/sys/module/" + tcpKernelModuleName); err == nil {
		return true
	}

	// The "/proc/modules" file displays a list of all modules dynamically
	// loaded into the kernel.
	if a.searchFile("/proc/modules", buildKernelModuleRegex(kernelModuleDynamicRegex, tcpKernelModuleName)) {
		return true
	}

	// The host kernel information is needed to perform some kernel module
	// lookups. Performing this here, means we can do this only once, but we
	// must protect against using a nil object later on.
	hostInfo, err := host.Info()
	if err != nil {
		a.logger.Error("failed to lookup host kernel version", "error", err)
		return false
	}

	// The file "/lib/modules/$(uname -r)/modules.builtin" contains a list
	// of all modules which are statically compiled into the kernel.
	builtinPath := fmt.Sprintf("/lib/modules/%s/modules.builtin", hostInfo.KernelVersion)
	if a.searchFile(builtinPath, buildKernelModuleRegex(kernelModuleBuiltinRegex, tcpKernelModuleName)) {
		return true
	}

	// The file "/lib/modules/$(uname -r)/modules.dep" contains a list of
	// the dependencies for every module in the directories under
	// "lib/modules/$(uname -r)".
	dependsPath := fmt.Sprintf("/lib/modules/%s/modules.dep", hostInfo.KernelVersion)
	if a.searchFile(dependsPath, buildKernelModuleRegex(kernelModuleDependsRegex, tcpKernelModuleName)) {
		return true
	}

	a.logger.Debug("did not find TCP kernel module", "module_name", tcpKernelModuleName)
	return false
}

// searchFile opens and reads the contents of the named file looking for the
// passed regex. The function will return on the first match indicating it has
// been found.
func (a *AttributeHandler) searchFile(filePath string, re *regexp.Regexp) bool {

	file, err := os.Open(filePath)
	if err != nil {
		a.logger.Error("failed to open file", "path", filePath, "error", err)
		return false
	}
	defer func() {
		_ = file.Close()
	}()

	// Scan the file and look for a regex match. The first match is good enough
	// to return. Any error scanning the file that isn't an EOF is terminal and
	// logged.
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		if re.MatchString(scanner.Text()) {
			a.logger.Debug("found regex match in file",
				"path", filePath, "regex", re.String())
			return true
		}
	}
	if err := scanner.Err(); err != nil {
		a.logger.Error("failed to scan file", "path", filePath, "error", err)
		return false
	}

	return false
}

func buildKernelModuleRegex(pattern, module string) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(pattern, module))
}

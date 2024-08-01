//go:build !linux

package nvme

func defaultHostNQNFilePath() string { return "" }

func (a *AttributeHandler) HostNQN() string { return "" }

func (a *AttributeHandler) CLIVersion() string { return "" }

func (a *AttributeHandler) TCPKernelModule() bool { return false }

package image_tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"github.com/hashicorp/go-hclog"
)

var (
	ErrNoMemory = errors.New("invalid memory assignation")
)

type QemuTools struct {
	logger hclog.Logger
}

func NewHandler(logger hclog.Logger) *QemuTools {
	return &QemuTools{
		logger: logger,
	}
}

// GetImageFormat runs `qemu-img info` to get the format of a disk image.
func (q *QemuTools) GetImageFormat(basePath string) (string, error) {
	q.logger.Debug("reading the disk format", "base", basePath)

	var stdoutBuf, stderrBuf bytes.Buffer

	cmd := exec.Command("qemu-img", "info", "--output=json", basePath)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if err != nil {
		q.logger.Error("qemu-img read image", "stderr", stderrBuf.String())
		q.logger.Debug("qemu-img read image", "stdout", stdoutBuf.String())
		return "", err
	}

	q.logger.Debug("qemu-img read image", "stdout", stdoutBuf.String())

	// The qemu command returns more information, but for now, only the format
	// is necessary.
	var output = struct {
		Format string `json:"format"`
	}{}

	err = json.Unmarshal(stdoutBuf.Bytes(), &output)
	if err != nil {
		return "", fmt.Errorf("qemu-img: unable read info response %s: %w", basePath, err)
	}

	return output.Format, nil
}

func (q *QemuTools) CreateThinCopy(basePath string, destination string, sizeM int64) error {
	q.logger.Debug("creating thin copy", "base", basePath, "dest", destination)

	var stdoutBuf, stderrBuf bytes.Buffer

	if sizeM <= 0 {
		return fmt.Errorf("qemu-img: %w", ErrNoMemory)
	}

	cmd := exec.Command("qemu-img", "create", "-b", basePath, "-f", "qcow2", "-F", "qcow2",
		destination, fmt.Sprintf("%dM", sizeM),
	)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if err != nil {
		q.logger.Error("qemu-img create output", "stderr", stderrBuf.String())
		q.logger.Debug("qemu-img create output", "stdout", stdoutBuf.String())
		return err
	}

	q.logger.Debug("qemu-img create output", "stdout", stdoutBuf.String())
	return nil
}

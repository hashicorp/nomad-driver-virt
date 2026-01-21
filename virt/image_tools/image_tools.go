// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package image_tools

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/hashicorp/go-hclog"
)

var (
	ErrNoMemory = errors.New("invalid memory assignation")
)

// ImageHandler is the interface handling image files directly.
type ImageHandler interface {
	GetImageFormat(path string) (string, error)
	CreateCopy(src, dst string, sizeM int64) error
	CreateChainedCopy(src, dst string, sizeM int64) error
}

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

// CreateCopy creates a full copy from the src image
func (q *QemuTools) CreateCopy(src, dst string, sizeM int64) error {
	q.logger.Debug("creating copy", "base", src, "dest", dst)
	if err := os.MkdirAll(filepath.Base(dst), 0755); err != nil {
		return err
	}

	var stdoutBuf, stderrBuf bytes.Buffer

	if sizeM <= 0 {
		return fmt.Errorf("qemu-img: %w", ErrNoMemory)
	}

	// First create the new image file of approripate size
	cmd := exec.Command("qemu-img", "create", "-F", "qcow2", dst, fmt.Sprintf("%dM", sizeM))
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if err != nil {
		q.logger.Error("qemu-img create output", "stderr", stderrBuf.String())
		q.logger.Debug("qemu-img create output", "stdout", stdoutBuf.String())
		return err
	}

	// Now copy the source image into the new file
	stderrBuf.Reset()
	stdoutBuf.Reset()
	cmd = exec.Command("qemu-img", "dd", fmt.Sprintf("if=%s", src), fmt.Sprintf("of=%s", dst))
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err = cmd.Run()
	if err != nil {
		q.logger.Error("qemu-img dd output", "stderr", stderrBuf.String())
		q.logger.Debug("qemu-img dd  output", "stdout", stdoutBuf.String())
		return err
	}

	return nil
}

// CreateChainedCopy creates a copy chained copy from src image
func (q *QemuTools) CreateChainedCopy(src string, destination string, sizeM int64) error {
	q.logger.Debug("creating chained copy", "base", src, "dest", destination)

	var stdoutBuf, stderrBuf bytes.Buffer

	if sizeM <= 0 {
		return fmt.Errorf("qemu-img: %w", ErrNoMemory)
	}

	cmd := exec.Command("qemu-img", "create", "-b", src, "-f", "qcow2", "-F", "qcow2",
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

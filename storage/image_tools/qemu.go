// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package image_tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/hashicorp/go-hclog"
)

type QemuTools struct {
	logger hclog.Logger
}

func NewQemuHandler(logger hclog.Logger) *QemuTools {
	return &QemuTools{
		logger: logger,
	}
}

// ConvertImage makes a copy of the image in a new format
func (q *QemuTools) ConvertImage(src, srcFmt, dst, dstFmt string) error {
	var err error
	if srcFmt == "" {
		srcFmt, err = q.GetImageFormat(src)
		if err != nil {
			return err
		}
	}

	var stdoutBuf, stderrBuf bytes.Buffer

	cmd := exec.Command("qemu-img", "convert", "-f", srcFmt, "-O", dstFmt, src, dst)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		q.logger.Error("qemu-img convert image", "stderr", stderrBuf.String())
		q.logger.Debug("qemu-img convert image", "stdout", stdoutBuf.String())
		return err
	}

	return nil
}

// GetImageFormat gets the format of a given image
func (q *QemuTools) GetImageFormat(path string) (string, error) {
	var output = struct {
		Format string `json:"format"`
	}{}

	if err := q.getImageInfo(path, &output); err != nil {
		return "", err
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
		q.logger.Debug("qemu-img dd output", "stdout", stdoutBuf.String())
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

// GetImageSize returns the real size of the image. For some formats
// the size of the image will be larger than the size of the image
// file itself.
func (q *QemuTools) GetImageSize(path string) (uint64, error) {
	var output = struct {
		Size uint64 `json:"virtual-size"`
	}{}

	if err := q.getImageInfo(path, &output); err != nil {
		return 0, err
	}

	return output.Size, nil
}

// getImageInfo reads information about the image file and unmarshals the result
// into the provided value.
func (q *QemuTools) getImageInfo(path string, result any) error {
	q.logger.Debug("reading image info", "image", path)

	var stdoutBuf, stderrBuf bytes.Buffer

	cmd := exec.Command("qemu-img", "info", "--output=json", path)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	if err != nil {
		q.logger.Error("qemu-img read image", "stderr", stderrBuf.String())
		q.logger.Debug("qemu-img read image", "stdout", stdoutBuf.String())
		return err
	}

	q.logger.Debug("qemu-img read image", "stdout", stdoutBuf.String())

	err = json.Unmarshal(stdoutBuf.Bytes(), &result)
	if err != nil {
		return fmt.Errorf("qemu-img: unable read info response %s: %w", path, err)
	}

	return nil
}

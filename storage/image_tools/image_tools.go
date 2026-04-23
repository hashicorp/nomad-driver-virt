// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package image_tools

import (
	"errors"
)

var (
	ErrNoMemory = errors.New("invalid memory assignation")
)

// ImageHandler is the interface handling image files directly.
type ImageHandler interface {
	// ConvertImage makes a copy of the image in a new format
	ConvertImage(src, srcFmt, dst, dstFmt string) error

	// CreateCopy creates a full copy from the src image
	CreateCopy(src, dst string, sizeM int64) error

	// CreateChainedCopy creates a copy chained copy from src image
	CreateChainedCopy(src, dst string, sizeM int64) error

	// GetImageFormat returns the format of a given image
	GetImageFormat(path string) (string, error)

	// GetImageSize returns the real size of the image. For some formats
	// the size of the image will be larger than the size of the image
	// file itself.
	GetImageSize(path string) (uint64, error)
}

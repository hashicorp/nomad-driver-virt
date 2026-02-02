// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

type Volume struct {
	Pool       string // Name of the pool containing volume
	Name       string // Name of the volume
	Kind       string // Kind of attachment (ide, scsi, etc)
	Driver     string // Driver for attachment
	Format     string // Format of the image
	DeviceName string // Device name of the attachment
	BusType    string // Bus type used by the attachment
	Primary    bool   // Primary disk for booting
	Block      string // Block device to pass through as attachment
}

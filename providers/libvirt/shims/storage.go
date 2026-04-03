// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package shims

import (
	"fmt"

	"libvirt.org/go/libvirt"
)

// StoragePool is the shim interface that wraps the libvirt StoragePool.
type StoragePool interface {
	// Create starts an inactive pool.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStoragePoolCreate
	Create(libvirt.StoragePoolCreateFlags) error

	// Free frees the resources associated to this instance.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStoragePoolFree
	Free() error

	// GetXMLDesc returns an XML document describing the storage pool.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStoragePoolGetXMLDesc
	GetXMLDesc(libvirt.StorageXMLFlags) (string, error)

	// GetName returns the locally unique name of the storage pool.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStoragePoolGetName
	GetName() (string, error)

	// IsActive determines if storage pool is running.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStoragePoolIsActive
	IsActive() (bool, error)

	// LookupStorageVolByName returns a storage volume based on its name within a pool.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStorageVolLookupByName
	LookupStorageVolByName(string) (StorageVol, error)

	// Refresh requests that the pool refresh its list of volumes
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStoragePoolRefresh
	Refresh(uint32) error

	// SetAutostart configures the storage pool to be started automatically when host boots.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStoragePoolSetAutostart
	SetAutostart(bool) error

	// StorageVolCreateXML creates a storage volume within a pool based on an XML description.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStorageVolCreateXML
	StorageVolCreateXML(string, libvirt.StorageVolCreateFlags) (StorageVol, error)

	// StorageVolCreateXMLFrom creates a storage volume in the parent pool, using the 'clonevol' volume as input.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStorageVolCreateXMLFrom
	StorageVolCreateXMLFrom(string, StorageVol, libvirt.StorageVolCreateFlags) (StorageVol, error)
}

// StorageVol is the shim interface that wraps the libvirt StorageVol.
type StorageVol interface {
	// Delete delete the storage volume from the pool.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStorageVolDelete
	Delete(libvirt.StorageVolDeleteFlags) error

	// Free frees the resources associated to this instance.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStorageVolFree
	Free() error

	// GetInfo returns volatile information about the storage volume such as its current allocation.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStorageVolGetInfo
	GetInfo() (*libvirt.StorageVolInfo, error)

	// GetPath returns the storage volume path.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStorageVolGetPath
	GetPath() (string, error)

	// GetXMLDesc returns an XML document describing all aspects of the storage volume.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStorageVolGetXMLDesc
	GetXMLDesc(uint32) (string, error)

	// LookupPoolByVolume
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStoragePoolLookupByVolume
	LookupPoolByVolume() (StoragePool, error)

	// Resize changes the storage capacity of the volume.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStorageVolResize
	Resize(uint64, libvirt.StorageVolResizeFlags) error

	// Upload configures a stream for uploading content to a volume.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-storage.html#virStorageVolUpload
	Upload(Stream, uint64, uint64, libvirt.StorageVolUploadFlags) error
}

// Stream is the shim interface that wraps the libvirt Stream.
type Stream interface {
	// Abort requests data transfer be cancelled abnormally.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-stream.html#virStreamAbort
	Abort() error

	// Finish indicates no further data to be transmitted on the stream.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-stream.html#virStreamFinish
	Finish() error

	// Free frees the resources associated to this instance.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-stream.html#virStreamFree
	Free() error

	// Read reads a series of bytes from the stream.
	// NOTE: Name is modified from Recv -> Read for interface support.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-stream.html#virStreamRecv
	Read([]byte) (int, error)
	// Write writes a series of bytes to the stream.
	// NOTE: Name is modified from Send -> Write for interface support.
	//
	// Also see:
	// https://libvirt.org/html/libvirt-libvirt-stream.html#virStreamSend
	Write([]byte) (int, error)
}

// WrapStoragePool wraps a libvirt StoragePool in the shim.
func WrapStoragePool(pool *libvirt.StoragePool) StoragePool {
	return &libvirtStoragePool{pool}
}

// WrapStorageVol wraps a libvirt StorageVol in the shim.
func WrapStorageVol(vol *libvirt.StorageVol) StorageVol {
	return &libvirtStorageVol{vol}
}

// WrapStream wraps a libvirt Stream in the shim.
func WrapStream(stream *libvirt.Stream) *libvirtStream {
	return &libvirtStream{stream}
}

// Below are storage related shim implementations
type libvirtStoragePool struct {
	pool *libvirt.StoragePool
}

func (l *libvirtStoragePool) Create(flags libvirt.StoragePoolCreateFlags) error {
	return l.pool.Create(flags)
}

func (l *libvirtStoragePool) Free() error {
	return l.pool.Free()
}

func (l *libvirtStoragePool) GetName() (string, error) {
	return l.pool.GetName()
}

func (l *libvirtStoragePool) GetXMLDesc(flags libvirt.StorageXMLFlags) (string, error) {
	return l.pool.GetXMLDesc(flags)
}

func (l *libvirtStoragePool) IsActive() (bool, error) {
	return l.pool.IsActive()
}

func (l *libvirtStoragePool) LookupStorageVolByName(name string) (StorageVol, error) {
	v, err := l.pool.LookupStorageVolByName(name)
	if err != nil {
		return nil, err
	}
	return &libvirtStorageVol{vol: v}, nil
}

func (l *libvirtStoragePool) Refresh(flags uint32) error {
	return l.pool.Refresh(flags)
}

func (l *libvirtStoragePool) SetAutostart(autostart bool) error {
	return l.pool.SetAutostart(autostart)
}

func (l *libvirtStoragePool) StorageVolCreateXML(desc string, flags libvirt.StorageVolCreateFlags) (StorageVol, error) {
	v, err := l.pool.StorageVolCreateXML(desc, flags)
	if err != nil {
		return nil, err
	}
	return &libvirtStorageVol{vol: v}, nil
}

func (l *libvirtStoragePool) StorageVolCreateXMLFrom(xmlDesc string, cloneVol StorageVol, flags libvirt.StorageVolCreateFlags) (StorageVol, error) {
	volShim, ok := cloneVol.(*libvirtStorageVol)
	if !ok {
		return nil, fmt.Errorf("invalid type, cannot cast to shim - %T", cloneVol)
	}
	v, err := l.pool.StorageVolCreateXMLFrom(xmlDesc, volShim.vol, flags)
	if err != nil {
		return nil, err
	}
	return &libvirtStorageVol{vol: v}, nil
}

type libvirtStorageVol struct {
	vol *libvirt.StorageVol
}

func (l *libvirtStorageVol) Delete(flags libvirt.StorageVolDeleteFlags) error {
	return l.vol.Delete(flags)
}

func (l *libvirtStorageVol) Free() error {
	return l.vol.Free()
}

func (l *libvirtStorageVol) GetPath() (string, error) {
	return l.vol.GetPath()
}

func (l *libvirtStorageVol) GetXMLDesc(_ uint32) (string, error) {
	return l.vol.GetXMLDesc(0)
}

func (l *libvirtStorageVol) LookupPoolByVolume() (StoragePool, error) {
	p, err := l.vol.LookupPoolByVolume()
	if err != nil {
		return nil, err
	}

	return WrapStoragePool(p), nil
}

func (l *libvirtStorageVol) Resize(size uint64, flags libvirt.StorageVolResizeFlags) error {
	return l.vol.Resize(size, flags)
}

func (l *libvirtStorageVol) GetInfo() (*libvirt.StorageVolInfo, error) {
	return l.vol.GetInfo()
}

func (l *libvirtStorageVol) Upload(stream Stream, offset uint64, size uint64, flags libvirt.StorageVolUploadFlags) error {
	ls, ok := stream.(*libvirtStream)
	if !ok {
		return fmt.Errorf("cannot access raw stream for upload")
	}

	return l.vol.Upload(ls.RawStream(), offset, size, flags)
}

type libvirtStream struct {
	s *libvirt.Stream
}

func (l *libvirtStream) Abort() error {
	return l.s.Abort()
}

func (l *libvirtStream) Finish() error {
	return l.s.Finish()
}

func (l *libvirtStream) Free() error {
	return l.s.Free()
}

func (l *libvirtStream) Read(data []byte) (int, error) {
	return l.s.Recv(data)
}

func (l *libvirtStream) Write(data []byte) (int, error) {
	return l.s.Send(data)
}

func (l *libvirtStream) RawStream() *libvirt.Stream {
	return l.s
}

var (
	_ StoragePool = (*libvirtStoragePool)(nil)
	_ StorageVol  = (*libvirtStorageVol)(nil)
	_ Stream      = (*libvirtStream)(nil)
)

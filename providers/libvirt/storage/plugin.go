// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package storage

import (
	"embed"
	"fmt"
	"io"
	"os"
	"plugin"
	"sync"
)

// Since libvirt does not support streams for rbd volumes a direct
// connection to ceph is required for uploading content into volumes.
// Adding direct support will create a dependency on rados and rbd
// libraries and require them to be available even if the user is
// not using ceph. To prevent this, the rados/rbd specifics are built
// as a plugin and embeded, only to be loaded if a ceph storage
// pool has been configured.

// pluginLoaderFn is the function signature for plugin loading functions
type pluginLoaderFn func(string, *plugin.Plugin) error

//go:embed plugins/*.so
var pluginsDir embed.FS

var (
	pluginLoadLock sync.Mutex
	pluginLoader   pluginLoaderFn = loadPlugin
)

func loadPlugin(name string, holder *plugin.Plugin) error {
	pluginLoadLock.Lock()
	defer pluginLoadLock.Unlock()

	// If the holder for the plugin is not nil then the
	// plugin has already been set so nothing to do.
	if holder != nil {
		return nil
	}

	// The plugin package does not support the FS interface
	// so create a temporary file for loading the plugin.
	// The plugin package doesn't support the FS interface so
	// create a temporary file to stash the shared library.
	dst, err := os.CreateTemp("", "lib-*")
	if err != nil {
		return err
	}
	defer dst.Close()
	defer os.RemoveAll(dst.Name())

	// Open the plugin from within the embedded fs and
	// copy it into the temporary file.
	src, err := pluginsDir.Open(fmt.Sprintf("plugins/%s.so", name))
	if err != nil {
		return err
	}
	defer src.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	dst.Close()

	// Now load the plugin!
	cephPlugin, err = plugin.Open(dst.Name())
	if err != nil {
		return err
	}

	return nil
}

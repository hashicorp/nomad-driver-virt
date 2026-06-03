// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package main

import "os"

func main() {
	println("test error")
	os.Exit(1)
}

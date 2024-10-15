// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package version

var (
	// GitCommit and GitDescribe are filled in by the compiler and describe the
	// git reference information at build time.
	GitCommit   string
	GitDescribe string

	// Version is the main version number that is being run at the moment. It
	// must conform to the format expected by github.com/hashicorp/go-version.
	Version = "0.0.1"

	// VersionPrerelease is a pre-release marker for the version. If this is ""
	// (empty string) then it means that it is a final release. Otherwise, this
	// is a pre-release such as "dev" (in development), "beta.1", "rc1.1", etc.
	VersionPrerelease = "dev"

	// VersionMetadata is metadata further describing the build type.
	VersionMetadata = ""
)

package virt

// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

import (
	"testing"

	"github.com/hashicorp/nomad/helper/pluginutils/hclutils"
	"github.com/shoenig/test/must"
)

func TestConfig_TaskCMDs(t *testing.T) {
	t.Parallel()

	parser := hclutils.NewConfigParser(taskConfigSpec)
	expectedCmds := []string{"redis"}
	expectedImg := "/path/to/image/here"
	validHCL := `
  config {
	image = "/path/to/image/here"
	cmds = ["redis"]
  }
`

	var tc *TaskConfig
	parser.ParseHCL(t, validHCL, &tc)
	must.SliceContainsAll(t, expectedCmds, tc.CMDs)
	must.StrContains(t, expectedImg, tc.ImagePath)
}

/* func TestConfig_Plugin(t *testing.T) {
	t.Parallel()

	parser := hclutils.NewConfigParser(configSpec)
	expectedCmds := []string{"redis"}
	expectedImg := "/path/to/image/here"
	validHCL := `
  config {
	data_dir = "blah"
  }
`

	var tc *TaskConfig
	parser.ParseHCL(t, validHCL, &tc)
	must.SliceContainsAll(t, expectedCmds, tc.CMDs)
	must.StrContains(t, expectedImg, tc.ImagePath)
} */

/*
func TestConfig_Logging(t *testing.T) {
	ci.Parallel(t)

	parser := hclutils.NewConfigParser(taskConfigSpec)
	expectedDriver := "journald"
	expectedTag := "redis"
	validHCL := `
  config {
	  image = "docker://redis"
	  logging = {
			driver = "journald"
			options = [
			  {
				  "tag" = "redis"
			  }
			]
	  }
  }
`

	var tc *TaskConfig
	parser.ParseHCL(t, validHCL, &tc)
	must.Eq(t, expectedDriver, tc.Logging.Driver)
	must.Eq(t, expectedTag, tc.Logging.Options["tag"])
}

func TestConfig_Labels(t *testing.T) {
	ci.Parallel(t)

	parser := hclutils.NewConfigParser(taskConfigSpec)
	validHCL := `
  config {
	  image = "docker://redis"
		labels = {
		  "nomad" = "job"
		 }
  }
`

	var tc *TaskConfig
	parser.ParseHCL(t, validHCL, &tc)
	must.Eq(t, "job", tc.Labels["nomad"])
}

func TestConfig_ForcePull(t *testing.T) {
	ci.Parallel(t)

	parser := hclutils.NewConfigParser(taskConfigSpec)
	validHCL := `
  config {
		image = "docker://redis"
		force_pull = true
  }
`

	var tc *TaskConfig
	parser.ParseHCL(t, validHCL, &tc)
	must.Eq(t, true, tc.ForcePull)
}

func TestConfig_CPUHardLimit(t *testing.T) {
	ci.Parallel(t)

	parser := hclutils.NewConfigParser(taskConfigSpec)
	validHCL := `
  config {
		image = "docker://redis"
		cpu_hard_limit = true
		cpu_cfs_period = 200000
  }
`

	var tc *TaskConfig
	parser.ParseHCL(t, validHCL, &tc)
	must.True(t, tc.CPUHardLimit)
	must.Eq(t, 200000, tc.CPUCFSPeriod)
}

func TestConfig_ImagePullTimeout(t *testing.T) {
	ci.Parallel(t)

	parser := hclutils.NewConfigParser(taskConfigSpec)
	validHCL := `
  config {
		image = "docker://redis"
		image_pull_timeout = "10m"
  }
`

	var tc *TaskConfig
	parser.ParseHCL(t, validHCL, &tc)
	must.Eq(t, "10m", tc.ImagePullTimeout)
}

func TestConfig_ExtraHosts(t *testing.T) {
	ci.Parallel(t)

	parser := hclutils.NewConfigParser(taskConfigSpec)
	validHCL := `
		config {
		image = "docker://redis"
		extra_hosts = ["myhost:127.0.0.2", "example.com:10.0.0.1"]
	}
	`

	var tc *TaskConfig
	parser.ParseHCL(t, validHCL, &tc)
	must.Eq(t, []string{"myhost:127.0.0.2", "example.com:10.0.0.1"}, tc.ExtraHosts)
}
*/

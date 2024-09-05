package metadata

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDriverDomainMetadataXMLString(t *testing.T) {
	type testcase struct {
		in        *DriverDomain
		expect    string
		expectErr bool
	}
	run := func(t *testing.T, tc testcase) {
		// got, err := xml.MarshalIndent(tc.in, "", "  ")
		got, err := tc.in.XMLString()
		if err != nil {
			t.Fatal(err)
		}
		require.Equal(t, tc.expect, string(got))
	}
	cases := map[string]testcase{
		"empty": {
			in:     &DriverDomain{},
			expect: `<hashivirt:instance xmlns:hashivirt="` + hashivirtNamespace + `"></hashivirt:instance>`,
		},
		"no namespace": {
			// alloc: 317e26d8
			in: &DriverDomain{
				Nomad: &DriverDomainNomad{
					Namespace: "",
					Job:       "my-job",
					Alloc:     "317e26d8",
					Task:      "my-task",
				},
			},
			expect: `<hashivirt:instance xmlns:hashivirt="` + hashivirtNamespace + `">
  <hashivirt:nomad>
    <hashivirt:job>my-job</hashivirt:job>
    <hashivirt:alloc>317e26d8</hashivirt:alloc>
    <hashivirt:task>my-task</hashivirt:task>
  </hashivirt:nomad>
</hashivirt:instance>`,
		},
		"with namespace": {
			// alloc: 317e26d8
			in: &DriverDomain{
				Nomad: &DriverDomainNomad{
					Namespace: "foo",
					Job:       "my-job",
					Alloc:     "317e26d8",
					Task:      "my-task",
				},
			},
			expect: `<hashivirt:instance xmlns:hashivirt="` + hashivirtNamespace + `">
  <hashivirt:nomad>
    <hashivirt:namespace>foo</hashivirt:namespace>
    <hashivirt:job>my-job</hashivirt:job>
    <hashivirt:alloc>317e26d8</hashivirt:alloc>
    <hashivirt:task>my-task</hashivirt:task>
  </hashivirt:nomad>
</hashivirt:instance>`,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			run(t, tc)
		})
	}
}

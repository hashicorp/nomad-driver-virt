package metadata

import (
	"encoding/xml"
	"strings"
)

const hashivirtNamespace = "http://hashicorp.com/xmlns/1.0/hashivirt"

type DriverDomain struct {
	XMLName xml.Name           `xml:"http://hashicorp.com/xmlns/1.0/hashivirt hashivirt:instance"`
	Nomad   *DriverDomainNomad `xml:"hashivirt:nomad"`
}

type DriverDomainNomad struct {
	Namespace string `xml:"hashivirt:namespace,omitempty"`
	Job       string `xml:"hashivirt:job"`
	Alloc     string `xml:"hashivirt:alloc"`
	Task      string `xml:"hashivirt:task"`
}

func (m *DriverDomain) XMLString() (string, error) {
	out, err := xml.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	x := string(out)

	// Stdlib cannot create `<foo:thing xmlns:foo="URI">` start elements, so fix it
	x = strings.Replace(x, ` xmlns="`, ` xmlns:hashivirt="`, 1)

	return x, nil
}

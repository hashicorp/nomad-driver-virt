// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package convert

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var decimal = map[string]int{
	"kb": 1000,
	"mb": 1000000,
	"gb": 1000000000,
	"tb": 1000000000000,
	"pb": 1000000000000000,
	"eb": 1000000000000000000,
}

var binary = map[string]int{
	"kib": 1024,
	"mib": 1048576,
	"gib": 1073741824,
	"tib": 1099511627776,
	"pib": 1125899906842624,
	"eib": 1152921504606846976,
}

var pattern = regexp.MustCompile(`^\s*(\d+)( ?([kmgtpe]i?b))?\s*$`)

// ToBytes converts the string into the number of bytes.
func ToBytes(val string) (uint64, error) {
	result := pattern.FindAllStringSubmatch(strings.ToLower(val), -1)
	if result == nil || len(result) != 1 || len(result[0]) != 4 {
		return 0, fmt.Errorf("cannot parse size value %q", val)
	}

	numStr := result[0][1]
	num, err := strconv.ParseUint(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("cannot convert size value %q - %w", val, err)
	}

	// If only a number of bytes was provided, nothing
	// else to do.
	if result[0][3] == "" {
		return num, nil
	}

	dem := result[0][3]

	var multiplier int
	var ok bool
	if strings.Contains(dem, "i") {
		multiplier, ok = binary[dem]
	} else {
		multiplier, ok = decimal[dem]
	}

	if !ok {
		return 0, fmt.Errorf("cannot convert unknown size suffix %q", dem)
	}

	return num * uint64(multiplier), nil
}

// MustToBytes converts the string to the number of bytes. If the
// string cannot be converted, it will panic.
func MustToBytes(val string) uint64 {
	b, err := ToBytes(val)
	if err != nil {
		panic(err)
	}

	return b
}

// ValidBytesString returns if the string can be converted into bytes.
func ValidBytesString(val string) bool {
	_, err := ToBytes(val)
	return err == nil
}

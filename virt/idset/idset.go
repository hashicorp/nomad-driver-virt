// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// Package idset provides a Set implementation for keeping track of various
// types of numeric IDs (e.g. CoreID, ProcessID, etc.).
package idset

import (
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/hashicorp/go-set/v2"
)

// An ID is representative of a non-negative identifier of something like
// a CPU core ID, a NUMA node ID, etc.
//
// See the hwids package for typical use cases.
type ID interface {
	~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uint
}

// A Set contains some IDs.
//
// See the List Format section of
// https://www.man7.org/linux/man-pages/man7/cpuset.7.html
// for more information on the syntax and utility of these sets.
type Set[T ID] struct {
	items *set.Set[T]
}

// Empty creates a fresh Set with no elements.
func Empty[T ID]() *Set[T] {
	return &Set[T]{
		items: set.New[T](0),
	}
}

var (
	numberRe = regexp.MustCompile(`^\d+$`)
	spanRe   = regexp.MustCompile(`^(\d+)-(\d+)$`)
)

func atoi[T ID](s string) T {
	i, _ := strconv.Atoi(s)
	return T(i)
}

func order[T ID](a, b T) (T, T) {
	if a < b {
		return a, b
	}
	return b, a
}

// Parse the given cpuset into a set.
//
// The input is assumed to be valid.
func Parse[T ID](list string) *Set[T] {
	result := Empty[T]()

	add := func(s string) {
		s = strings.TrimSpace(s)
		switch {
		case numberRe.MatchString(s):
			result.items.Insert(atoi[T](s))
		case spanRe.MatchString(s):
			values := spanRe.FindStringSubmatch(s)
			low, high := order(atoi[T](values[1]), atoi[T](values[2]))
			for i := low; i <= high; i++ {
				result.items.Insert(i)
			}
		}
	}

	pieces := strings.Split(list, ",")
	for _, piece := range pieces {
		add(piece)
	}

	return result
}

// Slice returns a slice copy of the Set.
func (s *Set[T]) Slice() []T {
	items := s.items.Slice()
	slices.Sort(items)
	return items
}

// Size returns the number of elements in the Set.
func (s *Set[T]) Size() int {
	return s.items.Size()
}

// Empty returns whether the set is empty.
func (s *Set[T]) Empty() bool {
	if s == nil || s.items == nil {
		return true
	}

	return s.items.Empty()
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package mock

import (
	"errors"
	"fmt"
	"testing"

	"github.com/shoenig/test/must"
)

// MockTestErr is a common error used for testing
var MockTestErr = errors.New("mock testing error")

// MockT provides an implementation of the must.T interface
// which will panic on Fatalf calls. This prevents test
// failures allowing recovery which enables the ability
// to test mock implementations.
func MockT() *mustT {
	return &mustT{}
}

type mustT struct{}

func (m *mustT) Helper() {}
func (m *mustT) Fatalf(msg string, args ...any) {
	panic(fmt.Sprintf(msg, args...))
}

// AssertUnexpectedCall verifies that the function was unexpectedly
// called on a mock.
//
//	func Test_MyMock(t *testing.T) {
//	    m := NewMock(MockT())
//	    defer AssertUnexpectedCall(t, "Add")
//	    m.Add(1, 2)
//	}
func AssertUnexpectedCall(t *testing.T, fnName string) {
	t.Helper()

	r := recover()
	must.NotNil(t, r, must.Sprint("Check that mock was created using MockT()"))
	must.StrContains(t, r.(string), fmt.Sprintf("Unexpected call to %s", fnName))
}

// AssertIncorrectArguments verifies that a function was called
// on a mock with arguments that do not match what was expected.
//
//	func Test_MyMock(t *testing.T) {
//	    m := NewMock(MockT())
//	    m.ExpectAdd(Add{1, 2})
//	    defer AssertIncorrectArguments(t, "Add")
//	    m.Add(3, 4)
//	}
func AssertIncorrectArguments(t *testing.T, fnName string) {
	t.Helper()

	r := recover()
	must.NotNil(t, r, must.Sprint("Check that mock was created using MockT()"))
	must.StrContains(t, r.(string), fmt.Sprintf("%s received incorrect arguments", fnName))
}

// AssertExpectations verifies that a function which was expected
// on a mock was not called.
//
//	func Test_MyMock(t *testing.T) {
//	    m := NewMock(MockT())
//	    m.ExpectAdd(Add{1, 2})
//	    defer AssertExpectations(t, "Add")
//	    m.AssertExpectations()
//	}
func AssertExpectations(t *testing.T, fnName string) {
	t.Helper()

	r := recover()
	must.NotNil(t, r, must.Sprint("Check that mock was created using MockT()"))
	must.StrContains(t, r.(string), fmt.Sprintf("%s expecting", fnName))
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package image_tools

import "github.com/shoenig/test/must"

func NewMockImageHandler(t must.T) *MockImageHandler {
	return &MockImageHandler{t: t}
}

func NewStaticImageHandler() *StaticImageHandler {
	return &StaticImageHandler{}
}

type StaticImageHandler struct {
	GetImageFormatResult string
}

func (s *StaticImageHandler) GetImageFormat(string) (string, error) {
	if s.GetImageFormatResult != "" {
		return s.GetImageFormatResult, nil
	}

	return "raw", nil
}

func (s *StaticImageHandler) CreateCopy(string, string, int64) error {
	return nil
}

func (s *StaticImageHandler) CreateChainedCopy(string, string, int64) error {
	return nil
}

type GetImageFormat struct {
	Path   string
	Result string
	Err    error
}

type CreateCopy struct {
	Src   string
	Dst   string
	SizeM int64
	Err   error
}

type CreateChainedCopy struct {
	Src   string
	Dst   string
	SizeM int64
	Err   error
}

type MockImageHandler struct {
	t must.T

	getImageFormat    []GetImageFormat
	createCopy        []CreateCopy
	createChainedCopy []CreateChainedCopy
}

func (m *MockImageHandler) Expect(calls ...any) *MockImageHandler {
	for _, call := range calls {
		switch c := call.(type) {
		case GetImageFormat:
			m.ExpectGetImageFormat(c)
		case CreateCopy:
			m.ExpectCreateCopy(c)
		case CreateChainedCopy:
			m.ExpectCreateChainedCopy(c)
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockImageHandler) ExpectGetImageFormat(c GetImageFormat) *MockImageHandler {
	m.getImageFormat = append(m.getImageFormat, c)
	return m
}

func (m *MockImageHandler) ExpectCreateCopy(c CreateCopy) *MockImageHandler {
	m.createCopy = append(m.createCopy, c)
	return m
}

func (m *MockImageHandler) ExpectCreateChainedCopy(c CreateChainedCopy) *MockImageHandler {
	m.createChainedCopy = append(m.createChainedCopy, c)
	return m
}

func (m *MockImageHandler) GetImageFormat(path string) (string, error) {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getImageFormat,
		must.Sprint("Unexpected call to GetImageFormat"))
	call := m.getImageFormat[0]
	m.getImageFormat = m.getImageFormat[1:]

	must.Eq(m.t,
		struct{ Path string }{call.Path},
		struct{ Path string }{path},
		must.Sprint("GetImageFormat received incorrect arguments"),
	)

	return call.Result, call.Err
}

func (m *MockImageHandler) CreateCopy(src, dst string, sizeM int64) error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.createCopy,
		must.Sprint("Unexpected call to CreateCopy"))
	call := m.createCopy[0]
	m.createCopy = m.createCopy[1:]

	must.Eq(m.t,
		struct {
			Src, Dst string
			SizeM    int64
		}{call.Src, call.Dst, call.SizeM},
		struct {
			Src, Dst string
			SizeM    int64
		}{src, dst, sizeM},
		must.Sprint("CreateCopy received incorrect arguments"),
	)

	return call.Err
}

func (m *MockImageHandler) CreateChainedCopy(src, dst string, sizeM int64) error {
	m.t.Helper()

	must.SliceNotEmpty(m.t, m.createChainedCopy,
		must.Sprint("Unexpected call to CreateChainedCopy"))
	call := m.createChainedCopy[0]
	m.createChainedCopy = m.createChainedCopy[1:]

	must.Eq(m.t,
		struct {
			Src, Dst string
			SizeM    int64
		}{call.Src, call.Dst, call.SizeM},
		struct {
			Src, Dst string
			SizeM    int64
		}{src, dst, sizeM},
		must.Sprint("CreateChainedCopy received incorrect arguments"),
	)

	return call.Err
}

func (m *MockImageHandler) AssertExpectations() {
	m.t.Helper()

	must.SliceEmpty(m.t, m.getImageFormat,
		must.Sprintf("GetImageFormat expecting %d more invocations", len(m.getImageFormat)))
	must.SliceEmpty(m.t, m.createCopy,
		must.Sprintf("CreateCopy expecting %d more invocations", len(m.createCopy)))
	must.SliceEmpty(m.t, m.createChainedCopy,
		must.Sprintf("CreateChainedCopy expecting %d more invocations", len(m.createChainedCopy)))
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package image_tools

import (
	"runtime"
	"strings"
	"sync"

	"github.com/shoenig/test/must"
)

func NewMockImageHandler(t must.T) *MockImageHandler {
	return &MockImageHandler{t: t}
}

func NewStaticImageHandler() *StaticImageHandler {
	return &StaticImageHandler{}
}

type StaticImageHandler struct {
	GetImageFormatResult string
	GetImageSizeResult   uint64

	counts map[string]int
	m      sync.Mutex
	o      sync.Once
}

func (s *StaticImageHandler) incrCount() {
	s.o.Do(func() {
		s.counts = make(map[string]int)
	})

	ctr, _, _, ok := runtime.Caller(1)
	if !ok {
		panic("unable to get caller information")
	}
	info := runtime.FuncForPC(ctr)
	if info == nil {
		panic("unable to get function information")
	}

	name := info.Name()[strings.LastIndex(info.Name(), ".")+1:]
	s.counts[name]++
}

func (s *StaticImageHandler) CallCount(fnName string) int {
	s.m.Lock()
	defer s.m.Unlock()

	if s.counts == nil {
		return 0
	}

	return s.counts[fnName]
}

func (s *StaticImageHandler) ConvertImage(string, string, string, string) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticImageHandler) GetImageFormat(string) (string, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.GetImageFormatResult != "" {
		return s.GetImageFormatResult, nil
	}

	return "test-format", nil
}

func (s *StaticImageHandler) GetImageSize(string) (uint64, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.GetImageSizeResult, nil
}

func (s *StaticImageHandler) CreateCopy(string, string, int64) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticImageHandler) CreateChainedCopy(string, string, int64) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

type ConvertImage struct {
	Src    string
	SrcFmt string
	Dst    string
	DstFmt string
	Err    error
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

type GetImageSize struct {
	Path   string
	Result uint64
	Err    error
}

type MockImageHandler struct {
	t must.T

	getImageFormat    []GetImageFormat
	convertImage      []ConvertImage
	createCopy        []CreateCopy
	createChainedCopy []CreateChainedCopy
	getImageSize      []GetImageSize
	m                 sync.Mutex
}

func (m *MockImageHandler) Expect(calls ...any) *MockImageHandler {
	for _, call := range calls {
		switch c := call.(type) {
		case GetImageFormat:
			m.ExpectGetImageFormat(c)
		case GetImageSize:
			m.ExpectGetImageSize(c)
		case CreateCopy:
			m.ExpectCreateCopy(c)
		case CreateChainedCopy:
			m.ExpectCreateChainedCopy(c)
		case ConvertImage:
			m.ExpectConvertImage(c)
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockImageHandler) ExpectConvertImage(c ConvertImage) *MockImageHandler {
	m.m.Lock()
	defer m.m.Unlock()

	m.convertImage = append(m.convertImage, c)
	return m
}

func (m *MockImageHandler) ExpectGetImageFormat(c GetImageFormat) *MockImageHandler {
	m.m.Lock()
	defer m.m.Unlock()

	m.getImageFormat = append(m.getImageFormat, c)
	return m
}

func (m *MockImageHandler) ExpectGetImageSize(c GetImageSize) *MockImageHandler {
	m.m.Lock()
	defer m.m.Unlock()

	m.getImageSize = append(m.getImageSize, c)
	return m
}

func (m *MockImageHandler) ExpectCreateCopy(c CreateCopy) *MockImageHandler {
	m.m.Lock()
	defer m.m.Unlock()

	m.createCopy = append(m.createCopy, c)
	return m
}

func (m *MockImageHandler) ExpectCreateChainedCopy(c CreateChainedCopy) *MockImageHandler {
	m.m.Lock()
	defer m.m.Unlock()

	m.createChainedCopy = append(m.createChainedCopy, c)
	return m
}

func (m *MockImageHandler) ConvertImage(src, srcFmt, dst, dstFmt string) error {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.convertImage,
		must.Sprint("Unexpected call to ConvertImage"))
	call := m.convertImage[0]
	m.convertImage = m.convertImage[1:]

	must.Eq(m.t, call, ConvertImage{
		Src: src, SrcFmt: srcFmt, Dst: dst, DstFmt: dstFmt, Err: call.Err})

	return call.Err
}

func (m *MockImageHandler) GetImageFormat(path string) (string, error) {
	m.m.Lock()
	defer m.m.Unlock()

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

func (m *MockImageHandler) GetImageSize(path string) (uint64, error) {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceNotEmpty(m.t, m.getImageSize,
		must.Sprint("Unexpected call to GetImageSize"))
	call := m.getImageSize[0]
	m.getImageSize = m.getImageSize[1:]

	must.Eq(m.t,
		struct{ Path string }{call.Path},
		struct{ Path string }{path},
		must.Sprint("GetImageSize received incorrect arguments"),
	)

	return call.Result, call.Err
}

func (m *MockImageHandler) CreateCopy(src, dst string, sizeM int64) error {
	m.m.Lock()
	defer m.m.Unlock()

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
	m.m.Lock()
	defer m.m.Unlock()

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
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceEmpty(m.t, m.getImageFormat,
		must.Sprintf("GetImageFormat expecting %d more invocations", len(m.getImageFormat)))
	must.SliceEmpty(m.t, m.getImageSize,
		must.Sprintf("GetImageSize expecting %d more invocations", len(m.getImageSize)))
	must.SliceEmpty(m.t, m.createCopy,
		must.Sprintf("CreateCopy expecting %d more invocations", len(m.createCopy)))
	must.SliceEmpty(m.t, m.createChainedCopy,
		must.Sprintf("CreateChainedCopy expecting %d more invocations", len(m.createChainedCopy)))
	must.SliceEmpty(m.t, m.convertImage,
		must.Sprintf("ConvertImage expecting %d more invocations", len(m.convertImage)))
}

// Copyright IBM Corp. 2024, 2025
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	//	"io"
	"runtime"
	"strings"
	"sync"

	"github.com/shoenig/test/must"
)

func NewStaticStream() *StaticStream {
	return &StaticStream{}
}

func NewMockStream(t must.T) *MockStream {
	return &MockStream{t: t}
}

type StaticStream struct {
	ReadResult  *Read
	WriteResult *Write

	counts map[string]int
	m      sync.Mutex
	o      sync.Once
}

func (s *StaticStream) incrCount() {
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

func (s *StaticStream) CallCount(fnName string) int {
	s.m.Lock()
	defer s.m.Unlock()

	if s.counts == nil {
		return 0
	}

	return s.counts[fnName]
}

func (s *StaticStream) Abort() error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticStream) Free() error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticStream) Finish() error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticStream) Read(in []byte) (int, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.ReadResult != nil {
		limit := max(len(s.ReadResult.DataResult), cap(in))
		copy(in, s.ReadResult.DataResult)

		return limit, s.ReadResult.Err
	}

	return 0, nil //io.EOF
}

func (s *StaticStream) Write(data []byte) (int, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.WriteResult != nil {
		return s.WriteResult.Result, s.WriteResult.Err
	}

	return len(data), nil // io.EOF
}

type Abort struct {
	Err error
}

type Finish struct {
	Err error
}

type Read struct {
	DataResult []byte // Bytes expected to be read
	Err        error
}

type Write struct {
	Data   []byte
	Result int
	Err    error
}

type MockStream struct {
	t must.T

	abort  []Abort
	finish []Finish
	free   []Free
	read   []Read
	write  []Write
	m      sync.Mutex
}

func (m *MockStream) Expect(calls ...any) *MockStream {
	for _, call := range calls {
		switch c := call.(type) {
		case Abort:
			m.ExpectAbort(c)
		case Finish:
			m.ExpectFinish(c)
		case Free:
			m.ExpectFree(c)
		case Read:
			m.ExpectRead(c)
		case Write:
			m.ExpectWrite(c)
		default:
			m.t.Fatalf("unsupported type for mock expectation: %T", c)
		}
	}

	return m
}

func (m *MockStream) ExpectAbort(c Abort) *MockStream {
	m.m.Lock()
	defer m.m.Unlock()

	m.abort = append(m.abort, c)
	return m
}

func (m *MockStream) ExpectFree(c Free) *MockStream {
	m.m.Lock()
	defer m.m.Unlock()

	m.free = append(m.free, c)
	return m
}

func (m *MockStream) ExpectFinish(c Finish) *MockStream {
	m.m.Lock()
	defer m.m.Unlock()

	m.finish = append(m.finish, c)
	return m
}

func (m *MockStream) ExpectRead(c Read) *MockStream {
	m.m.Lock()
	defer m.m.Unlock()

	m.read = append(m.read, c)
	return m
}

func (m *MockStream) ExpectWrite(c Write) *MockStream {
	m.m.Lock()
	defer m.m.Unlock()

	m.write = append(m.write, c)
	return m
}

func (m *MockStream) Abort() error {
	m.m.Lock()
	defer m.m.Unlock()

	must.SliceNotEmpty(m.t, m.abort,
		must.Sprint("Unexpected call to Abort"))
	call := m.abort[0]
	m.abort = m.abort[1:]

	return call.Err
}

func (m *MockStream) Finish() error {
	m.m.Lock()
	defer m.m.Unlock()

	must.SliceNotEmpty(m.t, m.finish,
		must.Sprint("Unexpected call to Finish"))
	call := m.finish[0]
	m.finish = m.finish[1:]

	return call.Err
}

func (m *MockStream) Free() error {
	m.m.Lock()
	defer m.m.Unlock()

	must.SliceNotEmpty(m.t, m.free,
		must.Sprint("Unexpected call to Free"))
	call := m.free[0]
	m.free = m.free[1:]

	return call.Err
}

func (m *MockStream) Read(data []byte) (int, error) {
	m.m.Lock()
	defer m.m.Unlock()

	must.SliceNotEmpty(m.t, m.read,
		must.Sprint("Unexpected call to Read"))
	call := m.read[0]
	limit := max(cap(data), len(call.DataResult))
	for i := range limit {
		data[i] = call.DataResult[i]
	}

	return limit, call.Err
}

func (m *MockStream) Write(data []byte) (int, error) {
	m.m.Lock()
	defer m.m.Unlock()

	must.SliceNotEmpty(m.t, m.write,
		must.Sprint("Unexpected call to Write"))
	call := m.write[0]
	m.write = m.write[1:]

	must.Eq(m.t, struct{ Data []byte }{call.Data}, struct{ Data []byte }{data},
		must.Sprint("Write received incorrect argument"))

	return call.Result, call.Err
}

func (m *MockStream) AssertExpectations() {
	m.m.Lock()
	defer m.m.Unlock()

	m.t.Helper()

	must.SliceEmpty(m.t, m.abort,
		must.Sprintf("Abort expecting %d more invocations", len(m.abort)))
	must.SliceEmpty(m.t, m.finish,
		must.Sprintf("Finish expecting %d more invocations", len(m.finish)))
	must.SliceEmpty(m.t, m.free,
		must.Sprintf("Free expecting %d more invocations", len(m.free)))
	must.SliceEmpty(m.t, m.read,
		must.Sprintf("Read expecting %d more invocations", len(m.read)))
	must.SliceEmpty(m.t, m.write,
		must.Sprintf("Write expecting %d more invocations", len(m.write)))
}

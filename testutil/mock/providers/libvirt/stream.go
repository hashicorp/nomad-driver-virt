// Copyright IBM Corp. 2024, 2026
// SPDX-License-Identifier: MPL-2.0

package libvirt

import (
	"runtime"
	"strings"
	"sync"

	"github.com/shoenig/test/must"
	"libvirt.org/go/libvirt"
)

func NewStaticStream() *StaticStream {
	return &StaticStream{}
}

func NewMockStream(t must.T) *MockStream {
	return &MockStream{t: t}
}

type StaticStream struct {
	ReadResult      *Read
	RecvResult      *Recv
	SendResult      *Send
	WriteResult     *Write
	SparseResult    bool
	RawStreamResult *libvirt.Stream

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

func (s *StaticStream) RawStream() (*libvirt.Stream, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.RawStreamResult, nil
}

func (s *StaticStream) Read(in []byte) (int, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.ReadResult != nil {
		limit := max(len(s.ReadResult.Data), cap(in))
		copy(in, s.ReadResult.Data)

		return limit, s.ReadResult.Err
	}

	return 0, nil
}

func (s *StaticStream) Recv(in []byte) (int, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.RecvResult != nil {
		limit := max(len(s.RecvResult.Data), cap(in))
		copy(in, s.RecvResult.Data)

		return limit, s.RecvResult.Err
	}

	return 0, nil
}

func (s *StaticStream) Send(data []byte) (int, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.SendResult != nil {
		return s.SendResult.Result, s.SendResult.Err
	}

	return len(data), nil
}

func (s *StaticStream) SendHole(int64, uint32) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return nil
}

func (s *StaticStream) Write(data []byte) (int, error) {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	if s.WriteResult != nil {
		return s.WriteResult.Result, s.WriteResult.Err
	}

	return len(data), nil
}

func (s *StaticStream) Sparse() bool {
	s.m.Lock()
	defer s.m.Unlock()
	s.incrCount()

	return s.SparseResult
}

type Abort struct {
	Err error
}

type Finish struct {
	Err error
}

type RawStream struct {
	Result *libvirt.Stream
	Err    error
}

type Read struct {
	Data   []byte
	Result int
	Err    error
}

type Recv struct {
	Data   []byte
	Result int
	Err    error
}

type Send struct {
	Data   []byte
	Result int
	Err    error
}

type SendHole struct {
	Size  int64
	Flags uint32
	Err   error
}

type Write struct {
	Data   []byte
	Result int
	Err    error
}

type Sparse struct {
	Result bool
}

type MockStream struct {
	t must.T

	abort     []Abort
	finish    []Finish
	free      []Free
	rawStream []RawStream
	read      []Read
	recv      []Recv
	send      []Send
	sendHole  []SendHole
	sparse    []Sparse
	write     []Write
	m         sync.Mutex
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
		case RawStream:
			m.ExpectRawStream(c)
		case Read:
			m.ExpectRead(c)
		case Recv:
			m.ExpectRecv(c)
		case Send:
			m.ExpectSend(c)
		case SendHole:
			m.ExpectSendHole(c)
		case Sparse:
			m.ExpectSparse(c)
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

func (m *MockStream) ExpectRawStream(c RawStream) *MockStream {
	m.m.Lock()
	defer m.m.Unlock()

	m.rawStream = append(m.rawStream, c)
	return m
}

func (m *MockStream) ExpectRead(c Read) *MockStream {
	m.m.Lock()
	defer m.m.Unlock()

	m.read = append(m.read, c)
	return m
}

func (m *MockStream) ExpectRecv(c Recv) *MockStream {
	m.m.Lock()
	defer m.m.Unlock()

	m.recv = append(m.recv, c)
	return m
}

func (m *MockStream) ExpectSend(c Send) *MockStream {
	m.m.Lock()
	defer m.m.Unlock()

	m.send = append(m.send, c)
	return m
}

func (m *MockStream) ExpectSendHole(c SendHole) *MockStream {
	m.m.Lock()
	defer m.m.Unlock()

	m.sendHole = append(m.sendHole, c)
	return m
}

func (m *MockStream) ExpectSparse(c Sparse) *MockStream {
	m.m.Lock()
	defer m.m.Unlock()

	m.sparse = append(m.sparse, c)
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

func (m *MockStream) RawStream() (*libvirt.Stream, error) {
	m.m.Lock()
	defer m.m.Unlock()

	must.SliceNotEmpty(m.t, m.rawStream,
		must.Sprint("Unexpected call to RawStream"))
	call := m.rawStream[0]
	m.rawStream = m.rawStream[1:]

	return call.Result, call.Err
}

func (m *MockStream) Read(data []byte) (int, error) {
	m.m.Lock()
	defer m.m.Unlock()

	must.SliceNotEmpty(m.t, m.read,
		must.Sprint("Unexpected call to Read"))
	call := m.read[0]
	m.read = m.read[1:]

	if call.Data != nil {
		must.Eq(m.t, call.Data, data,
			must.Sprint("Read received incorrect argument"))
	}

	result := call.Result
	if result < 0 {
		result = len(data)
	}

	return result, call.Err
}

func (m *MockStream) Recv(data []byte) (int, error) {
	m.m.Lock()
	defer m.m.Unlock()

	must.SliceNotEmpty(m.t, m.recv,
		must.Sprint("Unexpected call to Recv"))
	call := m.recv[0]
	m.recv = m.recv[1:]

	if call.Data != nil {
		must.Eq(m.t, call.Data, data,
			must.Sprint("Recv received incorrect argument"))
	}

	result := call.Result
	if result < 0 {
		result = len(data)
	}

	return result, call.Err
}

func (m *MockStream) SendHole(size int64, flags uint32) error {
	m.m.Lock()
	defer m.m.Unlock()

	must.SliceNotEmpty(m.t, m.sendHole,
		must.Sprint("Unexpected call to SendHole"))
	call := m.sendHole[0]
	m.sendHole = m.sendHole[1:]

	must.Eq(m.t, call, SendHole{Size: size, Flags: flags, Err: call.Err},
		must.Sprint("SendHole received incorrect argument"))

	return call.Err
}

func (m *MockStream) Sparse() bool {
	m.m.Lock()
	defer m.m.Unlock()

	must.SliceNotEmpty(m.t, m.sparse,
		must.Sprint("Unexpected call to Sparse"))
	call := m.sparse[0]
	m.sparse = m.sparse[1:]

	return call.Result
}

func (m *MockStream) Send(data []byte) (int, error) {
	m.m.Lock()
	defer m.m.Unlock()

	must.SliceNotEmpty(m.t, m.send,
		must.Sprint("Unexpected call to Send"))
	call := m.send[0]
	m.send = m.send[1:]

	// Only check data if provided.
	if call.Data != nil {
		must.Eq(m.t, struct{ Data []byte }{call.Data}, struct{ Data []byte }{data},
			must.Sprint("Send received incorrect argument"))
	}

	result := call.Result
	if result < 0 {
		result = len(data)
	}

	return result, call.Err
}

func (m *MockStream) Write(data []byte) (int, error) {
	m.m.Lock()
	defer m.m.Unlock()

	must.SliceNotEmpty(m.t, m.write,
		must.Sprint("Unexpected call to Write"))
	call := m.write[0]
	m.write = m.write[1:]

	// Only check data if provided.
	if call.Data != nil {
		must.Eq(m.t, struct{ Data []byte }{call.Data}, struct{ Data []byte }{data},
			must.Sprint("Write received incorrect argument"))
	}

	result := call.Result
	if result < 0 {
		result = len(data)
	}

	return result, call.Err
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
	must.SliceEmpty(m.t, m.rawStream,
		must.Sprintf("RawStream expecting %d more invocations", len(m.rawStream)))
	must.SliceEmpty(m.t, m.read,
		must.Sprintf("Read expecting %d more invocations", len(m.read)))
	must.SliceEmpty(m.t, m.recv,
		must.Sprintf("Recv expecting %d more invocations", len(m.recv)))
	must.SliceEmpty(m.t, m.sparse,
		must.Sprintf("Sparse expecting %d more invocations", len(m.sparse)))
	must.SliceEmpty(m.t, m.send,
		must.Sprintf("Send expecting %d more invocations", len(m.send)))
	must.SliceEmpty(m.t, m.sendHole,
		must.Sprintf("SendHole expecting %d more invocations", len(m.sendHole)))
	must.SliceEmpty(m.t, m.write,
		must.Sprintf("Write expecting %d more invocations", len(m.write)))
}

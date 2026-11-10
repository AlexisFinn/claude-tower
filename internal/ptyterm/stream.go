package ptyterm

import (
	"encoding/base64"
	"sync"
	"time"
)

const (
	// maxPacket is the largest number of bytes sent in one data event.
	maxPacket = 16 * 1024
	// flushInterval is the shortest time between two flushes.
	flushInterval = 16 * time.Millisecond
	// maxHeld is the output kept while the frontend is not ready: once the held
	// bytes exceed twice this size, only the most recent maxHeld bytes remain.
	// Trimming in batches keeps the copy cost proportional to the output.
	maxHeld = 4 << 20
)

// stream carries the output of a terminal to the frontend. Bytes are held
// until the frontend is ready to receive them, then sent in packets. The end
// of the output is announced after the last byte.
type stream struct {
	dataEvent, exitEvent string
	emit                 Emitter

	mu      sync.Mutex
	pending []byte
	ready   bool
	eof     bool
	wake    chan struct{}
}

func newStream(emit Emitter, dataEvent, exitEvent string) *stream {
	return &stream{dataEvent: dataEvent, exitEvent: exitEvent, emit: emit, wake: make(chan struct{}, 1)}
}

func (s *stream) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// write queues bytes for the frontend. Before the frontend is ready, the held
// bytes are trimmed to the most recent maxHeld once they exceed twice that.
func (s *stream) write(p []byte) {
	s.mu.Lock()
	s.pending = append(s.pending, p...)
	if !s.ready && len(s.pending) > 2*maxHeld {
		s.pending = append([]byte(nil), s.pending[len(s.pending)-maxHeld:]...)
	}
	s.mu.Unlock()
	s.signal()
}

// finish records that no more bytes will be written.
func (s *stream) finish() {
	s.mu.Lock()
	s.eof = true
	s.mu.Unlock()
	s.signal()
}

// setReady lets the queued bytes, and the following ones, reach the frontend.
func (s *stream) setReady() {
	s.mu.Lock()
	s.ready = true
	s.mu.Unlock()
	s.signal()
}

// flush sends the queued bytes in packets, then the exit event when the
// output is over. It reports whether the exit event was sent.
func (s *stream) flush() bool {
	s.mu.Lock()
	if !s.ready {
		s.mu.Unlock()
		return false
	}
	data, eof := s.pending, s.eof
	s.pending = nil
	s.mu.Unlock()
	for len(data) > 0 {
		n := min(len(data), maxPacket)
		s.emit.Emit(s.dataEvent, base64.StdEncoding.EncodeToString(data[:n]))
		data = data[n:]
	}
	if eof {
		s.emit.Emit(s.exitEvent, nil)
	}
	return eof
}

// run flushes on every signal, at most once per flushInterval, until the exit
// event is sent or stop is closed.
func (s *stream) run(stop <-chan struct{}) {
	timer := time.NewTimer(flushInterval)
	timer.Stop()
	defer timer.Stop()
	for {
		select {
		case <-s.wake:
		case <-stop:
			return
		}
		if s.flush() {
			return
		}
		timer.Reset(flushInterval)
		select {
		case <-timer.C:
		case <-stop:
			return
		}
	}
}

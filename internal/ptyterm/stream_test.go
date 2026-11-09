package ptyterm

import (
	"bytes"
	"encoding/base64"
	"testing"
	"time"
)

func TestStreamHoldsUntilReadyThenSendsPackets(t *testing.T) {
	rec := &recorder{}
	s := newStream(rec, "d", "x")
	s.write(bytes.Repeat([]byte("a"), 2*maxPacket+10))
	if s.flush() || len(rec.snapshot()) != 0 {
		t.Fatal("flushed before ready")
	}
	s.setReady()
	if s.flush() {
		t.Fatal("exit emitted without end of output")
	}
	ev := rec.snapshot()
	if len(ev) != 3 {
		t.Fatalf("got %d packets", len(ev))
	}
	sizes := []int{maxPacket, maxPacket, 10}
	for i, e := range ev {
		b, err := base64.StdEncoding.DecodeString(e.data.(string))
		if err != nil || len(b) != sizes[i] || e.name != "d" {
			t.Errorf("packet %d: %d bytes, name %q, err %v", i, len(b), e.name, err)
		}
	}
}

func TestStreamExitFollowsRemainingOutput(t *testing.T) {
	rec := &recorder{}
	s := newStream(rec, "d", "x")
	s.write([]byte("tail"))
	s.finish()
	s.setReady()
	if !s.flush() {
		t.Fatal("exit not emitted")
	}
	ev := rec.snapshot()
	if len(ev) != 2 || ev[0].name != "d" || ev[1].name != "x" {
		t.Fatalf("events %v", ev)
	}
}

func TestStreamRunStopsOnStop(t *testing.T) {
	rec := &recorder{}
	s := newStream(rec, "d", "x")
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { s.run(stop); close(done) }()
	s.setReady()
	s.write([]byte("hi"))
	waitFor(t, "packet", func() bool { return len(rec.snapshot()) == 1 })
	close(stop)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not stop")
	}
}

func TestStreamKeepsMostRecentBytesBeforeReady(t *testing.T) {
	rec := &recorder{}
	s := newStream(rec, "d", "x")
	s.write(bytes.Repeat([]byte("a"), 2*maxHeld))
	s.mu.Lock()
	n := len(s.pending)
	s.mu.Unlock()
	if n != 2*maxHeld {
		t.Fatalf("trimmed below twice the limit: held %d bytes", n)
	}
	s.write([]byte("tail"))
	s.mu.Lock()
	n, end := len(s.pending), string(s.pending[len(s.pending)-4:])
	s.mu.Unlock()
	if n != maxHeld || end != "tail" {
		t.Fatalf("held %d bytes ending with %q", n, end)
	}
	s.setReady()
	s.write(bytes.Repeat([]byte("b"), maxHeld+1))
	s.mu.Lock()
	n = len(s.pending)
	s.mu.Unlock()
	if n != maxHeld+1+maxHeld {
		t.Errorf("bytes after ready were trimmed: %d", n)
	}
}

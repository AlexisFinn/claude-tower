package main

import (
	"context"
	"sync"
	"testing"
	"time"
)

// testWindow is a window whose events are counted and whose quits are sent
// on the returned channel instead of to Wails.
func testWindow(t *testing.T, ackWait time.Duration) (*window, func() int, chan struct{}) {
	t.Helper()
	var mu sync.Mutex
	emitted := 0
	quits := make(chan struct{}, 4)
	w := newWindow()
	w.ackWait = ackWait
	w.emit = func(context.Context, string) { mu.Lock(); emitted++; mu.Unlock() }
	w.quit = func(context.Context) { quits <- struct{}{} }
	w.setContext(context.Background())
	return w, func() int { mu.Lock(); defer mu.Unlock(); return emitted }, quits
}

func TestCloseWithoutGuardClosesAtOnce(t *testing.T) {
	w, emitted, _ := testWindow(t, time.Hour)
	if w.beforeClose(context.Background()) {
		t.Error("close prevented without a frontend to confirm it")
	}
	if e := emitted(); e != 0 {
		t.Errorf("%d events", e)
	}
}

func TestGuardedCloseAsksTheFrontend(t *testing.T) {
	w, emitted, quits := testWindow(t, 10*time.Millisecond)
	w.GuardQuit(true)
	if !w.beforeClose(context.Background()) {
		t.Fatal("close not prevented")
	}
	w.AckQuit()
	if e := emitted(); e != 1 {
		t.Errorf("%d events", e)
	}
	// The timer has run long before this: an answered request never quits.
	select {
	case <-quits:
		t.Error("answered close request quit")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestUnansweredCloseRequestQuits(t *testing.T) {
	w, _, quits := testWindow(t, 10*time.Millisecond)
	w.GuardQuit(true)
	w.beforeClose(context.Background())
	select {
	case <-quits:
	case <-time.After(5 * time.Second):
		t.Fatal("unanswered close request did not quit")
	}
	if w.beforeClose(context.Background()) {
		t.Error("close prevented after the quit")
	}
}

func TestConfirmedQuitIsNotAskedAgain(t *testing.T) {
	w, emitted, quits := testWindow(t, time.Hour)
	w.GuardQuit(true)
	w.Quit()
	if len(quits) != 1 {
		t.Errorf("%d quits", len(quits))
	}
	if w.beforeClose(context.Background()) {
		t.Error("confirmed quit prevented")
	}
	if e := emitted(); e != 0 {
		t.Errorf("%d events", e)
	}
}

func TestUnguardedCloseClosesAtOnce(t *testing.T) {
	w, _, _ := testWindow(t, time.Hour)
	w.GuardQuit(true)
	w.GuardQuit(false)
	if w.beforeClose(context.Background()) {
		t.Error("close prevented after the frontend left")
	}
}

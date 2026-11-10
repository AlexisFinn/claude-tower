package main

import (
	"context"
	"sync"
	"testing"
	"time"
)

// testWindow is a window whose events and quits are recorded instead of sent to Wails.
func testWindow(t *testing.T) (*window, func() (emitted, quits int)) {
	t.Helper()
	var mu sync.Mutex
	emitted, quits := 0, 0
	w := newWindow()
	w.ackWait = 20 * time.Millisecond
	w.emit = func(context.Context, string) { mu.Lock(); emitted++; mu.Unlock() }
	w.quit = func(context.Context) { mu.Lock(); quits++; mu.Unlock() }
	w.setContext(context.Background())
	return w, func() (int, int) { mu.Lock(); defer mu.Unlock(); return emitted, quits }
}

func TestCloseWithoutGuardClosesAtOnce(t *testing.T) {
	w, counts := testWindow(t)
	if w.beforeClose(context.Background()) {
		t.Error("close prevented without a frontend to confirm it")
	}
	if e, _ := counts(); e != 0 {
		t.Errorf("%d events", e)
	}
}

func TestGuardedCloseAsksTheFrontend(t *testing.T) {
	w, counts := testWindow(t)
	w.GuardQuit(true)
	if !w.beforeClose(context.Background()) {
		t.Fatal("close not prevented")
	}
	w.AckQuit()
	time.Sleep(60 * time.Millisecond)
	if e, q := counts(); e != 1 || q != 0 {
		t.Errorf("events=%d quits=%d", e, q)
	}
}

func TestUnansweredCloseRequestQuits(t *testing.T) {
	w, counts := testWindow(t)
	w.GuardQuit(true)
	w.beforeClose(context.Background())
	time.Sleep(60 * time.Millisecond)
	if _, q := counts(); q != 1 {
		t.Errorf("quits=%d", q)
	}
	if w.beforeClose(context.Background()) {
		t.Error("close prevented after the quit")
	}
}

func TestConfirmedQuitIsNotAskedAgain(t *testing.T) {
	w, counts := testWindow(t)
	w.GuardQuit(true)
	w.Quit()
	if w.beforeClose(context.Background()) {
		t.Error("confirmed quit prevented")
	}
	if e, q := counts(); e != 0 || q != 1 {
		t.Errorf("events=%d quits=%d", e, q)
	}
}

func TestUnguardedCloseClosesAtOnce(t *testing.T) {
	w, _ := testWindow(t)
	w.GuardQuit(true)
	w.GuardQuit(false)
	if w.beforeClose(context.Background()) {
		t.Error("close prevented after the frontend left")
	}
}

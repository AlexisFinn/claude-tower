package main

import (
	"context"
	"sync"
	"testing"
	"time"
)

// testWindow is a window whose events are counted, whose quits are counted
// instead of sent to Wails, and whose close timers run when fire is called.
type testWindow struct {
	*window
	mu     sync.Mutex
	events int
	quits  int
	delays []time.Duration
	timers []func()
}

func newTestWindow() *testWindow {
	w := &testWindow{window: newWindow()}
	w.after = func(d time.Duration, f func()) {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.delays = append(w.delays, d)
		w.timers = append(w.timers, f)
	}
	w.emit = func(context.Context, string) { w.mu.Lock(); w.events++; w.mu.Unlock() }
	w.quit = func(context.Context) { w.mu.Lock(); w.quits++; w.mu.Unlock() }
	w.setContext(context.Background())
	return w
}

// fire runs the pending close timers.
func (w *testWindow) fire() {
	w.mu.Lock()
	pending := w.timers
	w.timers = nil
	w.mu.Unlock()
	for _, f := range pending {
		f()
	}
}

func (w *testWindow) counts() (events, quits int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.events, w.quits
}

func TestCloseWithoutGuardClosesAtOnce(t *testing.T) {
	w := newTestWindow()
	if w.beforeClose(context.Background()) {
		t.Error("close prevented without a frontend to confirm it")
	}
	if events, _ := w.counts(); events != 0 {
		t.Errorf("%d events", events)
	}
}

func TestGuardedCloseAsksTheFrontend(t *testing.T) {
	w := newTestWindow()
	w.GuardQuit(true)
	if !w.beforeClose(context.Background()) {
		t.Fatal("close not prevented")
	}
	w.AckQuit()
	if events, _ := w.counts(); events != 1 {
		t.Errorf("%d events", events)
	}
	if len(w.delays) != 1 || w.delays[0] != closeAckWait {
		t.Errorf("close timers: %v", w.delays)
	}
	w.fire()
	if _, quits := w.counts(); quits != 0 {
		t.Error("answered close request quit")
	}
}

func TestUnansweredCloseRequestQuits(t *testing.T) {
	w := newTestWindow()
	w.GuardQuit(true)
	w.beforeClose(context.Background())
	w.fire()
	if _, quits := w.counts(); quits != 1 {
		t.Fatalf("%d quits after an unanswered close request", quits)
	}
	if w.beforeClose(context.Background()) {
		t.Error("close prevented after the quit")
	}
}

func TestConfirmedQuitIsNotAskedAgain(t *testing.T) {
	w := newTestWindow()
	w.GuardQuit(true)
	w.Quit()
	if _, quits := w.counts(); quits != 1 {
		t.Errorf("%d quits", quits)
	}
	if w.beforeClose(context.Background()) {
		t.Error("confirmed quit prevented")
	}
	if events, _ := w.counts(); events != 0 {
		t.Errorf("%d events", events)
	}
}

func TestUnguardedCloseClosesAtOnce(t *testing.T) {
	w := newTestWindow()
	w.GuardQuit(true)
	w.GuardQuit(false)
	if w.beforeClose(context.Background()) {
		t.Error("close prevented after the frontend left")
	}
}

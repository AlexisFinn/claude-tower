package core

import (
	"context"
	"reflect"
	"testing"

	"claude-tower/internal/locate"
)

// fitEnv has a session in the user's tmux (PID 100) and one in the private server (PID 200).
func fitEnv(t *testing.T) (*env, *Core) {
	t.Helper()
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"idle"`)
	e.session(200, `"sessionId":"`+idB+`","startedAt":2000,"status":"idle"`)
	e.tree = locate.Tree{100: 10, 10: 1, 200: 20, 20: 1}
	e.fake.panes[e.userSock] = paneLine("%1", 10, "$1", "a", "@1", "")
	e.fake.panes[e.privSock] = paneLine("%2", 20, "$7", "b", "@7", "")
	c := e.core()
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	return e, c
}

func resizeCalls(e *env, sock string) [][]string {
	var out [][]string
	for _, call := range e.fake.commands(sock) {
		if len(call) > 0 && call[0] == "resize-window" {
			out = append(out, call)
		}
	}
	return out
}

func TestFitResizesPrivateWindowAndRestoresItsSizeOption(t *testing.T) {
	e, c := fitEnv(t)
	if ok, err := c.Fit(context.Background(), 200, 120, 40); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	want := [][]string{{"resize-window", "-t", "@7", "-x", "120", "-y", "40", ";", "set-option", "-wu", "-t", "@7", "window-size"}}
	if got := resizeCalls(e, e.privSock); !reflect.DeepEqual(got, want) {
		t.Errorf("resize calls: %v", got)
	}
}

func TestFitLeavesTheUsersTmuxAlone(t *testing.T) {
	e, c := fitEnv(t)
	if _, err := c.Fit(context.Background(), 100, 120, 40); err == nil {
		t.Error("session of the user's tmux resized")
	}
	if got := resizeCalls(e, e.userSock); got != nil {
		t.Errorf("resize sent to the user's server: %v", got)
	}
}

func TestFitKeepsTheSizeOfAnAttachedClient(t *testing.T) {
	e, c := fitEnv(t)
	// The client attaches after the snapshot: Fit reads the clients again.
	e.fake.clients[e.privSock] = clientLine("/dev/pts/4", 700, "$7", "b", "", 1)
	if ok, err := c.Fit(context.Background(), 200, 120, 40); err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got := resizeCalls(e, e.privSock); got != nil {
		t.Errorf("attached window resized: %v", got)
	}
}

func TestFitSkipsAWindowAlreadyAtThatSize(t *testing.T) {
	e, c := fitEnv(t)
	if ok, err := c.Fit(context.Background(), 200, 80, 24); err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got := resizeCalls(e, e.privSock); got != nil {
		t.Errorf("resize of a window at the right size: %v", got)
	}
}

func TestFitRefusesUnusableSizes(t *testing.T) {
	e, c := fitEnv(t)
	for _, size := range [][2]int{{59, 40}, {120, 11}, {1001, 40}, {120, 1001}} {
		if _, err := c.Fit(context.Background(), 200, size[0], size[1]); err == nil {
			t.Errorf("size %v accepted", size)
		}
	}
	if got := resizeCalls(e, e.privSock); got != nil {
		t.Errorf("resize calls: %v", got)
	}
}

func TestFitKeepsTheSizeOfTheFullscreenView(t *testing.T) {
	e, c := fitEnv(t)
	// The view session exists, attached or not: its client is not among the clients the snapshot counts.
	e.fake.listing[e.privSock] = "$7\tb\tb\n$9\tclaude-tower-view-1-1\tb\n"
	if ok, err := c.Fit(context.Background(), 200, 120, 40); err != nil || ok {
		t.Fatalf("before the attach: ok=%v err=%v", ok, err)
	}
	e.fake.clients[e.privSock] = clientLine("/dev/pts/5", 800, "$9", "claude-tower-view-1-1", "b", 1)
	if ok, err := c.Fit(context.Background(), 200, 120, 40); err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got := resizeCalls(e, e.privSock); got != nil {
		t.Errorf("window of the fullscreen view resized: %v", got)
	}
}

func TestFitLeavesBackgroundAttachSessionsAlone(t *testing.T) {
	e, c := fitEnv(t)
	e.fake.panes[e.privSock] = paneLine("%2", 20, "$7", "claude-tower-bg-bbbbbbbb", "@7", "")
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Fit(context.Background(), 200, 120, 40); err == nil {
		t.Error("attach session resized")
	}
	if got := resizeCalls(e, e.privSock); got != nil {
		t.Errorf("resize calls: %v", got)
	}
}

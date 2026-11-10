package core

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"claude-tower/internal/locate"
	"claude-tower/internal/tmux"
)

func TestPendingPaneGetsStartingCard(t *testing.T) {
	e := newEnv(t)
	e.fake.panes[e.privSock] = paneLineAt("%5", 77, "$3", "proj", "@3", "", testNow.Add(-2*time.Second))
	c := e.core()
	cards, err := c.Cards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 {
		t.Fatalf("cards: %+v", cards)
	}
	p := cards[0]
	if p.PID != 77 || p.Name != "proj" || p.State != "busy" || p.Label != "démarrage" || !p.InTmux || !p.Private ||
		p.CanPrompt || p.PromptReason != "la session démarre" || p.Cwd != "~/proj" || p.Since == "" {
		t.Errorf("starting card: %+v", p)
	}
	if pane, ok := c.PaneOf(77); !ok || pane.ID != "%5" {
		t.Errorf("pane of the starting session: %+v %v", pane, ok)
	}

	e.fake.panes[e.privSock] = paneLineAt("%5", 77, "$3", "proj", "@3", "", testNow.Add(-startupGrace))
	cards, _ = c.Cards(context.Background())
	if p := cards[0]; p.State != "waiting" || p.Label != "attend (démarrage)" {
		t.Errorf("blocked start: %+v", p)
	}
}

func TestPendingCardsComeAfterRegisteredOnes(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","name":"alpha","startedAt":1000,"status":"idle"`)
	e.tree = locate.Tree{100: 10, 10: 1}
	e.fake.panes[e.privSock] = paneLineAt("%6", 88, "$4", "late", "@4", "", testNow.Add(-time.Second)) +
		paneLine("%2", 10, "$7", "alpha", "@7", "") +
		paneLineAt("%5", 77, "$3", "early", "@3", "", testNow.Add(-2*time.Second))
	e.fake.panes[e.userSock] = paneLine("%1", 30, "$1", "shell", "@1", "")
	cards, err := e.core().Cards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var pids, orders []int
	for _, c := range cards {
		pids = append(pids, c.PID)
		orders = append(orders, c.Order)
	}
	if !reflect.DeepEqual(pids, []int{100, 77, 88}) || !reflect.DeepEqual(orders, []int{0, 1, 2}) {
		t.Errorf("pids=%v orders=%v (panes of the user's servers never get a card)", pids, orders)
	}
}

func TestPromptRefusedWhileStarting(t *testing.T) {
	e := newEnv(t)
	e.fake.panes[e.privSock] = paneLine("%5", 77, "$3", "proj", "@3", "")
	c := e.core()
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Prompt(context.Background(), 77, "hello"); err == nil || !strings.Contains(err.Error(), "démarre") {
		t.Errorf("prompt to a starting session: %v", err)
	}
	if e.fake.has(e.privSock, "load-buffer") {
		t.Error("text pasted")
	}
}

func TestStartFailureReportsLastLines(t *testing.T) {
	e := newEnv(t)
	prepareClaude(t, e)
	e.fake.panes[e.privSock] = paneLine("%5", 77, "$3", "proj", "@3", "")
	e.fake.capture["%5"] = "Welcome\n\x1b[31mError: config missing\x1b[0m\n\n"
	c := e.core()
	if err := c.NewSession(context.Background(), "/home/u/proj"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Screen(context.Background(), 77, 0); err != nil {
		t.Fatal(err)
	}
	e.fake.capture["%5"] = "\n\n"
	if _, err := c.Screen(context.Background(), 77, 0); err != nil {
		t.Fatal(err)
	}
	if got := c.StartFailures(); len(got) != 0 {
		t.Fatalf("failure while the pane runs: %+v", got)
	}
	e.fake.panes[e.privSock] = ""
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []StartFailure{{Name: "proj", Cwd: "~/proj", Lines: []string{"Welcome", "Error: config missing"}}}
	if got := c.StartFailures(); !reflect.DeepEqual(got, want) {
		t.Errorf("failures %+v want %+v", got, want)
	}
	if got := c.StartFailures(); len(got) != 0 {
		t.Errorf("failures reported twice: %+v", got)
	}
}

func TestStartFailureWhenPrivateServerStops(t *testing.T) {
	e := newEnv(t)
	prepareClaude(t, e)
	e.fake.panes[e.privSock] = paneLine("%5", 77, "$3", "proj", "@3", "")
	c := e.core()
	if err := c.NewSession(context.Background(), "/home/u/proj"); err != nil {
		t.Fatal(err)
	}
	e.fake.failing[e.privSock] = true
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := c.StartFailures(); len(got) != 1 || got[0].Name != "proj" || len(got[0].Lines) != 0 {
		t.Errorf("failures: %+v", got)
	}
}

func TestRegisteredLaunchIsNotAFailure(t *testing.T) {
	e := newEnv(t)
	prepareClaude(t, e)
	e.fake.panes[e.privSock] = paneLine("%5", 77, "$3", "proj", "@3", "")
	c := e.core()
	if err := c.NewSession(context.Background(), "/home/u/proj"); err != nil {
		t.Fatal(err)
	}
	e.session(77, `"sessionId":"`+idA+`","name":"proj","startedAt":1000,"status":"idle"`)
	e.tree = locate.Tree{77: 1}
	if cards, err := c.Cards(context.Background()); err != nil || len(cards) != 1 || cards[0].State != "idle" {
		t.Fatalf("cards %+v err %v", cards, err)
	}
	e.fake.panes[e.privSock] = ""
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := c.StartFailures(); len(got) != 0 {
		t.Errorf("registered session reported as a failed start: %+v", got)
	}
}

func TestKilledLaunchIsNotAFailure(t *testing.T) {
	e := newEnv(t)
	prepareClaude(t, e)
	e.fake.panes[e.privSock] = paneLine("%5", 77, "$3", "proj", "@3", "")
	c := e.core()
	if err := c.NewSession(context.Background(), "/home/u/proj"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Kill(context.Background(), 77); err != nil {
		t.Fatal(err)
	}
	e.fake.panes[e.privSock] = ""
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := c.StartFailures(); len(got) != 0 {
		t.Errorf("killed session reported as a failed start: %+v", got)
	}
}

func TestFailedKillKeepsFollowingLaunch(t *testing.T) {
	e := newEnv(t)
	c := e.core()
	c.recordLaunch(tmux.Pane{ID: "%5", SessionName: "proj"}, "/home/u/proj")
	l := c.takeLaunch("%5")
	c.restoreLaunch("%5", l)
	c.mu.Lock()
	c.updateLaunches(snapshot{private: e.privSock, privateGone: true}, 1)
	c.mu.Unlock()
	if got := c.StartFailures(); len(got) != 1 {
		t.Errorf("restored launch not followed: %+v", got)
	}
}

func TestUpdateLaunchesWaitsForLaterListingAndTransientErrors(t *testing.T) {
	e := newEnv(t)
	c := e.core()
	c.recordLaunch(tmux.Pane{ID: "%5", SessionName: "proj"}, "/home/u/proj")
	c.mu.Lock()
	// The listing began before the launch was recorded: the pane may be missing.
	c.updateLaunches(snapshot{private: e.privSock, privateOK: true}, 0)
	// The private server fails without being gone.
	c.updateLaunches(snapshot{private: e.privSock}, 1)
	c.mu.Unlock()
	if got := c.StartFailures(); len(got) != 0 {
		t.Errorf("failure reported too early: %+v", got)
	}
	c.mu.Lock()
	c.updateLaunches(snapshot{private: e.privSock, privateGone: true}, 1)
	c.mu.Unlock()
	if got := c.StartFailures(); len(got) != 1 {
		t.Errorf("failure missing once the server is gone: %+v", got)
	}
}

func TestServerNotRunning(t *testing.T) {
	for msg, want := range map[string]bool{
		"tmux: exit status 1: no server running on /s":                     true,
		"tmux: exit status 1: error connecting to /s (Connection refused)": true,
		"tmux: délai de 2s dépassé: context deadline exceeded":             false,
	} {
		if got := serverNotRunning(errors.New(msg)); got != want {
			t.Errorf("%q: %v", msg, got)
		}
	}
	if serverNotRunning(nil) {
		t.Error("nil error")
	}
}

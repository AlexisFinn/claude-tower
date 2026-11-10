package core

import (
	"context"
	"reflect"
	"testing"
	"time"

	"claude-tower/internal/locate"
	"claude-tower/internal/registry"
	"claude-tower/internal/tmux"
)

const idBG = "504d303c-0bd9-4395-8732-58110622c3c5"

func bgSession(e *env) {
	e.session(500, `"sessionId":"`+idBG+`","name":"bg","cwd":"/home/u/zest","kind":"bg","startedAt":1000,"status":"idle"`)
}

func TestBackgroundSessionShownByUserAttachPane(t *testing.T) {
	e := newEnv(t)
	bgSession(e)
	// The user runs `claude attach 504d303c` (PID 41) in a shell of pane %1.
	e.tree = locate.Tree{500: 400, 400: 1, 41: 40, 40: 1}
	e.attached = locate.Attachments{41: "504d303c"}
	e.fake.panes[e.userSock] = paneLine("%1", 40, "$1", "main", "@1", "")
	e.fake.panes[e.privSock] = paneLine("%9", 90, "$9", tmux.AttachPrefix+"504d303c", "@9", "")
	cards, err := e.core().Cards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 {
		t.Fatalf("cards: %+v", cards)
	}
	c := cards[0]
	if c.PID != 500 || !c.Detached || !c.InTmux || c.Private || !c.CanPrompt {
		t.Errorf("card: %+v", c)
	}
}

func TestBackgroundSessionShownByPrivateAttachSession(t *testing.T) {
	e := newEnv(t)
	bgSession(e)
	e.tree = locate.Tree{500: 400, 400: 1}
	e.fake.panes[e.privSock] = paneLine("%9", 90, "$9", tmux.AttachPrefix+"504d303c", "@9", "")
	c := e.core()
	cards, err := c.Cards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || !cards[0].Detached || !cards[0].Private || !cards[0].InTmux {
		t.Fatalf("cards (the attach session never gets a starting card): %+v", cards)
	}
	if p, ok := c.PaneOf(500); !ok || p.ID != "%9" {
		t.Errorf("pane: %+v %v", p, ok)
	}
}

func TestBackgroundSessionWithoutPaneIsReadOnly(t *testing.T) {
	e := newEnv(t)
	bgSession(e)
	e.tree = locate.Tree{500: 400, 400: 1}
	e.hosting = nil
	cards, err := e.core().Cards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 1 || !cards[0].Detached || cards[0].InTmux {
		t.Errorf("cards: %+v", cards)
	}
}

func TestPlanAttachments(t *testing.T) {
	priv := "/priv.sock"
	srv := tmux.Server{Socket: priv}
	user := tmux.Server{Socket: "/user.sock"}
	ours := tmux.Pane{Server: srv, ID: "%9", SessionName: tmux.AttachPrefix + "aaaaaaaa"}
	orphan := tmux.Pane{Server: srv, ID: "%8", SessionName: tmux.AttachPrefix + "gone0000"}
	plain := tmux.Pane{Server: srv, ID: "%7", SessionName: "proj"}
	sessions := []registry.Session{
		{PID: 1, SessionID: "aaaaaaaa-1"}, // shown by our attach session: kept
		{PID: 2, SessionID: "bbbbbbbb-2"}, // shown by nothing: created
		{PID: 3, SessionID: "cccccccc-3"}, // shown by nothing, tried recently: waits
		{PID: 4, SessionID: "dddddddd-4"}, // shown by a pane of the user: nothing
		{PID: 5, SessionID: "eeeeeeee-5"}, // not a background session
	}
	snap := snapshot{
		sessions:      sessions,
		private:       priv,
		panes:         map[int]tmux.Pane{1: ours, 4: {Server: user, ID: "%1"}},
		background:    map[int]string{1: "aaaaaaaa", 2: "bbbbbbbb", 3: "cccccccc", 4: "dddddddd"},
		listedPrivate: []tmux.Pane{ours, orphan, plain},
		userComplete:  true,
	}
	now := time.Unix(1000, 0)
	tried := map[string]attachAttempt{
		"cccccccc": {at: now.Add(-time.Second), count: 1},
		"bbbbbbbb": {at: now.Add(-attachRetry), count: 1},
	}
	w := planAttachments(snap, tried, now)
	var created []int
	for _, s := range w.create {
		created = append(created, s.PID)
	}
	if !reflect.DeepEqual(created, []int{2}) {
		t.Errorf("created: %v", created)
	}
	if !reflect.DeepEqual(w.remove, []tmux.Pane{orphan}) {
		t.Errorf("removed: %+v", w.remove)
	}
	if !reflect.DeepEqual(w.shown, []string{"aaaaaaaa"}) {
		t.Errorf("shown: %v", w.shown)
	}

	// A user server that failed may hide the pane showing session 2.
	snap.userComplete = false
	if w := planAttachments(snap, tried, now); len(w.create) != 0 {
		t.Errorf("created with an incomplete snapshot: %+v", w.create)
	}
	snap.userComplete = true

	// After two failed attempts, the wait doubles.
	tried["bbbbbbbb"] = attachAttempt{at: now.Add(-attachRetry), count: 2}
	if w := planAttachments(snap, tried, now); len(w.create) != 0 {
		t.Errorf("retried too early: %+v", w.create)
	}

	// Once the user shows session 1 in a pane of theirs, our attach session goes.
	snap.panes[1] = tmux.Pane{Server: user, ID: "%2"}
	if w := planAttachments(snap, tried, now); len(w.remove) != 2 {
		t.Errorf("removed: %+v", w.remove)
	}
}

func TestCardsStartAttachSession(t *testing.T) {
	e := newEnv(t)
	prepareClaude(t, e)
	bgSession(e)
	e.tree = locate.Tree{500: 400, 400: 1}
	c := e.core()
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, cmd := range e.fake.newSessions(e.privSock) {
			if i := indexOf(cmd, "-s"); i >= 0 && cmd[i+1] == tmux.AttachPrefix+"504d303c" {
				if !reflect.DeepEqual(cmd[len(cmd)-2:], []string{"attach", "504d303c"}) {
					t.Errorf("command: %v", cmd)
				}
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no attach session started: %v", e.fake.calls)
}

func TestKillStopsBackgroundSession(t *testing.T) {
	e := newEnv(t)
	claude := prepareClaude(t, e)
	bgSession(e)
	e.tree = locate.Tree{500: 400, 400: 1}
	e.fake.panes[e.privSock] = paneLine("%9", 90, "$9", tmux.AttachPrefix+"504d303c", "@9", "")
	c := e.core()
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Kill(context.Background(), 500); err != nil {
		t.Fatal(err)
	}
	var stop []string
	for _, call := range e.fake.others {
		if len(call) > 2 && call[len(call)-2] == "stop" {
			stop = call
		}
	}
	if len(stop) != 6 || stop[0] != "exec-env" || !reflect.DeepEqual(stop[3:], []string{claude, "stop", "504d303c"}) {
		t.Errorf("stop call: %v (others %v)", stop, e.fake.others)
	}
	if e.fake.has(e.privSock, "kill-session") {
		t.Error("attach session killed instead of the background session")
	}
}

func TestAttachDelay(t *testing.T) {
	for count, want := range map[int]time.Duration{0: attachRetry, 1: attachRetry, 2: 2 * attachRetry, 3: 4 * attachRetry, 50: maxAttachRetry} {
		if got := attachDelay(count); got != want {
			t.Errorf("attachDelay(%d) = %v, want %v", count, got, want)
		}
	}
}

func TestAmbiguousNamesAreNotAttachTargets(t *testing.T) {
	a := registry.Session{SessionID: "aaaaaaaa-1", Name: "work"}
	b := registry.Session{SessionID: "bbbbbbbb-2", Name: "work"}
	c := registry.Session{SessionID: "cccccccc-3", Name: "aaaaaaaa"}
	d := registry.Session{SessionID: "dddddddd-4", Name: "solo"}
	amb := ambiguousNames([]registry.Session{a, b, c, d})
	if attachTargets("work", a, amb) || attachTargets("aaaaaaaa", c, amb) {
		t.Errorf("ambiguous name accepted: %v", amb)
	}
	if !attachTargets("aaaaaaaa", a, amb) || !attachTargets("solo", d, amb) || !attachTargets("bbbbbbbb-2", b, amb) {
		t.Errorf("target refused: %v", amb)
	}
}

func TestOrphanAttachSessionRemovedAfterTwoSnapshots(t *testing.T) {
	e := newEnv(t)
	e.fake.panes[e.privSock] = paneLine("%9", 90, "$9", tmux.AttachPrefix+"gone0000", "@9", "")
	c := e.core()
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if e.fake.has(e.privSock, "kill-session") {
		t.Fatal("removed after a single snapshot")
	}
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for !e.fake.has(e.privSock, "kill-session") {
		if time.Now().After(deadline) {
			t.Fatal("orphan attach session kept")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const bgTime = "2026-11-09T11:30:00.000Z"

func bgAgent(id string) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"toolUseResult":{"status":"async_launched","agentId":%q}}`, bgTime, id)
}

func bgShell(id string) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"toolUseResult":{"backgroundTaskId":%q}}`, bgTime, id)
}

func bgEnd(id string) string {
	return fmt.Sprintf(`{"type":"queue-operation","operation":"enqueue","content":"<task-notification>\n<task-id>%s</task-id>\n<status>completed</status>\n</task-notification>"}`, id)
}

func TestCardsBackgroundCounts(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","name":"alpha","startedAt":1000,"status":"idle"`)
	e.session(200, `"sessionId":"`+idB+`","name":"beta","startedAt":1000,"status":"idle"`)
	e.session(300, `"sessionId":"`+idC+`","name":"gamma","startedAt":1794224700000,"status":"idle"`)
	e.transcript(idA, bgAgent("a1"), bgAgent("a2"), bgEnd("a2"), bgShell("b1"), bgShell("b2"))
	e.transcript(idB, `{"type":"assistant","message":{"content":[]}}`)
	e.transcript(idC, bgAgent("a3"), bgShell("b3"))
	e.tree = map[int]int{100: 10, 10: 1, 101: 100, 102: 101, 200: 20, 20: 1, 300: 30, 30: 1, 301: 300}
	fds := map[int][]string{
		102: {"/tmp/t/tasks/b1.output"},
		301: {"/tmp/t/tasks/b3.output"},
	}
	c := e.core(func(o *Options) {
		o.ReadFDs = func(pid int) []string { return fds[pid] }
	})
	cards, err := c.Cards(context.Background())
	if err != nil || len(cards) != 3 {
		t.Fatalf("cards=%+v err=%v", cards, err)
	}
	byPID := map[int]Card{}
	for _, card := range cards {
		byPID[card.PID] = card
	}
	if a := byPID[100]; a.BackgroundAgents != 1 || a.BackgroundShells != 1 || a.State != "background" || a.Label != "fini · fond" {
		t.Errorf("alpha: %+v", a)
	}
	if b := byPID[200]; b.BackgroundAgents != 0 || b.BackgroundShells != 0 || b.State != "idle" {
		t.Errorf("beta: %+v", b)
	}
	if g := byPID[300]; g.BackgroundAgents != 0 || g.BackgroundShells != 0 || g.State != "idle" {
		t.Errorf("gamma (resumed after the launches): %+v", g)
	}
}

func TestCardsBackgroundUpdatesIncrementally(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"idle"`)
	e.transcript(idA, bgAgent("a1"))
	path := filepath.Join(e.cfgDir, "projects", "slug", idA+".jsonl")
	c := e.core(func(o *Options) { o.ReadFDs = func(int) []string { return nil } })
	agents := func() int {
		cards, err := c.Cards(context.Background())
		if err != nil || len(cards) != 1 {
			t.Fatalf("cards=%+v err=%v", cards, err)
		}
		return cards[0].BackgroundAgents
	}
	if got := agents(); got != 1 {
		t.Fatalf("agents=%d want 1", got)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(bgAgent("a2") + "\n" + bgEnd("a1") + "\n"); err != nil {
		t.Fatal(err)
	}
	if got := agents(); got != 1 {
		t.Errorf("agents=%d want 1 (a2 running, a1 ended)", got)
	}
}

func TestCardsBackgroundShellsUnknownWithoutDescriptorReader(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"idle"`)
	e.transcript(idA, bgShell("b1"))
	c := e.core()
	c.opts.ReadFDs = nil
	cards, err := c.Cards(context.Background())
	if err != nil || len(cards) != 1 {
		t.Fatalf("cards=%+v err=%v", cards, err)
	}
	if cards[0].BackgroundShells != -1 || cards[0].State != "idle" {
		t.Errorf("card: %+v", cards[0])
	}
}

func TestCardsDropTrackersOfVanishedSessions(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"idle"`)
	e.session(200, `"sessionId":"`+idB+`","startedAt":1000,"status":"idle"`)
	e.transcript(idA, bgAgent("a1"))
	e.transcript(idB, bgAgent("a2"))
	c := e.core(func(o *Options) { o.ReadFDs = func(int) []string { return nil } })
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(c.trackers) != 2 {
		t.Fatalf("trackers=%d want 2", len(c.trackers))
	}
	first := c.trackers[idA]
	e.alive[200] = false
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.trackers[idB]; ok || len(c.trackers) != 1 || c.trackers[idA] != first {
		t.Errorf("trackers=%v", c.trackers)
	}
}

func TestCardsBackgroundUnknownWithoutTranscript(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"idle"`)
	c := e.core(func(o *Options) { o.ReadFDs = func(int) []string { return nil } })
	cards, err := c.Cards(context.Background())
	if err != nil || len(cards) != 1 {
		t.Fatalf("cards=%+v err=%v", cards, err)
	}
	if cards[0].BackgroundAgents != -1 || cards[0].BackgroundShells != -1 {
		t.Errorf("card: %+v", cards[0])
	}
}

func TestCardsBackgroundUnknownWhenTranscriptIsUnreadable(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"idle"`)
	e.transcript(idA, bgAgent("a1"))
	path := filepath.Join(e.cfgDir, "projects", "slug", idA+".jsonl")
	c := e.core(func(o *Options) { o.ReadFDs = func(int) []string { return nil } })
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0o644)
	if f, err := os.Open(path); err == nil {
		f.Close()
		t.Skip("file permissions are not enforced")
	}
	cards, err := c.Cards(context.Background())
	if err != nil || len(cards) != 1 {
		t.Fatalf("cards=%+v err=%v", cards, err)
	}
	if cards[0].BackgroundAgents != -1 || cards[0].BackgroundShells != -1 {
		t.Errorf("card: %+v", cards[0])
	}
}

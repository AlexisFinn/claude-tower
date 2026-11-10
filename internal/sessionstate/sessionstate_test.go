package sessionstate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"claude-tower/internal/background"
	"claude-tower/internal/registry"
	"claude-tower/internal/transcript"
)

const sid = "11111111-2222-3333-4444-555555555555"

func finderWith(t *testing.T, lines ...string) *transcript.Finder {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "projects", "slug", sid+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return transcript.NewFinder(dir)
}

func TestEnrich(t *testing.T) {
	finder := finderWith(t,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}`,
		`{"type":"system","subtype":"turn_duration"}`)
	cases := []struct {
		name           string
		session        registry.Session
		counts         background.Counts
		wantTurnEnded  bool
		wantBackground bool
	}{
		{"turn ended, no background", registry.Session{PID: 1, SessionID: sid}, background.Counts{}, true, false},
		{"agents running", registry.Session{PID: 1, SessionID: sid}, background.Counts{Agents: 2}, true, true},
		{"shells running", registry.Session{PID: 1, SessionID: sid}, background.Counts{Shells: 1}, true, true},
		{"unknown counts", registry.Session{PID: 1, SessionID: sid}, background.Counts{Agents: background.Unknown, Shells: background.Unknown}, true, false},
		{"no transcript", registry.Session{PID: 1, SessionID: "99999999-2222-3333-4444-555555555555"}, background.Counts{Shells: 1}, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			infos := Enrich([]registry.Session{c.session}, finder, map[int]background.Counts{1: c.counts})
			if len(infos) != 1 {
				t.Fatalf("infos=%d", len(infos))
			}
			got := infos[0]
			if got.Session.TurnEnded != c.wantTurnEnded || got.Session.HasBackground != c.wantBackground || got.Background != c.counts {
				t.Errorf("got %+v", got)
			}
		})
	}
}

func TestEnrichKeepsTheInputAndItsOrder(t *testing.T) {
	finder := finderWith(t, `{"type":"system","subtype":"turn_duration"}`)
	sessions := []registry.Session{{PID: 7, SessionID: sid}, {PID: 3}}
	infos := Enrich(sessions, finder, map[int]background.Counts{3: {Agents: 1}})
	if infos[0].Session.PID != 7 || infos[1].Session.PID != 3 {
		t.Errorf("order: %+v", infos)
	}
	if sessions[0].TurnEnded || sessions[1].HasBackground {
		t.Errorf("input modified: %+v", sessions)
	}
	if !infos[0].Session.TurnEnded || infos[0].Session.HasBackground || !infos[1].Session.HasBackground {
		t.Errorf("got %+v", infos)
	}
}

func TestEnrichActivity(t *testing.T) {
	finder := finderWith(t, `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"make"}}]}}`)
	infos := Enrich([]registry.Session{{PID: 1, SessionID: sid}}, finder, nil)
	if infos[0].Activity != "Bash: make" {
		t.Errorf("activity=%q", infos[0].Activity)
	}
}

func TestBusyTranscriptsOnlyReadsBusySessionsOnly(t *testing.T) {
	finder := finderWith(t,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"make"}}]}}`,
		`{"type":"system","subtype":"turn_duration"}`)
	for _, c := range []struct {
		state    registry.State
		wantRead bool
	}{
		{registry.StateBusy, true},
		{registry.StateIdle, false},
		{registry.StateWaiting, false},
	} {
		t.Run(fmt.Sprint(c.state), func(t *testing.T) {
			infos := Enrich([]registry.Session{{PID: 1, SessionID: sid, State: c.state}}, finder, nil, BusyTranscriptsOnly())
			got := infos[0]
			if got.Session.TurnEnded != c.wantRead || (got.Activity != "") != c.wantRead {
				t.Errorf("got %+v, want transcript read: %v", got, c.wantRead)
			}
		})
	}
}

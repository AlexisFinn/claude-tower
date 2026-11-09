package tmux

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestNewViewSessionCommands(t *testing.T) {
	r := &recorder{reply: func(args []string) ([]byte, error) {
		switch args[2] {
		case "display-message":
			return []byte("%3\n"), nil
		case "new-session":
			return []byte("$9\n"), nil
		}
		return nil, nil
	}}
	pane := Pane{ID: "%5", SessionID: "$2", WindowID: "@4"}
	v, err := r.server().NewViewSession(context.Background(), pane)
	if err != nil {
		t.Fatal(err)
	}
	if v.ID != "$9" || v.PreviousActive != "%3" || !strings.HasPrefix(v.Name, ViewPrefix) {
		t.Errorf("view: %+v", v)
	}
	want := [][]string{
		{"display-message", "-p", "-t", "@4", "#{pane_id}"},
		{"new-session", "-d", "-t", "$2", "-s", v.Name, "-P", "-F", "#{session_id}"},
		{"set-option", "-t", "$9", "status", "off"},
		{"select-window", "-t", "$9:@4"},
		{"select-pane", "-t", "%5"},
	}
	if !reflect.DeepEqual(r.commands(), want) {
		t.Errorf("commands:\n got %v\nwant %v", r.commands(), want)
	}
	for _, c := range r.commands() {
		for _, a := range c {
			if strings.Contains(a, "destroy-unattached") {
				t.Errorf("destroy-unattached set: %v", c)
			}
		}
	}
}

func TestNewViewSessionNamesAreUnique(t *testing.T) {
	r := &recorder{reply: func(args []string) ([]byte, error) { return []byte("$1\n"), nil }}
	a, _ := r.server().NewViewSession(context.Background(), Pane{})
	b, _ := r.server().NewViewSession(context.Background(), Pane{})
	if a.Name == b.Name {
		t.Errorf("same name %q", a.Name)
	}
}

func TestNewViewSessionKillsViewOnFailure(t *testing.T) {
	r := &recorder{reply: func(args []string) ([]byte, error) {
		switch args[2] {
		case "new-session":
			return []byte("$9\n"), nil
		case "select-pane":
			return nil, errors.New("no pane")
		}
		return nil, nil
	}}
	if _, err := r.server().NewViewSession(context.Background(), Pane{ID: "%1", SessionID: "$1", WindowID: "@1"}); err == nil {
		t.Fatal("error swallowed")
	}
	cmds := r.commands()
	if last := cmds[len(cmds)-1]; !reflect.DeepEqual(last, []string{"kill-session", "-t", "$9"}) {
		t.Errorf("last command: %v", last)
	}
}

func TestNewViewSessionFirstStepFailure(t *testing.T) {
	r := &recorder{reply: func([]string) ([]byte, error) { return nil, errors.New("gone") }}
	if _, err := r.server().NewViewSession(context.Background(), Pane{}); err == nil {
		t.Fatal("error swallowed")
	}
	if len(r.calls) != 1 {
		t.Errorf("calls after failure: %v", r.commands())
	}
}

func TestRestoreActivePane(t *testing.T) {
	r := &recorder{}
	if err := r.server().RestoreActivePane(context.Background(), ViewSession{ID: "$9", PreviousActive: "%3"}); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"display-message", "-p", "-t", "%3", "#{pane_id}"},
		{"select-pane", "-t", "%3"},
	}
	if !reflect.DeepEqual(r.commands(), want) {
		t.Errorf("commands: %v", r.commands())
	}
}

func TestRestoreActivePaneSkipsVanishedPane(t *testing.T) {
	r := &recorder{reply: func(args []string) ([]byte, error) {
		if args[2] == "display-message" {
			return nil, errors.New("can't find pane")
		}
		return nil, nil
	}}
	if err := r.server().RestoreActivePane(context.Background(), ViewSession{PreviousActive: "%3"}); err != nil {
		t.Errorf("err=%v", err)
	}
	if len(r.calls) != 1 {
		t.Errorf("select-pane run for a missing pane: %v", r.commands())
	}
	r = &recorder{}
	r.server().RestoreActivePane(context.Background(), ViewSession{})
	if len(r.calls) != 0 {
		t.Errorf("commands run without a noted pane: %v", r.commands())
	}
}

func TestSweepViewSessionsKillsOnlyUnattachedViews(t *testing.T) {
	list := "$1\tmain\t1\n" +
		"$2\tclaude-tower-view-1-1\t0\n" +
		"$3\tclaude-tower-view-1-2\t1\n" +
		"$4\tclaude-tower-view-1-3\t0\n" +
		"$5\tother\t0\n"
	r := &recorder{reply: func(args []string) ([]byte, error) {
		if args[2] == "list-sessions" {
			return []byte(list), nil
		}
		return nil, nil
	}}
	if err := r.server().SweepViewSessions(context.Background()); err != nil {
		t.Fatal(err)
	}
	cmds := r.commands()
	if cmds[0][0] != "list-sessions" {
		t.Fatalf("commands: %v", cmds)
	}
	want := [][]string{{"kill-session", "-t", "$2"}, {"kill-session", "-t", "$4"}}
	if !reflect.DeepEqual(cmds[1:], want) {
		t.Errorf("kills: %v", cmds[1:])
	}
}

func TestSweepViewSessionsReportsListFailure(t *testing.T) {
	r := &recorder{reply: func([]string) ([]byte, error) { return nil, errors.New("no server") }}
	if err := r.server().SweepViewSessions(context.Background()); err == nil {
		t.Error("error swallowed")
	}
}

func sessionNames(t *testing.T, s Server) []string {
	t.Helper()
	out := strings.Fields(mustExec(t, s, "list-sessions", "-F", "#{session_name}"))
	return out
}

func TestViewSessionIntegration(t *testing.T) {
	srv := testServer(t, "sleep")
	ctx := context.Background()
	mustExec(t, srv, "split-window", "-d", "-t", "main:")
	panes, err := srv.Panes(ctx)
	if err != nil || len(panes) != 2 {
		t.Fatalf("panes=%v err=%v", panes, err)
	}
	var active, other Pane
	for _, p := range panes {
		if p.Active {
			active = p
		} else {
			other = p
		}
	}
	if active.ID == "" || other.ID == "" {
		t.Fatalf("expected one active pane and one inactive: %+v", panes)
	}

	v, err := srv.NewViewSession(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if v.PreviousActive != active.ID {
		t.Errorf("PreviousActive=%q want %q", v.PreviousActive, active.ID)
	}
	if got := strings.Count(mustExec(t, srv, "list-panes", "-a", "-F", "#{pane_id}"), "\n"); got != 4 {
		t.Fatalf("raw list-panes lists %d entries, want 4 (two panes in two sessions)", got)
	}
	panes, err = srv.Panes(ctx)
	if err != nil || len(panes) != 2 {
		t.Fatalf("Panes after view: %v err=%v", panes, err)
	}
	for _, p := range panes {
		if p.Active != (p.ID == other.ID) {
			t.Errorf("after view, pane %s Active=%v", p.ID, p.Active)
		}
		if p.SessionName != "main" || p.SessionID != active.SessionID {
			t.Errorf("after view, pane %s reports session %s %s", p.ID, p.SessionID, p.SessionName)
		}
	}

	if err := srv.RestoreActivePane(ctx, v); err != nil {
		t.Fatal(err)
	}
	panes, _ = srv.Panes(ctx)
	for _, p := range panes {
		if p.Active != (p.ID == active.ID) {
			t.Errorf("after restore, pane %s Active=%v", p.ID, p.Active)
		}
	}

	if err := srv.SweepViewSessions(ctx); err != nil {
		t.Fatal(err)
	}
	if names := sessionNames(t, srv); !reflect.DeepEqual(names, []string{"main"}) {
		t.Errorf("sessions after sweep: %v", names)
	}
}

func TestKillSessionIntegration(t *testing.T) {
	srv := testServer(t, "sleep")
	mustExec(t, srv, "new-session", "-d", "-s", "second", "-t", "main")
	id := strings.TrimSpace(mustExec(t, srv, "display-message", "-p", "-t", "second:", "#{session_id}"))
	if err := srv.KillSession(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if names := sessionNames(t, srv); !reflect.DeepEqual(names, []string{"main"}) {
		t.Errorf("sessions: %v", names)
	}
}

func TestGroupedSessionsYieldOnePane(t *testing.T) {
	srv := testServer(t, "sleep")
	mustExec(t, srv, "new-session", "-d", "-s", "twin", "-t", "main")
	panes, err := srv.Panes(context.Background())
	if err != nil || len(panes) != 1 {
		t.Fatalf("panes=%v err=%v", panes, err)
	}
	if panes[0].SessionGroup == "" {
		t.Error("grouped pane has no session group")
	}
}

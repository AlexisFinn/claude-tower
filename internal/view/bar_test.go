package view

import (
	"testing"
	"time"

	"claude-tower/internal/registry"
)

func sess(name string, st registry.State, ended bool, ago time.Duration) registry.Session {
	return registry.Session{Name: name, State: st, TurnEnded: ended, StatusSince: now.Add(-ago)}
}

func TestBarEmpty(t *testing.T) {
	if got := Bar(nil, now, 100); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestBarEscapesHashAndOmitsMissingDuration(t *testing.T) {
	ss := []registry.Session{
		{Name: "a#b", State: registry.StateBusy},
		{Name: "w", State: registry.StateWaiting, StatusSince: now.Add(-3 * time.Minute)},
	}
	want := "#[fg=yellow]● a##b#[default]  #[fg=red,bold]! w 3m#[default]"
	if got := Bar(ss, now, 0); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func fiveSessions() []registry.Session {
	return []registry.Session{
		sess("w", registry.StateWaiting, false, 3*time.Minute),
		sess("b1", registry.StateBusy, false, 10*time.Second),
		sess("b2", registry.StateBusy, false, 20*time.Second),
		sess("i1", registry.StateIdle, false, time.Hour),
		sess("g", registry.StateBusy, true, time.Hour),
	}
}

func TestBarFullAndCollapsed(t *testing.T) {
	cases := []struct {
		name     string
		maxWidth int
		want     string
	}{
		{"full", 100, "#[fg=yellow]● b1 10s#[default]  #[fg=yellow]● b2 20s#[default]  #[fg=cyan]◐ g#[default]  #[fg=colour244]○ i1#[default]  #[fg=red,bold]! w 3m#[default]"},
		{"exact width", 37, "#[fg=yellow]● b1 10s#[default]  #[fg=yellow]● b2 20s#[default]  #[fg=cyan]◐ g#[default]  #[fg=colour244]○ i1#[default]  #[fg=red,bold]! w 3m#[default]"},
		{"one column short", 36, "#[fg=yellow]● b1 10s#[default]  #[fg=yellow]● b2 20s#[default]  #[fg=red,bold]! w 3m#[default]  #[fg=colour244]○ 2#[default]"},
		{"idle grouped, exact width", 31, "#[fg=yellow]● b1 10s#[default]  #[fg=yellow]● b2 20s#[default]  #[fg=red,bold]! w 3m#[default]  #[fg=colour244]○ 2#[default]"},
		{"busy grouped, one column short", 30, "#[fg=red,bold]! w 3m#[default]  #[fg=yellow]● 2#[default]  #[fg=colour244]○ 2#[default]"},
		{"idle grouped", 35, "#[fg=yellow]● b1 10s#[default]  #[fg=yellow]● b2 20s#[default]  #[fg=red,bold]! w 3m#[default]  #[fg=colour244]○ 2#[default]"},
		{"busy grouped too", 20, "#[fg=red,bold]! w 3m#[default]  #[fg=yellow]● 2#[default]  #[fg=colour244]○ 2#[default]"},
		{"waiting always listed", 1, "#[fg=red,bold]! w 3m#[default]  #[fg=yellow]● 2#[default]  #[fg=colour244]○ 2#[default]"},
	}
	for _, c := range cases {
		if got := Bar(fiveSessions(), now, c.maxWidth); got != c.want {
			t.Errorf("%s:\ngot  %q\nwant %q", c.name, got, c.want)
		}
	}
}

func TestBarUnknownAndNoGroupWhenNothingToGroup(t *testing.T) {
	ss := []registry.Session{{Name: "u"}, sess("w", registry.StateWaiting, false, 5*time.Second)}
	if got, want := Bar(ss, now, 0), "#[fg=colour244]? u#[default]  #[fg=red,bold]! w 5s#[default]"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
	only := []registry.Session{sess("w", registry.StateWaiting, false, 5*time.Second)}
	if got, want := Bar(only, now, 1), "#[fg=red,bold]! w 5s#[default]"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

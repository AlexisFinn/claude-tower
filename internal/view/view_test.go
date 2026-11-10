package view

import (
	"strings"
	"testing"
	"time"

	"claude-tower/internal/registry"
)

var now = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

func TestDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"}, {59 * time.Second, "59s"}, {60 * time.Second, "1m"}, {7 * time.Minute, "7m"},
		{59 * time.Minute, "59m"}, {60 * time.Minute, "1h00"}, {65 * time.Minute, "1h05"},
		{23*time.Hour + 59*time.Minute, "23h59"}, {24 * time.Hour, "1j"}, {80 * time.Hour, "3j"},
		{-5 * time.Second, "0s"},
	}
	for _, c := range cases {
		if got := Duration(now.Add(-c.d), now); got != c.want {
			t.Errorf("%v: got %q want %q", c.d, got, c.want)
		}
	}
	if got := Duration(time.Time{}, now); got != "" {
		t.Errorf("zero since: %q", got)
	}
}

func TestFullName(t *testing.T) {
	cases := []struct {
		name string
		s    registry.Session
		want string
	}{
		{"name", registry.Session{Name: "api", Cwd: "/x/y"}, "api"},
		{"cwd fallback", registry.Session{Cwd: "/home/a/zest"}, "zest"},
		{"empty cwd", registry.Session{PID: 42}, "pid 42"},
		{"control chars", registry.Session{Name: "a\x1b[0mb"}, "a[0mb"},
		{"long name is kept", registry.Session{Name: strings.Repeat("é", 20)}, strings.Repeat("é", 20)},
	}
	for _, c := range cases {
		if got := FullName(c.s); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestDisplayName(t *testing.T) {
	cases := []struct {
		name string
		s    registry.Session
		want string
	}{
		{"name", registry.Session{Name: "api", Cwd: "/x/y"}, "api"},
		{"cwd fallback", registry.Session{Cwd: "/home/a/zest"}, "zest"},
		{"empty cwd", registry.Session{PID: 42}, "pid 42"},
		{"root cwd", registry.Session{PID: 42, Cwd: "/"}, "pid 42"},
		{"control chars", registry.Session{Name: "a\x1b[0mb"}, "a[0mb"},
		{"multibyte truncation", registry.Session{Name: strings.Repeat("é", 20)}, strings.Repeat("é", 15) + "…"},
		{"wide truncation", registry.Session{Name: strings.Repeat("界", 10)}, strings.Repeat("界", 7) + "…"},
		{"exact fit", registry.Session{Name: strings.Repeat("a", 16)}, strings.Repeat("a", 16)},
	}
	for _, c := range cases {
		if got := DisplayName(c.s); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestShortPath(t *testing.T) {
	cases := []struct{ cwd, home, want string }{
		{"/home/alexis", "/home/alexis", "~"},
		{"/home/alexis/zest", "/home/alexis", "~/zest"},
		{"/home/alexis/zest", "/home/alex", "/home/alexis/zest"},
		{"/home/alexis/zest", "", "/home/alexis/zest"},
		{"/srv/x", "/home/alexis", "/srv/x"},
		{"/home/alexis/zest", "/home/alexis/", "~/zest"},
		{"/srv/x", "/", "/srv/x"},
	}
	for _, c := range cases {
		if got := ShortPath(c.cwd, c.home); got != c.want {
			t.Errorf("%q %q: got %q want %q", c.cwd, c.home, got, c.want)
		}
	}
}

func TestDisplayOf(t *testing.T) {
	cases := []struct {
		s    registry.Session
		want Display
	}{
		{registry.Session{State: registry.StateBusy}, DisplayBusy},
		{registry.Session{State: registry.StateBusy, TurnEnded: true}, DisplayBackground},
		{registry.Session{State: registry.StateIdle, TurnEnded: true}, DisplayIdle},
		{registry.Session{State: registry.StateIdle, HasBackground: true}, DisplayBackground},
		{registry.Session{State: registry.StateWaiting, HasBackground: true}, DisplayWaiting},
		{registry.Session{State: registry.StateBusy, HasBackground: true}, DisplayBusy},
		{registry.Session{State: registry.StateWaiting}, DisplayWaiting},
		{registry.Session{}, DisplayUnknown},
	}
	for _, c := range cases {
		if got := DisplayOf(c.s); got != c.want {
			t.Errorf("%+v: got %v want %v", c.s, got, c.want)
		}
	}
}

func TestIconAndLabel(t *testing.T) {
	icons := map[Display]string{DisplayBusy: "●", DisplayWaiting: "!", DisplayBackground: "◐", DisplayIdle: "○", DisplayUnknown: "?"}
	for d, want := range icons {
		if got := Icon(d); got != want {
			t.Errorf("icon %v: %q", d, got)
		}
	}
	cases := []struct {
		s    registry.Session
		want string
	}{
		{registry.Session{State: registry.StateBusy, RawStatus: "busy"}, "travaille"},
		{registry.Session{State: registry.StateBusy, RawStatus: "shell"}, "shell"},
		{registry.Session{State: registry.StateWaiting}, "attend"},
		{registry.Session{State: registry.StateWaiting, WaitingFor: "dialog open"}, "attend (dialog open)"},
		{registry.Session{State: registry.StateBusy, TurnEnded: true}, "fini · fond"},
		{registry.Session{State: registry.StateIdle}, "fini"},
		{registry.Session{State: registry.StateIdle, HasBackground: true}, "fini · fond"},
		{registry.Session{RawStatus: "odd"}, "odd"},
		{registry.Session{}, "?"},
	}
	for _, c := range cases {
		if got := Label(c.s); got != c.want {
			t.Errorf("%+v: got %q want %q", c.s, got, c.want)
		}
	}
}

func names(ss []registry.Session) string {
	var out []string
	for _, s := range ss {
		out = append(out, s.Name+"/"+string(rune('0'+s.PID)))
	}
	return strings.Join(out, " ")
}

func TestSortByName(t *testing.T) {
	t0 := now
	in := []registry.Session{
		{Name: "beta", PID: 3, StartedAt: t0},
		{Name: "Beta", PID: 2, StartedAt: t0.Add(time.Second)},
		{Name: "alpha", PID: 4},
		{Name: "beta", PID: 1, StartedAt: t0},
	}
	before := names(in)
	got := names(sortByName(in))
	if got != "alpha/4 beta/1 beta/3 Beta/2" {
		t.Errorf("got %s", got)
	}
	if names(in) != before {
		t.Error("argument was modified")
	}
}

func TestSortByUrgency(t *testing.T) {
	in := []registry.Session{
		{Name: "idle", PID: 1, State: registry.StateIdle},
		{Name: "bg", PID: 2, State: registry.StateBusy, TurnEnded: true},
		{Name: "unk", PID: 3},
		{Name: "busyB", PID: 4, State: registry.StateBusy},
		{Name: "Busya", PID: 5, State: registry.StateBusy},
		{Name: "wait", PID: 6, State: registry.StateWaiting},
	}
	before := names(in)
	got := names(SortByUrgency(in))
	if got != "wait/6 Busya/5 busyB/4 unk/3 bg/2 idle/1" {
		t.Errorf("got %s", got)
	}
	if names(in) != before {
		t.Error("argument was modified")
	}
}

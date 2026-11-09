// Package view holds the formatting shared by the status bar, the list output
// and the terminal interface: icons, labels, durations, names and ordering.
package view

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"claude-tower/internal/registry"

	"github.com/charmbracelet/x/ansi"
)

// Display is the presentation category of a session.
type Display int

// Presentation categories. The zero value is DisplayUnknown.
const (
	DisplayUnknown Display = iota
	DisplayWaiting
	DisplayBusy
	DisplayBackground
	DisplayIdle
)

const maxNameWidth = 16

// DisplayOf maps a session to its presentation category. A session that is
// busy but whose last turn has ended is only running background work; an idle
// session that has background work is too.
func DisplayOf(s registry.Session) Display {
	switch s.State {
	case registry.StateBusy:
		if s.TurnEnded {
			return DisplayBackground
		}
		return DisplayBusy
	case registry.StateWaiting:
		return DisplayWaiting
	case registry.StateIdle:
		if s.HasBackground {
			return DisplayBackground
		}
		return DisplayIdle
	}
	return DisplayUnknown
}

// Icon returns the symbol of a presentation category.
func Icon(d Display) string {
	switch d {
	case DisplayBusy:
		return "●"
	case DisplayWaiting:
		return "!"
	case DisplayBackground:
		return "◐"
	case DisplayIdle:
		return "○"
	}
	return "?"
}

// Label returns the human-readable state of a session.
func Label(s registry.Session) string {
	switch DisplayOf(s) {
	case DisplayBusy:
		if s.RawStatus == "shell" {
			return "shell"
		}
		return "travaille"
	case DisplayWaiting:
		if s.WaitingFor != "" {
			return "attend (" + s.WaitingFor + ")"
		}
		return "attend"
	case DisplayBackground:
		return "fini · fond"
	case DisplayIdle:
		return "fini"
	}
	if s.RawStatus == "" {
		return "?"
	}
	return s.RawStatus
}

// Duration formats the time elapsed since the given instant, or "" when since
// is the zero time.
func Duration(since, now time.Time) string {
	if since.IsZero() {
		return ""
	}
	d := max(now.Sub(since), 0)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d/time.Second))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%02d", int(d/time.Hour), int(d%time.Hour/time.Minute))
	}
	return fmt.Sprintf("%dj", int(d/(24*time.Hour)))
}

// DisplayName returns the name shown for a session, at most 16 columns wide.
func DisplayName(s registry.Session) string {
	name := s.Name
	if name == "" {
		name = "pid " + fmt.Sprint(s.PID)
		if s.Cwd != "" {
			if base := filepath.Base(s.Cwd); base != "." && base != "/" {
				name = base
			}
		}
	}
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	return ansi.Truncate(name, maxNameWidth, "…")
}

// ShortPath replaces the home directory prefix of cwd with "~".
func ShortPath(cwd, home string) string {
	home = strings.TrimRight(home, "/")
	if home == "" {
		return cwd
	}
	if cwd == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(cwd, home+"/"); ok {
		return "~/" + rest
	}
	return cwd
}

// SortByName returns a copy ordered by display name (case-insensitive), then
// start time, then PID. It is stable from one refresh to the next.
func SortByName(ss []registry.Session) []registry.Session {
	out := append([]registry.Session(nil), ss...)
	sort.SliceStable(out, func(i, j int) bool { return lessByName(out[i], out[j]) })
	return out
}

// SortByUrgency returns a copy ordered by how much attention a session needs:
// waiting, busy, unknown, background, idle, then by name.
func SortByUrgency(ss []registry.Session) []registry.Session {
	out := append([]registry.Session(nil), ss...)
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := urgencyRank(DisplayOf(out[i])), urgencyRank(DisplayOf(out[j]))
		if ri != rj {
			return ri < rj
		}
		return lessByName(out[i], out[j])
	})
	return out
}

func urgencyRank(d Display) int {
	switch d {
	case DisplayWaiting:
		return 0
	case DisplayBusy:
		return 1
	case DisplayUnknown:
		return 2
	case DisplayBackground:
		return 3
	}
	return 4
}

func lessByName(a, b registry.Session) bool {
	na, nb := strings.ToLower(DisplayName(a)), strings.ToLower(DisplayName(b))
	if na != nb {
		return na < nb
	}
	if !a.StartedAt.Equal(b.StartedAt) {
		return a.StartedAt.Before(b.StartedAt)
	}
	return a.PID < b.PID
}

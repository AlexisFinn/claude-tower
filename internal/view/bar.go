package view

import (
	"fmt"
	"strings"
	"time"

	"claude-tower/internal/registry"

	"github.com/charmbracelet/x/ansi"
)

const (
	barSeparator = "  "
	styleDefault = "#[default]"
)

// barItem is one rendered element of the status bar. width is the visible
// width, which excludes the tmux style directives.
type barItem struct {
	text  string
	width int
}

// Bar renders the sessions as a tmux status segment. When the visible width
// exceeds maxWidth, finished sessions and then busy ones are collapsed into
// counters; waiting sessions always stay listed. maxWidth <= 0 disables the
// limit.
func Bar(ss []registry.Session, now time.Time, maxWidth int) string {
	if len(ss) == 0 {
		return ""
	}
	sorted := SortByName(ss)
	items := make([]barItem, 0, len(sorted))
	for _, s := range sorted {
		items = append(items, sessionItem(s, now))
	}
	if maxWidth > 0 && totalWidth(items) > maxWidth {
		items = collapse(sorted, now, maxWidth)
	}
	texts := make([]string, len(items))
	for i, it := range items {
		texts[i] = it.text
	}
	return strings.Join(texts, barSeparator)
}

// collapse groups idle-like sessions, then busy ones, into counters.
func collapse(sorted []registry.Session, now time.Time, maxWidth int) []barItem {
	idle, busy := 0, 0
	for _, s := range sorted {
		switch DisplayOf(s) {
		case DisplayBusy:
			busy++
		case DisplayWaiting:
		default:
			idle++
		}
	}
	build := func(groupBusy bool) []barItem {
		var items []barItem
		for _, s := range sorted {
			d := DisplayOf(s)
			if d == DisplayWaiting || (d == DisplayBusy && !groupBusy) {
				items = append(items, sessionItem(s, now))
			}
		}
		if groupBusy && busy > 0 {
			items = append(items, counterItem("yellow", DisplayBusy, busy))
		}
		if idle > 0 {
			items = append(items, counterItem("colour244", DisplayIdle, idle))
		}
		return items
	}
	if items := build(false); totalWidth(items) <= maxWidth {
		return items
	}
	return build(true)
}

func counterItem(color string, d Display, n int) barItem {
	plain := fmt.Sprintf("%s %d", Icon(d), n)
	return barItem{text: "#[fg=" + color + "]" + plain + styleDefault, width: ansi.StringWidth(plain)}
}

func sessionItem(s registry.Session, now time.Time) barItem {
	d := DisplayOf(s)
	name := DisplayName(s)
	plain := Icon(d) + " " + name
	text := Icon(d) + " " + strings.ReplaceAll(name, "#", "##")
	if d == DisplayBusy || d == DisplayWaiting {
		if dur := Duration(s.StatusSince, now); dur != "" {
			plain += " " + dur
			text += " " + dur
		}
	}
	return barItem{text: barStyle(d) + text + styleDefault, width: ansi.StringWidth(plain)}
}

func barStyle(d Display) string {
	switch d {
	case DisplayBusy:
		return "#[fg=yellow]"
	case DisplayBackground:
		return "#[fg=cyan]"
	case DisplayWaiting:
		return "#[fg=red,bold]"
	}
	return "#[fg=colour244]"
}

func totalWidth(items []barItem) int {
	w := len(barSeparator) * max(len(items)-1, 0)
	for _, it := range items {
		w += it.width
	}
	return w
}

// Package tui is the interactive terminal interface listing the sessions and
// jumping to the tmux pane of the chosen one.
package tui

import (
	"strings"
	"time"

	"claude-tower/internal/registry"
	"claude-tower/internal/tmuxctl"
	"claude-tower/internal/transcript"
	"claude-tower/internal/view"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	defaultWidth    = 80
	defaultInterval = time.Second
	footerHelp      = "↑↓ choisir · 1-9 sélection · Entrée aller · q fermer"
	emptyMessage    = "Aucune session Claude en cours."
)

// loadedMsg carries the result of one load: the live sessions, the last
// activity text by PID, the instant of the load and any error.
type loadedMsg struct {
	sessions []registry.Session
	activity map[int]string
	now      time.Time
	err      error
}

type tickMsg struct{}

type jumpDoneMsg struct{ err error }

// Model is the Bubble Tea model of the interface. Its Update performs no I/O
// and its View never reads the clock.
type Model struct {
	load     func() loadedMsg
	jump     func(client, pane string, pid int) error
	client   string
	home     string
	interval time.Duration

	sessions []registry.Session
	activity map[int]string
	now      time.Time
	selected int // PID of the selected session, 0 when none
	offset   int
	width    int
	height   int
	jumpErr  error
	loadErr  error
}

func newModel(load func() loadedMsg, jump func(client, pane string, pid int) error, client, home string) Model {
	return Model{
		load:     load,
		jump:     jump,
		client:   client,
		home:     home,
		interval: defaultInterval,
		width:    defaultWidth,
	}
}

// activitySource is the part of transcript.Finder used by the loader.
type activitySource interface {
	Last(sessionID string) transcript.Activity
	Retain(sessionIDs []string)
}

// NewLoader returns a load function reading the sessions of configDir. It
// keeps one transcript Finder for its whole life.
func NewLoader(configDir string) func() loadedMsg {
	return newLoader(registry.Reader{Dir: configDir}, transcript.NewFinder(configDir))
}

func newLoader(reader registry.Reader, finder activitySource) func() loadedMsg {
	return func() loadedMsg {
		t := time.Now()
		sessions, err := reader.Sessions()
		if err != nil {
			return loadedMsg{now: t, err: err}
		}
		ids := make([]string, len(sessions))
		for i, s := range sessions {
			ids[i] = s.SessionID
		}
		finder.Retain(ids)
		activity := make(map[int]string, len(sessions))
		for i, s := range sessions {
			act := finder.Last(s.SessionID)
			sessions[i].TurnEnded = act.TurnEnded
			activity[s.PID] = act.Text
		}
		return loadedMsg{sessions: sessions, activity: activity, now: t}
	}
}

// Run starts the interface in the alternate screen and blocks until it
// exits.
func Run(configDir, home, client string) error {
	m := newModel(NewLoader(configDir), tmuxctl.Jump, client, home)
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

// Init starts the first load.
func (m Model) Init() tea.Cmd { return m.loadCmd() }

func (m Model) loadCmd() tea.Cmd {
	load := m.load
	return func() tea.Msg { return load() }
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ensureVisible()
	case loadedMsg:
		m.applyLoaded(msg)
		return m, tea.Tick(m.interval, func(time.Time) tea.Msg { return tickMsg{} })
	case tickMsg:
		return m, m.loadCmd()
	case jumpDoneMsg:
		if msg.err != nil {
			m.jumpErr = msg.err
			return m, nil
		}
		return m, tea.Quit
	case tea.KeyMsg:
		m.jumpErr = nil
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) applyLoaded(msg loadedMsg) {
	m.loadErr = msg.err
	if msg.err != nil {
		return
	}
	oldIdx := m.selectedIndex()
	m.sessions = view.SortByUrgency(msg.sessions)
	m.activity = msg.activity
	m.now = msg.now
	switch {
	case len(m.sessions) == 0:
		m.selected = 0
	case m.selected == 0:
		m.selected = m.sessions[0].PID
	case m.selectedIndex() < 0:
		m.selected = m.sessions[min(max(oldIdx, 0), len(m.sessions)-1)].PID
	}
	m.ensureVisible()
}

func (m Model) selectedIndex() int {
	for i, s := range m.sessions {
		if s.PID == m.selected {
			return i
		}
	}
	return -1
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "enter":
		return m, m.jumpCmd()
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			m.selectRank(int(key[0] - '0'))
		}
	}
	return m, nil
}

func (m *Model) move(delta int) {
	idx := m.selectedIndex()
	if idx < 0 {
		return
	}
	m.selectAt(idx + delta)
}

func (m *Model) selectRank(rank int) {
	m.selectAt(rank - 1)
}

func (m *Model) selectAt(idx int) {
	if idx < 0 || idx >= len(m.sessions) {
		return
	}
	m.selected = m.sessions[idx].PID
	m.ensureVisible()
}

func (m Model) jumpCmd() tea.Cmd {
	idx := m.selectedIndex()
	if idx < 0 {
		return nil
	}
	s, jump, client := m.sessions[idx], m.jump, m.client
	return func() tea.Msg { return jumpDoneMsg{err: jump(client, s.TmuxPane, s.PID)} }
}

// visibleCount is the number of sessions shown at once. Every session takes
// two lines; two lines stay reserved below them, one for the footer and one
// spare. An unknown height shows them all.
func (m Model) visibleCount() int {
	if m.height <= 0 {
		return max(len(m.sessions), 1)
	}
	return max(1, (m.height-2)/2)
}

func (m *Model) ensureVisible() {
	n := m.visibleCount()
	if idx := m.selectedIndex(); idx >= 0 {
		if idx < m.offset {
			m.offset = idx
		}
		if idx >= m.offset+n {
			m.offset = idx - n + 1
		}
	}
	m.offset = max(min(m.offset, len(m.sessions)-n), 0)
}

// View implements tea.Model.
func (m Model) View() string {
	var lines []string
	if len(m.sessions) == 0 {
		lines = append(lines, emptyMessage, "")
	}
	end := min(m.offset+m.visibleCount(), len(m.sessions))
	for i := m.offset; i < end; i++ {
		lines = append(lines, m.sessionLines(i)...)
	}
	lines = append(lines, m.footer())
	if m.height > 0 && len(lines) > m.height {
		lines = lines[:m.height]
	}
	return strings.Join(lines, "\n")
}

func (m Model) sessionLines(i int) []string {
	s := m.sessions[i]
	d := view.DisplayOf(s)
	rank := "  "
	if i < 9 {
		rank = string(rune('1'+i)) + " "
	}
	fields := []string{view.DisplayName(s), view.Label(s), view.Duration(s.StatusSince, m.now), view.ShortPath(s.Cwd, m.home)}
	kept := fields[:0]
	for _, f := range fields {
		if f != "" {
			kept = append(kept, f)
		}
	}
	first := rank + view.Icon(d) + " " + strings.Join(kept, "  ")
	second := ""
	if act := m.activity[s.PID]; act != "" {
		second = "  └ " + act
	}
	style := lineStyle(d)
	if s.PID == m.selected {
		style = style.Reverse(true)
	}
	return []string{m.render(style, first), m.render(style, second)}
}

func (m Model) render(style lipgloss.Style, line string) string {
	return style.Render(ansi.Truncate(line, m.width, "…"))
}

func (m Model) footer() string {
	text := footerHelp
	switch {
	case m.jumpErr != nil:
		text = m.jumpErr.Error()
	case m.loadErr != nil:
		text = m.loadErr.Error()
	}
	return ansi.Truncate(text, m.width, "…")
}

func lineStyle(d view.Display) lipgloss.Style {
	switch d {
	case view.DisplayBusy:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	case view.DisplayBackground:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	case view.DisplayWaiting:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
}

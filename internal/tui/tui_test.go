package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"claude-tower/internal/registry"
	"claude-tower/internal/transcript"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

var testNow = time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

func sess(pid int, name string, state registry.State, raw string) registry.Session {
	return registry.Session{
		PID: pid, Name: name, Cwd: "/home/u/" + name, State: state, RawStatus: raw,
		StatusSince: testNow.Add(-5 * time.Minute), TmuxPane: fmt.Sprintf("%%%d", pid),
	}
}

func busy(pid int, name string) registry.Session { return sess(pid, name, registry.StateBusy, "busy") }
func idle(pid int, name string) registry.Session { return sess(pid, name, registry.StateIdle, "idle") }

func loaded(ss ...registry.Session) loadedMsg {
	act := map[int]string{}
	for _, s := range ss {
		act[s.PID] = "act " + s.Name
	}
	return loadedMsg{sessions: ss, activity: act, now: testNow}
}

func newTest(t *testing.T, ss ...registry.Session) Model {
	t.Helper()
	m := newModel(func() loadedMsg { return loaded(ss...) }, func(string, string, int) error { return nil }, "cli", "/home/u")
	m.interval = time.Millisecond
	return feed(m, tea.WindowSizeMsg{Width: 80, Height: 24}, loaded(ss...))
}

func feed(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func key(s string) tea.KeyMsg {
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestInitCallsLoad(t *testing.T) {
	calls := 0
	m := newModel(func() loadedMsg { calls++; return loadedMsg{} }, nil, "", "")
	msg := m.Init()()
	if _, ok := msg.(loadedMsg); !ok || calls != 1 {
		t.Errorf("msg=%T calls=%d", msg, calls)
	}
}

func TestTickCycle(t *testing.T) {
	calls := 0
	m := newModel(func() loadedMsg { calls++; return loadedMsg{} }, nil, "", "")
	m.interval = time.Millisecond
	_, cmd := m.Update(loadedMsg{})
	if _, ok := cmd().(tickMsg); !ok {
		t.Fatalf("expected tickMsg after loadedMsg")
	}
	_, cmd = m.Update(tickMsg{})
	if _, ok := cmd().(loadedMsg); !ok || calls != 1 {
		t.Errorf("tick should run load: calls=%d", calls)
	}
}

func TestSortedByUrgencyAndFirstSelected(t *testing.T) {
	m := newTest(t, idle(1, "aaa"), busy(2, "bbb"))
	if m.sessions[0].PID != 2 || m.selected != 2 {
		t.Errorf("order/selection: %v selected=%d", m.sessions, m.selected)
	}
}

func TestSelectionFollowsPIDAfterResort(t *testing.T) {
	m := newTest(t, busy(1, "aaa"), busy(2, "bbb"))
	m = feed(m, key("j"))
	if m.selected != 2 {
		t.Fatalf("selected=%d", m.selected)
	}
	m = feed(m, loaded(idle(1, "aaa"), busy(2, "bbb"), busy(3, "ccc")))
	if m.selected != 2 || m.sessions[0].PID != 2 {
		t.Errorf("selected=%d order=%v", m.selected, m.sessions)
	}
}

func TestSelectedSessionDisappears(t *testing.T) {
	m := newTest(t, busy(1, "aaa"), busy(2, "bbb"), busy(3, "ccc"))
	m = feed(m, key("j"), loaded(busy(1, "aaa"), busy(3, "ccc")))
	if m.selected != 3 {
		t.Errorf("want same index (pid 3), got %d", m.selected)
	}
	m = feed(m, key("j"), loaded(busy(1, "aaa")))
	if m.selected != 1 {
		t.Errorf("want clamped to last (pid 1), got %d", m.selected)
	}
}

func TestEmptyThenNonEmpty(t *testing.T) {
	m := newTest(t, busy(1, "aaa"))
	m = feed(m, loaded())
	if m.selected != 0 || !strings.Contains(m.View(), "Aucune session Claude en cours.") {
		t.Errorf("selected=%d view=%q", m.selected, m.View())
	}
	m = feed(m, key("j"), key("1"), loaded(busy(7, "zzz")))
	if m.selected != 7 {
		t.Errorf("selected=%d", m.selected)
	}
}

func TestMoveBounds(t *testing.T) {
	m := newTest(t, busy(1, "aaa"), busy(2, "bbb"))
	m = feed(m, key("up"))
	if m.selected != 1 {
		t.Errorf("up at top: %d", m.selected)
	}
	m = feed(m, key("down"), key("down"))
	if m.selected != 2 {
		t.Errorf("down at bottom: %d", m.selected)
	}
	m = feed(m, key("k"))
	if m.selected != 1 {
		t.Errorf("k: %d", m.selected)
	}
}

func TestMoveWithEmptyList(t *testing.T) {
	m := newTest(t)
	m = feed(m, key("j"), key("k"), key("3"))
	if m.selected != 0 {
		t.Errorf("selected=%d", m.selected)
	}
}

func TestDigits(t *testing.T) {
	m := newTest(t, busy(1, "aaa"), busy(2, "bbb"), busy(3, "ccc"))
	m = feed(m, key("3"))
	if m.selected != 3 {
		t.Errorf("3: %d", m.selected)
	}
	m = feed(m, key("9"))
	if m.selected != 3 {
		t.Errorf("out of range changed selection: %d", m.selected)
	}
	m = feed(m, key("0"))
	if m.selected != 3 {
		t.Errorf("0 changed selection: %d", m.selected)
	}
	m = feed(m, key("1"))
	if m.selected != 1 {
		t.Errorf("1: %d", m.selected)
	}
}

func TestDigitsDoNotJump(t *testing.T) {
	m := newTest(t, busy(1, "aaa"), busy(2, "bbb"))
	_, cmd := m.Update(key("2"))
	if cmd != nil {
		t.Error("digit returned a command")
	}
}

func TestQuitKeys(t *testing.T) {
	m := newTest(t, busy(1, "aaa"))
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		if _, cmd := m.Update(key(k)); !isQuit(cmd) {
			t.Errorf("%s did not quit", k)
		}
	}
}

func TestEnterJumpSuccessQuits(t *testing.T) {
	m := newTest(t, busy(4, "aaa"))
	var gotClient, gotPane string
	var gotPID int
	m.jump = func(c, p string, pid int) error { gotClient, gotPane, gotPID = c, p, pid; return nil }
	_, cmd := m.Update(key("enter"))
	msg := cmd()
	done, ok := msg.(jumpDoneMsg)
	if !ok || done.err != nil || gotClient != "cli" || gotPane != "%4" || gotPID != 4 {
		t.Fatalf("msg=%v client=%q pane=%q pid=%d", msg, gotClient, gotPane, gotPID)
	}
	if _, cmd := m.Update(msg); !isQuit(cmd) {
		t.Error("success did not quit")
	}
}

func TestEnterJumpErrorStaysOpen(t *testing.T) {
	m := newTest(t, busy(4, "aaa"))
	next, cmd := m.Update(jumpDoneMsg{err: errors.New("session hors tmux")})
	m = next.(Model)
	if cmd != nil || !strings.Contains(m.View(), "session hors tmux") {
		t.Fatalf("cmd=%v view=%q", cmd, m.View())
	}
	m = feed(m, loaded(busy(4, "aaa")))
	if !strings.Contains(m.View(), "session hors tmux") {
		t.Error("jump error cleared by a load")
	}
	m = feed(m, key("j"))
	if strings.Contains(m.View(), "session hors tmux") || !strings.Contains(m.View(), "Entrée aller") {
		t.Errorf("jump error not cleared by a key: %q", m.View())
	}
}

func TestEnterWithEmptyList(t *testing.T) {
	m := newTest(t)
	if _, cmd := m.Update(key("enter")); cmd != nil {
		t.Error("enter on empty list returned a command")
	}
}

func TestLoadErrorShownThenCleared(t *testing.T) {
	m := newTest(t, busy(1, "aaa"))
	m = feed(m, loadedMsg{err: errors.New("lecture impossible")})
	v := m.View()
	if !strings.Contains(v, "lecture impossible") || !strings.Contains(v, "aaa") {
		t.Errorf("view=%q", v)
	}
	m = feed(m, loaded(busy(1, "aaa")))
	if strings.Contains(m.View(), "lecture impossible") {
		t.Error("load error not cleared")
	}
}

func TestJumpErrorWinsOverLoadError(t *testing.T) {
	m := newTest(t, busy(1, "aaa"))
	m = feed(m, loadedMsg{err: errors.New("load boom")}, jumpDoneMsg{err: errors.New("jump boom")})
	v := m.View()
	if !strings.Contains(v, "jump boom") || strings.Contains(v, "load boom") {
		t.Errorf("view=%q", v)
	}
}

func many(n int) []registry.Session {
	var ss []registry.Session
	for i := 1; i <= n; i++ {
		ss = append(ss, busy(i, fmt.Sprintf("s%02d", i)))
	}
	return ss
}

func TestScrolling(t *testing.T) {
	m := newTest(t, many(10)...)
	m = feed(m, tea.WindowSizeMsg{Width: 80, Height: 8}) // 3 visible
	if m.visibleCount() != 3 || m.offset != 0 {
		t.Fatalf("visible=%d offset=%d", m.visibleCount(), m.offset)
	}
	m = feed(m, key("j"), key("j"), key("j"))
	if m.offset != 1 {
		t.Errorf("offset=%d", m.offset)
	}
	v := m.View()
	if strings.Contains(v, "s01") || !strings.Contains(v, "s04") {
		t.Errorf("view=%q", v)
	}
	m = feed(m, key("1"))
	if m.offset != 0 {
		t.Errorf("offset after jump to first: %d", m.offset)
	}
	m = feed(m, key("9"))
	if m.offset != 6 {
		t.Errorf("offset after rank 9: %d", m.offset)
	}
}

func TestScrollingTinyHeight(t *testing.T) {
	m := newTest(t, many(4)...)
	m = feed(m, tea.WindowSizeMsg{Width: 80, Height: 3}, key("j"), key("j"))
	if m.visibleCount() != 1 || m.offset != 2 {
		t.Errorf("visible=%d offset=%d", m.visibleCount(), m.offset)
	}
	for _, h := range []int{1, 2, 3} {
		m = feed(m, tea.WindowSizeMsg{Width: 80, Height: h})
		if n := len(strings.Split(m.View(), "\n")); n > h {
			t.Errorf("height %d: %d lines", h, n)
		}
	}
}

func TestOffsetShrinksWithList(t *testing.T) {
	m := newTest(t, many(10)...)
	m = feed(m, tea.WindowSizeMsg{Width: 80, Height: 8}, key("9"), loaded(many(3)...))
	if m.offset != 0 {
		t.Errorf("offset=%d", m.offset)
	}
}

func TestViewLayout(t *testing.T) {
	m := newTest(t, busy(1, "aaa"), idle(2, "bbb"))
	want := "1 ● aaa  travaille  5m  ~/aaa\n" +
		"  └ act aaa\n" +
		"2 ○ bbb  fini  5m  ~/bbb\n" +
		"  └ act bbb\n" +
		footerHelp
	if got := m.View(); got != want {
		t.Errorf("got\n%q\nwant\n%q", got, want)
	}
	if m.View() != m.View() {
		t.Error("View not deterministic")
	}
}

func TestViewEmptyActivityAndRankBeyondNine(t *testing.T) {
	ss := many(10)
	msg := loaded(ss...)
	msg.activity[10] = ""
	m := feed(newTest(t), tea.WindowSizeMsg{Width: 80, Height: 100}, msg)
	lines := strings.Split(m.View(), "\n")
	if !strings.HasPrefix(lines[18], "  ● s10") || lines[19] != "" {
		t.Errorf("lines: %q %q", lines[18], lines[19])
	}
}

func TestViewTruncatesMultiByte(t *testing.T) {
	s := busy(1, "éé")
	msg := loaded(s)
	msg.activity[1] = strings.Repeat("日本語", 20)
	m := feed(newTest(t), tea.WindowSizeMsg{Width: 20, Height: 10}, msg)
	for _, l := range strings.Split(m.View(), "\n") {
		if w := ansi.StringWidth(l); w > 20 {
			t.Errorf("line too wide (%d): %q", w, l)
		}
	}
	if !strings.Contains(m.View(), "…") {
		t.Error("no ellipsis")
	}
}

func TestDefaultWidthBeforeSizeMessage(t *testing.T) {
	m := newModel(nil, nil, "", "")
	if m.width != 80 {
		t.Errorf("width=%d", m.width)
	}
	m = feed(m, loaded(busy(1, "aaa")))
	if !strings.Contains(m.View(), "aaa") {
		t.Errorf("view=%q", m.View())
	}
}

func TestSelectedLineIsStyled(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
	m := newTest(t, busy(1, "aaa"), busy(2, "bbb"))
	lines := strings.Split(m.View(), "\n")
	reversed := func(line string) bool {
		return strings.Contains(line, "\x1b[7m") || strings.Contains(line, "\x1b[7;")
	}
	if !reversed(lines[0]) || !strings.Contains(lines[0], "aaa") {
		t.Errorf("selected line not reversed: %q", lines[0])
	}
	if reversed(lines[2]) || !strings.Contains(lines[2], "\x1b[") {
		t.Errorf("other line: %q", lines[2])
	}
}

type fakeSource struct{ retained [][]string }

func (f *fakeSource) Last(string) transcript.Activity { return transcript.Activity{} }
func (f *fakeSource) Retain(ids []string)             { f.retained = append(f.retained, ids) }

func TestLoaderRetainsCurrentSessions(t *testing.T) {
	dir := t.TempDir()
	sd := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	pid := os.Getpid()
	file := filepath.Join(sd, fmt.Sprintf("%d.json", pid))
	body := fmt.Sprintf(`{"pid":%d,"sessionId":"aaaaaaaa-1111","status":"idle"}`, pid)
	if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	src := &fakeSource{}
	load := newLoader(registry.Reader{Dir: dir}, src)
	load()
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	load()
	if len(src.retained) != 2 || len(src.retained[0]) != 1 || src.retained[0][0] != "aaaaaaaa-1111" || len(src.retained[1]) != 0 {
		t.Errorf("retained=%v", src.retained)
	}
}

func TestUnknownMessageIgnored(t *testing.T) {
	m := newTest(t, busy(1, "aaa"))
	next, cmd := m.Update(struct{}{})
	if cmd != nil || next.(Model).selected != 1 {
		t.Error("unknown message changed the model")
	}
}

func TestNewLoader(t *testing.T) {
	dir := t.TempDir()
	if msg := NewLoader(dir)(); msg.err != nil || len(msg.sessions) != 0 || msg.now.IsZero() {
		t.Errorf("empty dir: %+v", msg)
	}

	pid := os.Getpid()
	sd := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	sid := "75f334f0-4dc9-41db-85e2-1d96e0dbaaf1"
	body := fmt.Sprintf(`{"pid":%d,"sessionId":%q,"name":"demo","status":"busy","cwd":"/x"}`, pid, sid)
	if err := os.WriteFile(filepath.Join(sd, fmt.Sprintf("%d.json", pid)), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	tdir := filepath.Join(dir, "projects", "p")
	if err := os.MkdirAll(tdir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"make"}}]}}` + "\n" +
		`{"type":"system","subtype":"turn_duration"}` + "\n"
	if err := os.WriteFile(filepath.Join(tdir, sid+".jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	msg := NewLoader(dir)()
	if msg.err != nil || len(msg.sessions) != 1 {
		t.Fatalf("msg=%+v", msg)
	}
	if !msg.sessions[0].TurnEnded || msg.activity[pid] != "Bash: make" {
		t.Errorf("turnEnded=%v activity=%q", msg.sessions[0].TurnEnded, msg.activity[pid])
	}
}

func TestNewLoaderUnreadableDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores permissions")
	}
	dir := t.TempDir()
	sd := filepath.Join(dir, "sessions")
	if err := os.Mkdir(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sd, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(sd, 0o755)
	if msg := NewLoader(dir)(); msg.err == nil {
		t.Error("expected an error")
	}
}

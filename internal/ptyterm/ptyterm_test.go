package ptyterm

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"claude-tower/internal/tmux"

	"github.com/creack/pty"
)

func open(t *testing.T, m *Manager, pid int) (int, string) {
	t.Helper()
	id, err := m.Open(context.Background(), pid, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	return id, strconv.Itoa(id)
}

func TestNothingEmittedBeforeReady(t *testing.T) {
	srv := testServer(t)
	m, rec := newManager(t, srv, os.Environ())
	id, sid := open(t, m, 1)
	waitFor(t, "output held", func() bool {
		held, _ := heldOutput(m, id)
		return held > 0
	})
	if ev := rec.snapshot(); len(ev) != 0 {
		t.Fatalf("events before Ready: %d", len(ev))
	}
	if err := m.Ready(id); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "buffered output", func() bool {
		out, _ := rec.output(sid)
		return out != ""
	})
}

func TestShellEchoesComputation(t *testing.T) {
	srv := testServer(t)
	m, rec := newManager(t, srv, os.Environ())
	id, sid := open(t, m, 1)
	m.Ready(id)
	waitFor(t, "first screen", func() bool {
		out, _ := rec.output(sid)
		return strings.Contains(out, "$")
	})
	before, _ := rec.output(sid)
	if err := m.Input(id, "echo $((6*7))\r"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "result 42", func() bool {
		out, _ := rec.output(sid)
		return strings.Contains(csi.ReplaceAllString(out[len(before):], ""), "42")
	})
}

func TestResizeReachesTmuxClient(t *testing.T) {
	srv := testServer(t)
	m, _ := newManager(t, srv, os.Environ())
	id, _ := open(t, m, 1)
	m.Ready(id)
	if err := m.Resize(id, 100, 30); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "client size 100x30", func() bool {
		out, err := srv.Exec(context.Background(), nil, "list-clients", "-F", "#{client_width}x#{client_height}")
		return err == nil && strings.TrimSpace(string(out)) == "100x30"
	})
	if err := m.Resize(id, 0, 30); err == nil {
		t.Error("Resize accepted a zero width")
	}
}

func TestCloseDestroysViewAndRestoresActivePane(t *testing.T) {
	srv := testServer(t)
	ps := panes(t, srv)
	m, rec := newManager(t, srv, os.Environ())
	id, sid := open(t, m, 2)
	m.Ready(id)
	waitFor(t, "attached client", func() bool {
		out, _ := rec.output(sid)
		return out != ""
	})
	active := func() string {
		out, _ := srv.Exec(context.Background(), nil, "display-message", "-p", "-t", "main:0", "#{pane_id}")
		return strings.TrimSpace(string(out))
	}
	if active() != ps[2].ID {
		t.Fatalf("view did not select the pane: active %s", active())
	}
	if err := m.Close(id); err != nil {
		t.Fatal(err)
	}
	if v := viewSessions(srv); len(v) != 0 {
		t.Errorf("view sessions left: %v", v)
	}
	if active() != ps[1].ID {
		t.Errorf("active pane %s, want %s", active(), ps[1].ID)
	}
	if err := m.Close(id); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// killClient kills the tmux client of the view session.
func killClient(t *testing.T, srv tmux.Server) {
	t.Helper()
	out, err := srv.Exec(context.Background(), nil, "list-clients", "-F", "#{client_pid}")
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("client pid %q", out)
	}
	if err := syscall.Kill(pid, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
}

func TestKillingClientDestroysViewAndEmitsExit(t *testing.T) {
	srv := testServer(t)
	m, rec := newManager(t, srv, os.Environ())
	id, sid := open(t, m, 1)
	m.Ready(id)
	waitFor(t, "attached client", func() bool {
		out, _ := rec.output(sid)
		return out != ""
	})
	killClient(t, srv)
	waitFor(t, "view destroyed", func() bool { return len(viewSessions(srv)) == 0 })
	waitFor(t, "exit event", func() bool {
		_, exited := rec.output(sid)
		return exited
	})
	if err := m.Close(id); err != nil {
		t.Errorf("Close after the client died: %v", err)
	}
}

func TestExitBeforeReadyIsEmittedAfterBufferedOutput(t *testing.T) {
	srv := testServer(t)
	m, rec := newManager(t, srv, os.Environ())
	id, sid := open(t, m, 1)
	waitFor(t, "output held", func() bool {
		held, _ := heldOutput(m, id)
		return held > 0
	})
	killClient(t, srv)
	waitFor(t, "view destroyed", func() bool { return len(viewSessions(srv)) == 0 })
	waitFor(t, "exit held", func() bool {
		_, eof := heldOutput(m, id)
		return eof
	})
	if ev := rec.snapshot(); len(ev) != 0 {
		t.Fatalf("events before Ready: %d", len(ev))
	}
	m.Ready(id)
	waitFor(t, "exit event", func() bool {
		out, exited := rec.output(sid)
		return exited && out != ""
	})
}

func TestEmptyEnvironmentWorks(t *testing.T) {
	srv := testServer(t)
	m, rec := newManager(t, srv, []string{})
	id, sid := open(t, m, 1)
	m.Ready(id)
	waitFor(t, "output", func() bool {
		out, _ := rec.output(sid)
		return out != ""
	})
	if _, exited := rec.output(sid); exited {
		t.Error("the client exited with an empty environment")
	}
}

func TestOpenFailureCleansUpView(t *testing.T) {
	srv := testServer(t)
	m, _ := newManager(t, srv, os.Environ())
	m.start = func(*exec.Cmd, *pty.Winsize) (*os.File, error) { return nil, errors.New("no pty") }
	if _, err := m.Open(context.Background(), 2, 80, 24); err == nil {
		t.Fatal("Open succeeded")
	}
	if v := viewSessions(srv); len(v) != 0 {
		t.Errorf("view sessions left: %v", v)
	}
	out, _ := srv.Exec(context.Background(), nil, "display-message", "-p", "-t", "main:0", "#{pane_id}")
	first := panes(t, srv)[1].ID
	if strings.TrimSpace(string(out)) != first {
		t.Errorf("active pane %q, want %q", out, first)
	}
}

func TestOpenRejectsUnknownSessionAndBadSize(t *testing.T) {
	srv := testServer(t)
	m, _ := newManager(t, srv, os.Environ())
	if _, err := m.Open(context.Background(), 99, 80, 24); err == nil {
		t.Error("unknown pid accepted")
	}
	if _, err := m.Open(context.Background(), 1, 0, 24); err == nil {
		t.Error("zero width accepted")
	}
	if _, err := m.Open(context.Background(), 1, 80, maxCells+1); err == nil {
		t.Error("oversized height accepted")
	}
	if _, err := m.Open(context.Background(), 1, maxCells+1, 24); err == nil {
		t.Error("oversized width accepted")
	}
	if err := m.Resize(7, 70000, 24); err == nil {
		t.Error("oversized resize accepted")
	}
	if err := m.Ready(7); err == nil {
		t.Error("Ready accepted an unknown terminal")
	}
	if err := m.Input(7, "x"); err == nil {
		t.Error("Input accepted an unknown terminal")
	}
}

func TestTerminalEnv(t *testing.T) {
	got := terminalEnv([]string{"PATH=/bin", "TERM=dumb", "TMUX=/tmp/x,1,0", "TMUX_PANE=%1", "COLORTERM=no"})
	want := map[string]bool{"PATH=/bin": true, "TERM=xterm-256color": true, "COLORTERM=truecolor": true, "LANG=C.UTF-8": true}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for _, e := range got {
		if !want[e] {
			t.Errorf("unexpected %q in %v", e, got)
		}
	}
	keep := terminalEnv([]string{"LANG=fr_FR.UTF-8"})
	if strings.Join(keep, " ") != "LANG=fr_FR.UTF-8 TERM=xterm-256color COLORTERM=truecolor" {
		t.Errorf("a UTF-8 locale changed: %v", keep)
	}
}

func TestOpenAfterCloseAllIsRefusedWithoutView(t *testing.T) {
	srv := testServer(t)
	m, _ := newManager(t, srv, os.Environ())
	m.CloseAll()
	if _, err := m.Open(context.Background(), 1, 80, 24); !errors.Is(err, errClosed) {
		t.Fatalf("got %v", err)
	}
	if v := viewSessions(srv); len(v) != 0 {
		t.Errorf("view sessions: %v", v)
	}
}

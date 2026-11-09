package ptyterm

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"claude-tower/internal/tmux"
)

func TestMain(m *testing.M) {
	os.Unsetenv("TMUX")
	os.Exit(m.Run())
}

// recorder is an Emitter that keeps every event.
type recorder struct {
	mu     sync.Mutex
	events []event
}

type event struct {
	name string
	data any
}

func (r *recorder) Emit(name string, data any) {
	r.mu.Lock()
	r.events = append(r.events, event{name, data})
	r.mu.Unlock()
}

func (r *recorder) snapshot() []event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]event(nil), r.events...)
}

// output returns the bytes of the data events of a terminal, and whether its
// exit event came last.
func (r *recorder) output(id string) (string, bool) {
	var sb strings.Builder
	exited := false
	for _, e := range r.snapshot() {
		switch e.name {
		case "term:" + id + ":data":
			if exited {
				return sb.String(), false
			}
			b, _ := base64.StdEncoding.DecodeString(e.data.(string))
			sb.Write(b)
		case "term:" + id + ":exit":
			exited = true
		}
	}
	return sb.String(), exited
}

var csi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

// paneSource returns the panes of one server by the PID they are registered
// under.
type paneSource map[int]tmux.Pane

func (p paneSource) PaneOf(pid int) (tmux.Pane, bool) {
	pane, ok := p[pid]
	return pane, ok
}

func requireTmux(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not installed")
	}
	return bin
}

// testServer starts a tmux server on a private socket with the session "main"
// running a shell, split in two panes with the first one active.
func testServer(t *testing.T) tmux.Server {
	t.Helper()
	bin := requireTmux(t)
	dir, err := os.MkdirTemp("/tmp", "ct")
	if err != nil {
		t.Fatal(err)
	}
	srv := tmux.Server{Socket: filepath.Join(dir, "sock"), Bin: bin, Run: tmux.ExecRunner(10 * time.Second)}
	t.Cleanup(func() {
		srv.Exec(context.Background(), nil, "kill-server")
		os.RemoveAll(dir)
	})
	start := [][]string{
		{"-f", "/dev/null", "-S", srv.Socket, "new-session", "-d", "-x", "80", "-y", "24", "-s", "main", "env", "PS1=$ ", "sh"},
		{"-S", srv.Socket, "split-window", "-t", "main:0", "env", "PS1=$ ", "sh"},
		{"-S", srv.Socket, "select-pane", "-t", "main:0.0"},
	}
	for _, args := range start {
		if out, err := exec.Command(bin, args...).CombinedOutput(); err != nil {
			t.Fatalf("tmux %v: %v: %s", args, err, bytes.TrimSpace(out))
		}
	}
	return srv
}

// panes returns the panes of the server keyed by 1 for the first and 2 for
// the second.
func panes(t *testing.T, srv tmux.Server) paneSource {
	t.Helper()
	list, err := srv.Panes(context.Background())
	if err != nil || len(list) != 2 {
		t.Fatalf("panes: %v %v", list, err)
	}
	return paneSource{1: list[0], 2: list[1]}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

// viewSessions lists the names of the view sessions of the server.
func viewSessions(srv tmux.Server) []string {
	out, err := srv.Exec(context.Background(), nil, "list-sessions", "-F", "#{session_name}")
	if err != nil {
		return nil
	}
	var names []string
	for _, n := range strings.Fields(string(out)) {
		if strings.HasPrefix(n, tmux.ViewPrefix) {
			names = append(names, n)
		}
	}
	return names
}

func newManager(t *testing.T, srv tmux.Server, env []string) (*Manager, *recorder) {
	t.Helper()
	rec := &recorder{}
	m := NewManager(panes(t, srv), rec, env)
	t.Cleanup(m.CloseAll)
	return m, rec
}

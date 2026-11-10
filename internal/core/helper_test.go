package core

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"claude-tower/internal/hosting"
	"claude-tower/internal/locate"
	"claude-tower/internal/tmux"
)

var testNow = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

// fakeTmux answers tmux commands from tables, per socket.
type fakeTmux struct {
	mu      sync.Mutex
	calls   [][]string // every call: the socket, then the command words
	stdin   []string
	version string
	panes   map[string]string
	clients map[string]string
	capture map[string]string
	sizes   map[string]string // live size of a pane, "<width>\t<height>"; default "80\t24"
	listing map[string]string // answer to list-sessions, per socket
	shell   string            // output of the login shell
	shells  int               // number of runs of the login shell
	failing map[string]bool
}

func (f *fakeTmux) run(_ context.Context, stdin io.Reader, _ string, args ...string) ([]byte, error) {
	if len(args) == 1 && args[0] == "-V" {
		return []byte("tmux " + f.version + "\n"), nil
	}
	if args[0] != "-S" {
		if len(args) == 2 && args[0] == "-ilc" && args[1] == "env -0" {
			f.mu.Lock()
			f.shells++
			f.mu.Unlock()
			return []byte(f.shell), nil
		}
		return nil, nil
	}
	var in string
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		in = string(b)
	}
	sock, cmd := args[1], args[2:]
	f.mu.Lock()
	f.calls = append(f.calls, append([]string{sock}, cmd...))
	f.stdin = append(f.stdin, in)
	f.mu.Unlock()
	if f.failing[sock] {
		return nil, fmt.Errorf("no server running on %s", sock)
	}
	switch cmd[0] {
	case "list-panes":
		return []byte(f.panes[sock]), nil
	case "list-clients":
		return []byte(f.clients[sock]), nil
	case "-f":
		return []byte("$3\t%5\n"), nil
	case "list-sessions":
		return []byte(f.listing[sock]), nil
	case "capture-pane":
		pane := cmd[indexOf(cmd, "-t")+1]
		size := f.sizes[pane]
		if size == "" {
			size = "80\t24"
		}
		return []byte(f.capture[pane] + size + "\n"), nil
	}
	return nil, nil
}

// commands returns the recorded commands of one socket.
func (f *fakeTmux) commands(sock string) [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out [][]string
	for _, c := range f.calls {
		if c[0] == sock {
			out = append(out, c[1:])
		}
	}
	return out
}

// stdinOf returns the standard input given to the first call of a command.
func (f *fakeTmux) stdinOf(command string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, c := range f.calls {
		if c[1] == command {
			return f.stdin[i]
		}
	}
	return ""
}

func (f *fakeTmux) has(sock, command string) bool {
	for _, c := range f.commands(sock) {
		if c[0] == command {
			return true
		}
	}
	return false
}

type env struct {
	t        *testing.T
	fake     *fakeTmux
	cfgDir   string
	userSock string
	privSock string
	hosting  *hosting.Hosting
	tree     locate.Tree
	alive    map[int]bool
	raised   [][]int
	raiseErr error
	supports bool
}

func listenUnix(t *testing.T, path string) {
	t.Helper()
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
}

// newEnv builds a Core environment with one user socket and one private
// socket, both served by a fake tmux.
func newEnv(t *testing.T) *env {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "ct")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	userDir := filepath.Join(root, "user")
	privDir := filepath.Join(root, "priv")
	for _, d := range []string{userDir, privDir, filepath.Join(root, "claude", "sessions")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	e := &env{
		t:        t,
		fake:     &fakeTmux{version: "3.8", panes: map[string]string{}, clients: map[string]string{}, capture: map[string]string{}, sizes: map[string]string{}, listing: map[string]string{}, failing: map[string]bool{}},
		cfgDir:   filepath.Join(root, "claude"),
		userSock: filepath.Join(userDir, "default"),
		privSock: filepath.Join(privDir, "tmux.sock"),
		tree:     locate.Tree{},
		alive:    map[int]bool{},
		supports: true,
	}
	listenUnix(t, e.userSock)
	listenUnix(t, e.privSock)
	e.hosting = &hosting.Hosting{
		SocketDir: privDir, ConfigDir: filepath.Join(root, "cfg"), Self: "/opt/claude-tower",
		Tmux: e.fake.run, Bin: "tmux", Env: []string{"TERM=xterm", "KEEP=1"}, ClaudeConfigDir: "/app/claude", SessionConfigDir: "/app/claude",
		Shell: "/bin/fake-shell", ShellRun: e.fake.run,
	}
	return e
}

func (e *env) core(opts ...func(*Options)) *Core {
	e.t.Helper()
	o := Options{
		Hosting:       e.hosting,
		Home:          "/home/u",
		UserSocketDir: filepath.Dir(e.userSock),
		Bin:           "tmux",
		Run:           e.fake.run,
		ReadTree:      func(context.Context, tmux.Runner) (locate.Tree, error) { return e.tree, nil },
		Raise:         func(pids []int) error { e.raised = append(e.raised, pids); return e.raiseErr },
		Supported:     func() bool { return e.supports },
		Now:           func() time.Time { return testNow },
	}
	o.Registry.Dir = e.cfgDir
	o.Registry.Alive = func(pid int, _ string) bool { return e.alive[pid] }
	for _, f := range opts {
		f(&o)
	}
	c, err := New(o)
	if err != nil {
		e.t.Fatal(err)
	}
	return c
}

// session writes a registry file and marks the process alive.
func (e *env) session(pid int, fields string) {
	e.t.Helper()
	e.alive[pid] = true
	body := fmt.Sprintf(`{"pid":%d,%s}`, pid, fields)
	path := filepath.Join(e.cfgDir, "sessions", fmt.Sprintf("%d.json", pid))
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) transcript(id string, lines ...string) {
	e.t.Helper()
	p := filepath.Join(e.cfgDir, "projects", "slug", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

func paneLine(id string, pid int, sessionID, name, window string, group string) string {
	return fmt.Sprintf("%s\t%d\t%s\t%s\t%s\t1\t%s\t80\t24\n", id, pid, sessionID, name, window, group)
}

func clientLine(name string, pid int, sessionID, sessionName, group string, activity int64) string {
	return fmt.Sprintf("%s\t%d\t%s\t%s\t%s\t%d\n", name, pid, sessionID, sessionName, group, activity)
}

const (
	idA = "aaaaaaaa-0000-0000-0000-000000000001"
	idB = "bbbbbbbb-0000-0000-0000-000000000002"
	idC = "cccccccc-0000-0000-0000-000000000003"
)

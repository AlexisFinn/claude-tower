package hosting

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type call struct {
	name  string
	args  []string
	stdin string
}

// recorder is a Runner that records every call and answers from a function.
type recorder struct {
	mu    sync.Mutex
	calls []call
	reply func(name string, args []string) ([]byte, error)
}

func (r *recorder) run(_ context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	var in string
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		in = string(b)
	}
	r.mu.Lock()
	r.calls = append(r.calls, call{name: name, args: args, stdin: in})
	r.mu.Unlock()
	if r.reply != nil {
		return r.reply(name, args)
	}
	return nil, nil
}

// tmuxCommands returns the tmux command words of the recorded calls, without
// the leading -S socket.
func (r *recorder) tmuxCommands() [][]string {
	var out [][]string
	for _, c := range r.calls {
		if isTmux(c.name) {
			out = append(out, c.args[2:])
		}
	}
	return out
}

// newHosting returns a Hosting confined to temporary directories.
func newHosting(t *testing.T, rec *recorder) *Hosting {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "ct")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sockDir := filepath.Join(dir, "run")
	return &Hosting{
		SocketDir: sockDir,
		ConfigDir: filepath.Join(dir, "cfg"),
		Self:      "/opt/claude-tower",
		Tmux:      rec.run,
		Bin:       "tmux",
		Env:       []string{"PATH=/nonexistent", "TERM=xterm", "SHELL=/bin/fake-shell"},
		Shell:     "/bin/fake-shell",
		ShellRun:  rec.run,
	}
}

func writeExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeConfig(t *testing.T, h *Hosting, body string) {
	t.Helper()
	if err := os.MkdirAll(h.ConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.ConfigDir, "config.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func join(args []string) string { return strings.Join(args, " ") }

func isTmux(name string) bool { return filepath.Base(name) == "tmux" }

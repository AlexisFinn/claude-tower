package tmux

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	auxEnv    = "CLAUDE_TOWER_TMUX_TEST_AUX"
	auxOutEnv = "CLAUDE_TOWER_TMUX_TEST_OUT"
)

// TestMain lets the test binary act as the program running inside test panes:
// "rawpaste" enables bracketed paste in raw mode and appends every received
// byte to a file, "sleep" blocks until killed.
func TestMain(m *testing.M) {
	os.Unsetenv("TMUX")
	switch os.Getenv(auxEnv) {
	case "rawpaste":
		os.Exit(auxRawPaste(os.Getenv(auxOutEnv)))
	case "sleep":
		select {}
	}
	os.Exit(m.Run())
}

func auxRawPaste(out string) int {
	stty := exec.Command("stty", "raw", "-echo")
	stty.Stdin = os.Stdin
	if err := stty.Run(); err != nil {
		return 1
	}
	f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return 1
	}
	fmt.Print("\x1b[?2004h")
	time.Sleep(150 * time.Millisecond)
	if err := os.WriteFile(out+".ready", nil, 0o600); err != nil {
		return 1
	}
	buf := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			f.Write(buf[:n])
		}
		if err != nil {
			return 0
		}
	}
}

type call struct {
	args  []string
	stdin string
}

// recorder is a Runner that records every call and answers from a function.
type recorder struct {
	mu    sync.Mutex
	calls []call
	reply func(args []string) ([]byte, error)
}

func (r *recorder) run(_ context.Context, stdin io.Reader, _ string, args ...string) ([]byte, error) {
	var in string
	if stdin != nil {
		b, _ := io.ReadAll(stdin)
		in = string(b)
	}
	r.mu.Lock()
	r.calls = append(r.calls, call{args: args, stdin: in})
	r.mu.Unlock()
	if r.reply != nil {
		return r.reply(args)
	}
	return nil, nil
}

func (r *recorder) server() Server {
	return Server{Socket: "/s/sock", Bin: "tmux", Run: r.run}
}

// commands returns the tmux command words, without the leading -S socket.
func (r *recorder) commands() [][]string {
	var out [][]string
	for _, c := range r.calls {
		out = append(out, c.args[2:])
	}
	return out
}

func requireTmux(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not installed")
	}
	return bin
}

// testServer starts a tmux server on a private socket with one session
// "main" running the test binary in the given auxiliary mode.
func testServer(t *testing.T, aux string, extraEnv ...string) Server {
	t.Helper()
	bin := requireTmux(t)
	dir, err := os.MkdirTemp("/tmp", "ct")
	if err != nil {
		t.Fatal(err)
	}
	srv := Server{Socket: filepath.Join(dir, "sock"), Bin: bin, Run: ExecRunner(10 * time.Second)}
	t.Cleanup(func() {
		srv.Exec(context.Background(), nil, "kill-server")
		os.RemoveAll(dir)
	})
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	args := []string{"-f", "/dev/null", "-S", srv.Socket, "new-session", "-d", "-x", "80", "-y", "24", "-s", "main",
		"env", auxEnv + "=" + aux}
	args = append(args, extraEnv...)
	args = append(args, self)
	if out, err := exec.Command(bin, args...).CombinedOutput(); err != nil {
		t.Fatalf("start tmux: %v: %s", err, bytes.TrimSpace(out))
	}
	return srv
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", what)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readFile(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

func mustExec(t *testing.T, s Server, args ...string) string {
	t.Helper()
	out, err := s.Exec(context.Background(), nil, args...)
	if err != nil {
		t.Fatalf("tmux %s: %v", strings.Join(args, " "), err)
	}
	return string(out)
}

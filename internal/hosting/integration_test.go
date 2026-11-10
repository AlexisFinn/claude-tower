package hosting_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"claude-tower/internal/cli"
	"claude-tower/internal/hosting"
	"claude-tower/internal/tmux"
)

const (
	auxEnv    = "CLAUDE_TOWER_HOSTING_TEST_AUX"
	auxOutEnv = "CLAUDE_TOWER_HOSTING_TEST_OUT"
)

// TestMain lets the test binary play two roles inside test panes: the
// claude-tower binary (exec-env) and, in the "writeenv" mode, a stand-in for
// claude that records its environment.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "exec-env" {
		os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
	}
	if os.Getenv(auxEnv) == "writeenv" {
		out := os.Getenv(auxOutEnv)
		data := strings.Join(os.Environ(), "\x00")
		if err := os.WriteFile(out+".tmp", []byte(data), 0o600); err != nil {
			os.Exit(1)
		}
		if err := os.Rename(out+".tmp", out); err != nil {
			os.Exit(1)
		}
		time.Sleep(time.Hour)
	}
	os.Unsetenv("TMUX")
	os.Exit(m.Run())
}

type fixture struct {
	h   *hosting.Hosting
	srv tmux.Server
	dir string
}

func newFixture(t *testing.T, claudeCommand string) fixture {
	t.Helper()
	bin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not installed")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp("/tmp", "ct")
	if err != nil {
		t.Fatal(err)
	}
	h := &hosting.Hosting{
		SocketDir:         filepath.Join(dir, "run"),
		ConfigDir:         filepath.Join(dir, "cfg"),
		Self:              self,
		Tmux:              tmux.ExecRunner(10 * time.Second),
		Bin:               bin,
		Env:               []string{"PATH=" + os.Getenv("PATH"), "TERM=xterm"},
		StartupCheckDelay: time.Second,
	}
	if err := os.MkdirAll(h.ConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`{"claudeCommand":[%q]}`, claudeCommand)
	if err := os.WriteFile(filepath.Join(h.ConfigDir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	sock, err := h.PrivateSocket()
	if err != nil {
		t.Fatal(err)
	}
	srv := tmux.Server{Socket: sock, Bin: bin, Run: h.Tmux}
	t.Cleanup(func() {
		srv.Exec(context.Background(), nil, "kill-server")
		os.RemoveAll(dir)
	})
	return fixture{h: h, srv: srv, dir: dir}
}

func envFiles(f fixture) []string {
	entries, _ := os.ReadDir(f.h.SocketDir)
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "env-") {
			out = append(out, e.Name())
		}
	}
	return out
}

func waitForFile(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil {
			return data
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", path)
	return nil
}

func TestNewSessionDeliversEnvironment(t *testing.T) {
	self, _ := os.Executable()
	f := newFixture(t, self)
	out := filepath.Join(f.dir, "env.out")
	env := []string{"PATH=" + os.Getenv("PATH"), "FROM_CALLER=hello world", auxEnv + "=writeenv", auxOutEnv + "=" + out}
	workdir := filepath.Join(f.dir, "my.proj")
	if err := os.Mkdir(workdir, 0o755); err != nil {
		t.Fatal(err)
	}

	pane, err := f.h.NewSession(context.Background(), workdir, nil, env)
	if err != nil {
		t.Fatal(err)
	}
	if pane.SessionName != "my-proj" || pane.ID == "" {
		t.Errorf("pane: %+v", pane)
	}
	got := strings.Split(string(waitForFile(t, out)), "\x00")
	has := func(prefix string) bool {
		for _, e := range got {
			if strings.HasPrefix(e, prefix) && len(e) > len(prefix) {
				return true
			}
		}
		return false
	}
	if !contains(got, "FROM_CALLER=hello world") {
		t.Error("variable of the caller missing")
	}
	for _, name := range []string{"TERM=", "TMUX=", "TMUX_PANE="} {
		if !has(name) {
			t.Errorf("%s missing from the session environment", name)
		}
	}
	if files := envFiles(f); len(files) != 0 {
		t.Errorf("environment files left: %v", files)
	}

	if err := f.h.Kill(context.Background(), pane); err != nil {
		t.Fatal(err)
	}
	if panes, err := f.srv.Panes(context.Background()); err == nil && len(panes) != 0 {
		t.Errorf("session still there: %+v", panes)
	}
}

func TestKillStopsViewSessionCreatedAfterTheSnapshot(t *testing.T) {
	self, _ := os.Executable()
	f := newFixture(t, self)
	ctx := context.Background()
	out := filepath.Join(f.dir, "env.out")
	env := []string{"PATH=" + os.Getenv("PATH"), auxEnv + "=writeenv", auxOutEnv + "=" + out}
	pane, err := f.h.NewSession(ctx, f.dir, nil, env)
	if err != nil {
		t.Fatal(err)
	}
	if pane.SessionGroup != "" {
		t.Fatalf("session already grouped: %+v", pane)
	}
	if _, err := f.srv.NewViewSession(ctx, pane); err != nil {
		t.Fatal(err)
	}

	if err := f.h.Kill(ctx, pane); err != nil {
		t.Fatal(err)
	}
	if panes, err := f.srv.Panes(ctx); err == nil && len(panes) != 0 {
		t.Errorf("pane still shown by a view session: %+v", panes)
	}
	if names, err := f.srv.Exec(ctx, nil, "list-sessions", "-F", "#{session_name}"); err == nil && strings.TrimSpace(string(names)) != "" {
		t.Errorf("sessions left: %q", names)
	}
}

func TestNewSessionReportsImmediateExit(t *testing.T) {
	falseBin, err := exec.LookPath("false")
	if err != nil {
		t.Skip("false not found")
	}
	f := newFixture(t, falseBin)
	_, err = f.h.NewSession(context.Background(), f.dir, nil, []string{"PATH=" + os.Getenv("PATH")})
	if err == nil || !strings.Contains(err.Error(), "arrêtée immédiatement") {
		t.Fatalf("err=%v", err)
	}
	if files := envFiles(f); len(files) != 0 {
		t.Errorf("environment files left: %v", files)
	}
}

func TestEnsureConfigReloadsRunningServer(t *testing.T) {
	self, _ := os.Executable()
	f := newFixture(t, self)
	out := filepath.Join(f.dir, "env.out")
	env := []string{"PATH=" + os.Getenv("PATH"), auxEnv + "=writeenv", auxOutEnv + "=" + out}
	if _, err := f.h.NewSession(context.Background(), f.dir, nil, env); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.h.ConfigDir, "tmux.conf"), []byte("set -g mouse off\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.h.EnsureConfig(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := f.srv.Exec(context.Background(), nil, "show-options", "-gv", "mouse")
	if err != nil || strings.TrimSpace(string(got)) != "on" {
		t.Errorf("mouse=%q err=%v", got, err)
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

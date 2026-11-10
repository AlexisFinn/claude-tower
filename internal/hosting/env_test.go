package hosting

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestSessionEnv(t *testing.T) {
	base := []string{"PATH=/bin", "TMUX=/s,1,0", "TMUX_PANE=%1", "TERM=xterm", "TERM_PROGRAM=x",
		"TERM_PROGRAM_VERSION=1", "COLUMNS=80", "LINES=24", "FOO=bar", "CLAUDE_CONFIG_DIR=/caller"}
	got := SessionEnv(base, "")
	want := []string{"PATH=/bin", "FOO=bar", "CLAUDE_CONFIG_DIR=/caller"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	got = SessionEnv(base, "/app")
	want = []string{"PATH=/bin", "FOO=bar", "CLAUDE_CONFIG_DIR=/app"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	if len(base) != 10 {
		t.Error("input modified")
	}
}

func TestWithUTF8Locale(t *testing.T) {
	cases := []struct {
		name string
		env  []string
		goos string
		want []string
	}{
		{"none on linux", []string{"A=1"}, "linux", []string{"A=1", "LANG=C.UTF-8"}},
		{"none on macos", nil, "darwin", []string{"LANG=en_US.UTF-8"}},
		{"non UTF-8 LANG", []string{"LANG=C"}, "linux", []string{"LANG=C.UTF-8"}},
		{"UTF-8 LANG", []string{"LANG=fr_FR.utf8"}, "linux", []string{"LANG=fr_FR.utf8"}},
		{"LC_ALL wins over LANG", []string{"LC_ALL=C", "LANG=en_US.UTF-8"}, "linux", []string{"LANG=en_US.UTF-8", "LC_ALL=C.UTF-8"}},
		{"non UTF-8 LC_ALL and LANG", []string{"LC_ALL=POSIX", "LANG=C"}, "linux", []string{"LC_ALL=C.UTF-8", "LANG=C.UTF-8"}},
		{"non UTF-8 LC_CTYPE", []string{"LC_CTYPE=C", "LANG=fr_FR.UTF-8"}, "linux", []string{"LANG=fr_FR.UTF-8", "LC_CTYPE=C.UTF-8"}},
		{"non UTF-8 LC_ALL over UTF-8 LC_CTYPE", []string{"LC_ALL=C", "LC_CTYPE=en_US.UTF-8"}, "darwin", []string{"LC_CTYPE=en_US.UTF-8", "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8"}},
		{"UTF-8 LC_CTYPE", []string{"LC_CTYPE=en_US.UTF-8"}, "linux", []string{"LC_CTYPE=en_US.UTF-8"}},
		{"UTF-8 LC_ALL", []string{"LC_ALL=en_US.UTF-8", "LANG=C"}, "linux", []string{"LC_ALL=en_US.UTF-8", "LANG=C"}},
	}
	for _, c := range cases {
		input := append([]string(nil), c.env...)
		result := withUTF8Locale(input, c.goos)
		got := append([]string(nil), result...)
		sort.Strings(got)
		want := append([]string(nil), c.want...)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v want %v", c.name, got, want)
		}
		if !utf8Locale(result) {
			t.Errorf("%s: locale in effect is not UTF-8: %v", c.name, result)
		}
		if !reflect.DeepEqual(input, c.env) {
			t.Errorf("%s: input modified: %v", c.name, input)
		}
	}
}

func TestParseEnvBlock(t *testing.T) {
	got := parseEnvBlock([]byte("motd line\n\x00A=1\x00B=two words\x00=bad\x00C=\x00\x00D=x\ny\x00"))
	want := []string{"A=1", "B=two words", "C=", "D=x\ny"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestEnvBlockRoundTrip(t *testing.T) {
	env := []string{"A=1", "B=multi\nline", "C="}
	if got := parseEnvBlock(encodeEnvBlock(env)); !reflect.DeepEqual(got, env) {
		t.Errorf("got %q", got)
	}
	if got := encodeEnvBlock([]string{"NOEQUALS", "A=1"}); string(got) != "A=1\x00" {
		t.Errorf("got %q", got)
	}
}

func TestLoginShellEnvMergesBelowCurrentEnv(t *testing.T) {
	rec := &recorder{reply: func(string, []string) ([]byte, error) {
		return []byte("banner\x00PATH=/login/bin:/usr/bin\x00FOO=login\x00ONLY=shell\x00"), nil
	}}
	h := newHosting(t, rec)
	h.Env = []string{"PATH=/app/bin:/usr/bin::/other", "HOME=/h", "FOO=mine"}
	got, err := h.LoginShellEnv(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	want := []string{"FOO=mine", "HOME=/h", "ONLY=shell", "PATH=/login/bin:/usr/bin:/app/bin:/other"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	c := rec.calls[0]
	if c.name != "/bin/fake-shell" || !reflect.DeepEqual(c.args, []string{"-ilc", "env -0"}) {
		t.Errorf("call: %+v", c)
	}
	if _, err := h.LoginShellEnv(context.Background()); err != nil || len(rec.calls) != 1 {
		t.Errorf("result not cached: calls=%d err=%v", len(rec.calls), err)
	}
}

func TestLoginShellEnvPathWhenOnlyOneSideHasIt(t *testing.T) {
	rec := &recorder{reply: func(string, []string) ([]byte, error) { return []byte("PATH=/login/bin\x00"), nil }}
	h := newHosting(t, rec)
	h.Env = []string{"HOME=/h"}
	if got, _ := h.LoginShellEnv(context.Background()); !contains(got, "PATH=/login/bin") {
		t.Errorf("shell PATH lost: %v", got)
	}
	rec = &recorder{reply: func(string, []string) ([]byte, error) { return []byte("A=1\x00"), nil }}
	h = newHosting(t, rec)
	h.Env = []string{"PATH=/app/bin"}
	if got, _ := h.LoginShellEnv(context.Background()); !contains(got, "PATH=/app/bin") {
		t.Errorf("application PATH lost: %v", got)
	}
}

func TestLoginShellEnvAcceptsLingeringChild(t *testing.T) {
	waitDelay := fmt.Errorf("sh: %w", exec.ErrWaitDelay)
	rec := &recorder{reply: func(string, []string) ([]byte, error) { return []byte("A=1\x00"), waitDelay }}
	h := newHosting(t, rec)
	got, err := h.LoginShellEnv(context.Background())
	if err != nil || !contains(got, "A=1") {
		t.Errorf("got %v err=%v", got, err)
	}
	rec = &recorder{reply: func(string, []string) ([]byte, error) { return nil, waitDelay }}
	h = newHosting(t, rec)
	if _, err := h.LoginShellEnv(context.Background()); err == nil {
		t.Error("empty output accepted")
	}
}

func TestLoginShellEnvFailureIsNotCached(t *testing.T) {
	fail := true
	rec := &recorder{reply: func(string, []string) ([]byte, error) {
		if fail {
			return nil, errors.New("boom")
		}
		return []byte("A=1\x00"), nil
	}}
	h := newHosting(t, rec)
	if _, err := h.LoginShellEnv(context.Background()); err == nil {
		t.Fatal("failure hidden")
	}
	fail = false
	if got, err := h.LoginShellEnv(context.Background()); err != nil || !contains(got, "A=1") {
		t.Errorf("got %v err=%v", got, err)
	}
}

func TestLoginShellEnvWithInjectedShellScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "shell")
	body := "#!/bin/sh\nsleep 1 &\nprintf 'X=1\\0Y=2\\0'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	h := &Hosting{Shell: script, Env: []string{"Y=mine"}}
	got, err := h.LoginShellEnv(context.Background())
	if err != nil || !contains(got, "X=1") || !contains(got, "Y=mine") || contains(got, "Y=2") {
		t.Errorf("got %v err=%v", got, err)
	}
}

func TestAppEnv(t *testing.T) {
	rec := &recorder{reply: func(string, []string) ([]byte, error) { return []byte("LANG=fr_FR.UTF-8\x00"), nil }}
	h := newHosting(t, rec)
	h.Env = []string{"TERM=xterm", "A=1"}
	if got := h.AppEnv(context.Background()); !reflect.DeepEqual(got, h.Env) || len(rec.calls) != 0 {
		t.Errorf("shell used although TERM is set: %v", got)
	}
	h = newHosting(t, rec)
	h.Env = []string{"A=1"}
	got := h.AppEnv(context.Background())
	if !contains(got, "A=1") || !contains(got, "LANG=fr_FR.UTF-8") {
		t.Errorf("login environment not used: %v", got)
	}
	rec.reply = func(string, []string) ([]byte, error) { return nil, errors.New("boom") }
	h = newHosting(t, rec)
	h.Env = []string{"A=1"}
	if got := h.AppEnv(context.Background()); !reflect.DeepEqual(got, h.Env) {
		t.Errorf("fallback: %v", got)
	}
}

func TestSessionEnvDropsEmptyConfigDir(t *testing.T) {
	got := SessionEnv([]string{"A=1", "CLAUDE_CONFIG_DIR="}, "")
	if !reflect.DeepEqual(got, []string{"A=1"}) {
		t.Errorf("got %v", got)
	}
}

func TestResolveClaudeConfigDir(t *testing.T) {
	login := "CLAUDE_CONFIG_DIR=/login\x00"
	rec := &recorder{reply: func(string, []string) ([]byte, error) { return []byte(login), nil }}
	cases := []struct {
		name    string
		env     []string
		session string
		login   string
		want    string
	}{
		{"login shell value without terminal", []string{"A=1"}, "", "CLAUDE_CONFIG_DIR=/login\x00", "/login"},
		{"empty login shell value", []string{"A=1"}, "", "CLAUDE_CONFIG_DIR=\x00", "/default"},
		{"absent from login shell", []string{"A=1"}, "", "B=2\x00", "/default"},
		{"empty value of the application counts as unset", []string{"A=1", "CLAUDE_CONFIG_DIR="}, "", "CLAUDE_CONFIG_DIR=/login\x00", "/login"},
		{"explicit value kept", []string{"A=1"}, "/explicit", "CLAUDE_CONFIG_DIR=/login\x00", "/explicit"},
		{"terminal environment only", []string{"TERM=xterm"}, "", "CLAUDE_CONFIG_DIR=/login\x00", "/default"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			login = c.login
			h := newHosting(t, rec)
			h.Env = c.env
			h.SessionConfigDir = c.session
			h.ClaudeConfigDir = "/default"
			if c.session != "" {
				h.ClaudeConfigDir = c.session
			}
			h.ResolveClaudeConfigDir(context.Background())
			session := c.session
			if c.want != "/default" {
				session = c.want
			}
			if h.ClaudeConfigDir != c.want || h.SessionConfigDir != session {
				t.Errorf("ClaudeConfigDir %q SessionConfigDir %q", h.ClaudeConfigDir, h.SessionConfigDir)
			}
		})
	}
}

func TestNewSessionEnvMatchesRegistryDir(t *testing.T) {
	rec := &recorder{reply: func(string, []string) ([]byte, error) { return []byte("CLAUDE_CONFIG_DIR=/login\x00"), nil }}
	h := newHosting(t, rec)
	h.Env = []string{"A=1"}
	if got := h.NewSessionEnv(context.Background()); contains(got, "CLAUDE_CONFIG_DIR=/login") {
		t.Errorf("unresolved login shell directory passed on: %v", got)
	}
	h.SessionConfigDir = "/explicit"
	if got := h.NewSessionEnv(context.Background()); !contains(got, "CLAUDE_CONFIG_DIR=/explicit") {
		t.Errorf("explicit directory missing: %v", got)
	}
}

func TestResolveClaudeConfigDirShellFailure(t *testing.T) {
	rec := &recorder{reply: func(string, []string) ([]byte, error) { return nil, errors.New("boom") }}
	h := newHosting(t, rec)
	h.Env = []string{"A=1"}
	h.ClaudeConfigDir = "/default"
	h.ResolveClaudeConfigDir(context.Background())
	if h.ClaudeConfigDir != "/default" || h.SessionConfigDir != "" {
		t.Errorf("ClaudeConfigDir %q SessionConfigDir %q", h.ClaudeConfigDir, h.SessionConfigDir)
	}
}

func TestWriteAndReadEnvFile(t *testing.T) {
	h := newHosting(t, &recorder{})
	path, err := h.WriteEnvFile([]string{"A=1", "B=x y"})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != h.SocketDir {
		t.Errorf("file outside the socket directory: %s", path)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", info.Mode())
	}
	got, err := ReadEnvFile(path)
	if err != nil || !reflect.DeepEqual(got, []string{"A=1", "B=x y"}) {
		t.Errorf("got %q err=%v", got, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file not removed")
	}
	if _, err := ReadEnvFile(path); err == nil {
		t.Error("missing file accepted")
	}
}

func TestExecEnvKeepsPaneTerminalVariables(t *testing.T) {
	file := []string{"PATH=/p", "TERM=from-file", "TMUX=old", "COLORTERM=old", "TERM_PROGRAM=old", "TERM_PROGRAM_VERSION=0", "FOO=1"}
	own := []string{"TERM=tmux-256color", "TERM_PROGRAM=tmux", "TERM_PROGRAM_VERSION=3.8", "TMUX=/s,2,0", "TMUX_PANE=%3", "OTHER=x"}
	got := ExecEnv(file, own)
	sort.Strings(got)
	want := []string{"FOO=1", "PATH=/p", "TERM=tmux-256color", "TERM_PROGRAM=tmux", "TERM_PROGRAM_VERSION=3.8", "TMUX=/s,2,0", "TMUX_PANE=%3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestSweepEnvFilesRemovesOldFilesOnly(t *testing.T) {
	h := newHosting(t, &recorder{})
	if _, err := h.PrivateSocket(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	write := func(name string, age time.Duration) {
		p := filepath.Join(h.SocketDir, name)
		if err := os.WriteFile(p, []byte("A=1\x00"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, now.Add(-age), now.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	write("env-old", 61*time.Second)
	write("env-recent", 59*time.Second)
	write("other-old", time.Hour)
	if err := os.Mkdir(filepath.Join(h.SocketDir, "env-dir"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := h.SweepEnvFiles(now); err != nil {
		t.Fatal(err)
	}
	var left []string
	entries, _ := os.ReadDir(h.SocketDir)
	for _, e := range entries {
		left = append(left, e.Name())
	}
	if want := []string{"env-dir", "env-recent", "other-old"}; !reflect.DeepEqual(left, want) {
		t.Errorf("left %v want %v", left, want)
	}
}

func TestExecEnvArgs(t *testing.T) {
	f, cmd, err := ExecEnvArgs([]string{"/tmp/e", "--", "/bin/claude", "--resume", "-c"})
	if err != nil || f != "/tmp/e" || !reflect.DeepEqual(cmd, []string{"/bin/claude", "--resume", "-c"}) {
		t.Errorf("f=%q cmd=%v err=%v", f, cmd, err)
	}
	for _, bad := range [][]string{nil, {"f"}, {"f", "--"}, {"f", "x", "y"}} {
		if _, _, err := ExecEnvArgs(bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
}

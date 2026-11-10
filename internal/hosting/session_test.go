package hosting

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"claude-tower/internal/tmux"
)

func init() { startupCheckDelay = 0 }

const paneLine = "%7\t4242\t$5\tproj\t@3\t1\t\t200\t50\t1700000000\t/proj\n"

// sessionRecorder answers like a tmux server that starts one session.
func sessionRecorder(duplicates int) *recorder {
	rec := &recorder{}
	rec.reply = func(name string, args []string) ([]byte, error) {
		if !isTmux(name) || len(args) < 3 {
			return nil, nil
		}
		switch {
		case args[2] == "list-sessions":
			return nil, errors.New("no server running")
		case contains(args, "new-session"):
			if duplicates > 0 {
				duplicates--
				return nil, errors.New("tmux: duplicate session: proj")
			}
			return []byte("$5\t%7\n"), nil
		case args[2] == "list-panes":
			return []byte(paneLine), nil
		}
		return nil, nil
	}
	return rec
}

func newSessionCall(rec *recorder) []string {
	for _, c := range rec.tmuxCommands() {
		if contains(c, "new-session") {
			return c
		}
	}
	return nil
}

func TestNewSessionCommand(t *testing.T) {
	rec := sessionRecorder(0)
	h := newHosting(t, rec)
	claude := writeExecutable(t, t.TempDir(), "claude")
	writeConfig(t, h, fmt.Sprintf(`{"claudeCommand":[%q,"--base"]}`, claude))
	env := []string{"SECRET=very-secret-value", "PATH=/bin"}
	pane, err := h.NewSession(context.Background(), "/work/my.proj", []string{"--resume", "-c"}, env)
	if err != nil {
		t.Fatal(err)
	}
	if pane.ID != "%7" || pane.SessionID != "$5" || pane.Server.Socket != filepath.Join(h.SocketDir, "tmux.sock") {
		t.Errorf("pane: %+v", pane)
	}
	call := newSessionCall(rec)
	if len(call) < 20 {
		t.Fatalf("call: %q", call)
	}
	prefix := []string{"-f", filepath.Join(h.ConfigDir, "tmux.conf"), "new-session", "-d", "-P",
		"-F", "#{session_id}\t#{pane_id}", "-x", "200", "-y", "50", "-c", "/work/my.proj", "-s", "my-proj", "--"}
	if !reflect.DeepEqual(call[:len(prefix)], prefix) {
		t.Errorf("prefix: %q", call[:len(prefix)])
	}
	command := call[len(prefix):]
	if len(command) != 8 || command[0] != "/opt/claude-tower" || command[1] != "exec-env" || command[3] != "--" ||
		!reflect.DeepEqual(command[4:], []string{claude, "--base", "--resume", "-c"}) {
		t.Fatalf("command: %q", command)
	}
	if strings.Contains(join(call), "very-secret-value") {
		t.Error("environment visible in the arguments")
	}
	data, err := os.ReadFile(command[2])
	if err != nil || string(data) != "SECRET=very-secret-value\x00PATH=/bin\x00" {
		t.Errorf("env file: %q err=%v", data, err)
	}
}

func TestNewSessionRetriesDuplicateName(t *testing.T) {
	rec := sessionRecorder(2)
	h := newHosting(t, rec)
	h.Env = []string{"PATH=" + filepath.Dir(writeExecutable(t, t.TempDir(), "claude"))}
	if _, err := h.NewSession(context.Background(), "/work/proj", nil, h.Env); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range rec.tmuxCommands() {
		for i, a := range c {
			if a == "-s" && contains(c, "new-session") {
				names = append(names, c[i+1])
			}
		}
	}
	if !reflect.DeepEqual(names, []string{"proj", "proj-2", "proj-3"}) {
		t.Errorf("names: %v", names)
	}
}

func TestNewSessionGivesUpAfterManyDuplicates(t *testing.T) {
	rec := sessionRecorder(1000)
	h := newHosting(t, rec)
	claude := writeExecutable(t, t.TempDir(), "claude")
	writeConfig(t, h, fmt.Sprintf(`{"claudeCommand":[%q]}`, claude))
	if _, err := h.NewSession(context.Background(), "/work/proj", nil, nil); err == nil {
		t.Fatal("no error")
	}
	n := 0
	for _, c := range rec.tmuxCommands() {
		if contains(c, "new-session") {
			n++
		}
	}
	if n != 50 {
		t.Errorf("attempts: %d", n)
	}
	assertNoEnvFiles(t, h)
}

func assertNoEnvFiles(t *testing.T, h *Hosting) {
	t.Helper()
	entries, _ := os.ReadDir(h.SocketDir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "env-") {
			t.Errorf("leftover environment file %s", e.Name())
		}
	}
}

func TestNewSessionClaudeNotFound(t *testing.T) {
	rec := sessionRecorder(0)
	rec.reply = func(name string, args []string) ([]byte, error) {
		if name == "/bin/fake-shell" {
			return []byte("alias claude='claude --foo'\n"), nil
		}
		return nil, errors.New("no server running")
	}
	h := newHosting(t, rec)
	_, err := h.NewSession(context.Background(), "/work/proj", nil, h.Env)
	if err == nil || !strings.Contains(err.Error(), "claude introuvable") || !strings.Contains(err.Error(), filepath.Join(h.ConfigDir, "config.json")) {
		t.Errorf("err=%v", err)
	}
	if newSessionCall(rec) != nil {
		t.Error("session started")
	}
	assertNoEnvFiles(t, h)
}

func TestNewSessionStoppedImmediately(t *testing.T) {
	rec := sessionRecorder(0)
	base := rec.reply
	rec.reply = func(name string, args []string) ([]byte, error) {
		if isTmux(name) && len(args) > 2 && args[2] == "list-panes" {
			return nil, errors.New("no server running")
		}
		return base(name, args)
	}
	h := newHosting(t, rec)
	claude := writeExecutable(t, t.TempDir(), "claude")
	writeConfig(t, h, fmt.Sprintf(`{"claudeCommand":[%q]}`, claude))
	_, err := h.NewSession(context.Background(), "/work/proj", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "arrêtée immédiatement") {
		t.Errorf("err=%v", err)
	}
	assertNoEnvFiles(t, h)
}

func TestResolveClaude(t *testing.T) {
	bin := t.TempDir()
	onPath := writeExecutable(t, bin, "claude")
	notExec := filepath.Join(bin, "plain")
	os.WriteFile(notExec, nil, 0o644)

	shellOut := ""
	rec := &recorder{reply: func(string, []string) ([]byte, error) { return []byte(shellOut), nil }}
	h := newHosting(t, rec)
	ctx := context.Background()

	writeConfig(t, h, fmt.Sprintf(`{"claudeCommand":[%q,"-x"]}`, onPath))
	if got, err := h.resolveClaude(ctx, nil); err != nil || !reflect.DeepEqual(got, []string{onPath, "-x"}) {
		t.Errorf("absolute: %v err=%v", got, err)
	}
	writeConfig(t, h, fmt.Sprintf(`{"claudeCommand":[%q]}`, notExec))
	if _, err := h.resolveClaude(ctx, nil); err == nil {
		t.Error("non executable accepted")
	}

	writeConfig(t, h, `{}`)
	if got, err := h.resolveClaude(ctx, []string{"PATH=/none:" + bin}); err != nil || got[0] != onPath {
		t.Errorf("PATH of env: %v err=%v", got, err)
	}
	if len(rec.calls) != 0 {
		t.Error("shell used although PATH had claude")
	}

	shellOut = "alias claude='claude --foo'\n"
	if _, err := h.resolveClaude(ctx, []string{"PATH=/none"}); err == nil {
		t.Error("alias accepted")
	}
	shellOut = "/not/absolute/or/missing\n"
	if _, err := h.resolveClaude(ctx, []string{"PATH=/none"}); err == nil {
		t.Error("missing path accepted")
	}
	shellOut = "banner\n" + onPath + "\n"
	got, err := h.resolveClaude(ctx, []string{"PATH=/none"})
	if err != nil || got[0] != onPath {
		t.Errorf("shell path: %v err=%v", got, err)
	}
	last := rec.calls[len(rec.calls)-1]
	if !reflect.DeepEqual(last.args, []string{"-ilc", "command -v claude"}) {
		t.Errorf("shell call: %v", last.args)
	}
}

func TestResolveClaudeShellLingeringChild(t *testing.T) {
	bin := t.TempDir()
	onPath := writeExecutable(t, bin, "claude")
	rec := &recorder{reply: func(string, []string) ([]byte, error) {
		return []byte(onPath + "\n"), fmt.Errorf("sh: %w", exec.ErrWaitDelay)
	}}
	h := newHosting(t, rec)
	if got, err := h.resolveClaude(context.Background(), []string{"PATH=/none"}); err != nil || got[0] != onPath {
		t.Errorf("got %v err=%v", got, err)
	}
}

func TestResolveClaudeRejectsUnsafeName(t *testing.T) {
	rec := &recorder{}
	h := newHosting(t, rec)
	writeConfig(t, h, `{"claudeCommand":["claude; rm -rf ~"]}`)
	if _, err := h.resolveClaude(context.Background(), []string{"PATH=/none"}); err == nil {
		t.Error("unsafe name accepted")
	}
	if len(rec.calls) != 0 {
		t.Errorf("shell called with %v", rec.calls)
	}
}

func TestSessionName(t *testing.T) {
	cases := map[string]string{"/a/proj": "proj", "/a/my.proj:x": "my-proj-x", "/": "claude", "": "claude", ".": "claude"}
	for dir, want := range cases {
		if got := sessionName(dir); got != want {
			t.Errorf("%q: got %q want %q", dir, got, want)
		}
	}
}

func privatePane(h *Hosting, rec *recorder) tmux.Pane {
	sock, _ := h.PrivateSocket()
	return tmux.Pane{
		Server: tmux.Server{Socket: sock, Bin: "tmux", Run: rec.run}, ID: "%7", SessionID: "$5",
	}
}

func TestKillRefusedOutsidePrivateServer(t *testing.T) {
	rec := &recorder{}
	h := newHosting(t, rec)
	pane := privatePane(h, rec)
	pane.Server.Socket = "/tmp/tmux-1000/default"
	if err := h.Kill(context.Background(), pane); err == nil {
		t.Error("kill accepted")
	}
	if len(rec.calls) != 0 {
		t.Errorf("tmux called: %v", rec.calls)
	}
}

func TestKillStopsViewSessionsFirst(t *testing.T) {
	rec := &recorder{reply: func(name string, args []string) ([]byte, error) {
		if args[2] == "list-sessions" {
			return []byte("$5\tproj\tgrp\n$8\tclaude-tower-view-1-1\tgrp\n$9\tclaude-tower-view-1-2\tother\n$10\tclaude-tower-view-1-3\tgrp\n$11\tnot-a-view\tgrp\nbroken\n"), nil
		}
		return nil, nil
	}}
	h := newHosting(t, rec)
	if err := h.Kill(context.Background(), privatePane(h, rec)); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"list-sessions", "-F", "#{session_id}\t#{session_name}\t#{session_group}"},
		{"kill-session", "-t", "$8"},
		{"kill-session", "-t", "$10"},
		{"kill-session", "-t", "$5"},
	}
	if got := rec.tmuxCommands(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestKillUngroupedSessionKillsOnlyItself(t *testing.T) {
	rec := &recorder{reply: func(name string, args []string) ([]byte, error) {
		if args[2] == "list-sessions" {
			return []byte("$5\tproj\t\n$9\tclaude-tower-view-1-1\t\n"), nil
		}
		return nil, nil
	}}
	h := newHosting(t, rec)
	if err := h.Kill(context.Background(), privatePane(h, rec)); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"list-sessions", "-F", "#{session_id}\t#{session_name}\t#{session_group}"},
		{"kill-session", "-t", "$5"},
	}
	if got := rec.tmuxCommands(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestKillReportsListFailureWithoutKilling(t *testing.T) {
	rec := &recorder{reply: func(name string, args []string) ([]byte, error) {
		return nil, errors.New("no server running")
	}}
	h := newHosting(t, rec)
	if err := h.Kill(context.Background(), privatePane(h, rec)); err == nil || !strings.Contains(err.Error(), "list-sessions") {
		t.Errorf("err=%v", err)
	}
	if got := rec.tmuxCommands(); len(got) != 1 {
		t.Errorf("got %q", got)
	}
}

func TestNewSessionWritesTmuxConf(t *testing.T) {
	rec := sessionRecorder(0)
	h := newHosting(t, rec)
	claude := writeExecutable(t, t.TempDir(), "claude")
	writeConfig(t, h, fmt.Sprintf(`{"claudeCommand":[%q]}`, claude))
	if _, err := h.NewSession(context.Background(), "/work/proj", nil, nil); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(h.ConfigDir, "tmux.conf"))
	if err != nil || !strings.Contains(string(data), "set -g status off\n") {
		t.Errorf("tmux.conf: %q err=%v", data, err)
	}
}

func TestNewSessionNeedsSelfPath(t *testing.T) {
	rec := sessionRecorder(0)
	h := newHosting(t, rec)
	h.Self = ""
	claude := writeExecutable(t, t.TempDir(), "claude")
	writeConfig(t, h, fmt.Sprintf(`{"claudeCommand":[%q]}`, claude))
	_, err := h.NewSession(context.Background(), "/work/proj", nil, nil)
	if err == nil || !strings.Contains(err.Error(), "chemin de claude-tower inconnu") {
		t.Errorf("err=%v", err)
	}
	if len(rec.calls) != 0 {
		t.Errorf("tmux called: %v", rec.calls)
	}
	assertNoEnvFiles(t, h)
}

func TestFindInPathIgnoresRelativeEntries(t *testing.T) {
	work := t.TempDir()
	writeExecutable(t, filepath.Join(work, "rel"), "claude")
	writeExecutable(t, work, "claude")
	t.Chdir(work)
	abs := t.TempDir()
	if p, ok := findInPath("rel:.::bin", "claude"); ok {
		t.Errorf("relative entry used: %s", p)
	}
	want := writeExecutable(t, abs, "claude")
	if p, ok := findInPath("rel:.::"+abs, "claude"); !ok || p != want {
		t.Errorf("got %q ok=%v", p, ok)
	}
}

func TestLookupClaudeRefusesRelativeShellOutput(t *testing.T) {
	work := t.TempDir()
	writeExecutable(t, work, "claude")
	t.Chdir(work)
	rec := &recorder{reply: func(string, []string) ([]byte, error) { return []byte("claude\n"), nil }}
	h := newHosting(t, rec)
	if p, ok := h.lookupClaude(context.Background(), "claude", []string{"PATH=/nonexistent"}); ok {
		t.Errorf("relative path accepted: %q", p)
	}
	if len(rec.calls) != 1 {
		t.Errorf("shell not consulted: %v", rec.calls)
	}
}

package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"claude-tower/internal/background"
	"claude-tower/internal/locate"
	"claude-tower/internal/registry"
	"claude-tower/internal/tmux"
	"claude-tower/internal/transcript"
)

var testNow = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

func init() { now = func() time.Time { return testNow } }

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    opts
		wantErr bool
	}{
		{"flag after command", []string{"bar", "--dir", "X"}, opts{cmd: "bar", dir: "X", maxWidth: 100}, false},
		{"flag before command", []string{"--dir", "X", "bar"}, opts{cmd: "bar", dir: "X", maxWidth: 100}, false},
		{"flags without command", []string{"--dir", "X"}, opts{dir: "X", maxWidth: 100}, false},
		{"flags on both sides", []string{"--dir", "X", "bar", "--max-width", "30"}, opts{cmd: "bar", dir: "X", maxWidth: 30}, false},
		{"unknown command parses", []string{"nope"}, opts{cmd: "nope", maxWidth: 100}, false},
		{"two positionals", []string{"bar", "list"}, opts{}, true},
		{"unknown flag", []string{"--nope"}, opts{}, true},
		{"unknown flag after command", []string{"bar", "--nope"}, opts{}, true},
	}
	for _, c := range cases {
		got, err := parseArgs(c.args)
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("%s: got %+v err=%v", c.name, got, err)
		}
	}
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// fixture builds a config dir whose sessions all use live PIDs of the test
// process tree, so that the default liveness check accepts them.
func fixture(t *testing.T, sessions map[int]string) string {
	t.Helper()
	dir := t.TempDir()
	sd := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	for pid, body := range sessions {
		if err := os.WriteFile(filepath.Join(sd, fmt.Sprintf("%d.json", pid)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func writeTranscript(t *testing.T, dir, id string, lines ...string) {
	t.Helper()
	p := filepath.Join(dir, "projects", "slug", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

const tid = "11111111-2222-3333-4444-555555555555"

func busySession(pid int) string {
	return fmt.Sprintf(`{"pid":%d,"sessionId":%q,"name":"demo","cwd":"/srv/demo","status":"busy","statusUpdatedAt":%d}`,
		pid, tid, testNow.Add(-5*time.Minute).UnixMilli())
}

// fakeDeps never touches the tmux servers of the machine running the tests.
func fakeDeps(locations map[int]string) deps {
	return deps{locations: func(context.Context, []registry.Session) map[int]string { return locations }}
}

func runArgsDeps(args []string, getenv func(string) string, d deps) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb, getenv, d)
	return code, out.String(), errb.String()
}

func runArgs(args []string, getenv func(string) string) (int, string, string) {
	return runArgsDeps(args, getenv, fakeDeps(nil))
}

func TestBar(t *testing.T) {
	pid := os.Getpid()
	dir := fixture(t, map[int]string{pid: busySession(pid)})
	code, out, _ := runArgs([]string{"bar", "--dir", dir}, env(nil))
	if want := "#[fg=yellow]● demo 5m#[default]\n"; code != 0 || out != want {
		t.Errorf("code=%d out=%q want %q", code, out, want)
	}
}

func TestBarTurnEnded(t *testing.T) {
	pid := os.Getpid()
	dir := fixture(t, map[int]string{pid: busySession(pid)})
	writeTranscript(t, dir, tid,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}`,
		`{"type":"system","subtype":"turn_duration"}`,
		`{"type":"attachment"}`)
	_, out, _ := runArgs([]string{"bar", "--dir", dir}, env(nil))
	if want := "#[fg=cyan]◐ demo#[default]\n"; out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestBarTurnNotEnded(t *testing.T) {
	pid := os.Getpid()
	dir := fixture(t, map[int]string{pid: busySession(pid)})
	writeTranscript(t, dir, tid,
		`{"type":"system","subtype":"turn_duration"}`,
		`{"type":"user","message":{"content":"go on"}}`)
	_, out, _ := runArgs([]string{"bar", "--dir", dir}, env(nil))
	if want := "#[fg=yellow]● demo 5m#[default]\n"; out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestBarMissingTranscriptKeepsBusy(t *testing.T) {
	pid := os.Getpid()
	dir := fixture(t, map[int]string{pid: busySession(pid)})
	p := filepath.Join(dir, "projects", "slug", tid+".jsonl")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	_, out, _ := runArgs([]string{"bar", "--dir", dir}, env(nil))
	if want := "#[fg=yellow]● demo 5m#[default]\n"; out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func TestBarUnreadableTranscriptKeepsBusy(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions are not enforced for root")
	}
	pid := os.Getpid()
	dir := fixture(t, map[int]string{pid: busySession(pid)})
	writeTranscript(t, dir, tid, `{"type":"system","subtype":"turn_duration"}`)
	p := filepath.Join(dir, "projects", "slug", tid+".jsonl")
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	_, out, _ := runArgs([]string{"bar", "--dir", dir}, env(nil))
	if want := "#[fg=yellow]● demo 5m#[default]\n"; out != want {
		t.Errorf("got %q want %q", out, want)
	}
}

func idleSession(pid int) string {
	return fmt.Sprintf(`{"pid":%d,"sessionId":%q,"name":"quiet","status":"idle"}`, pid, tid)
}

func TestBarShowsBackgroundWorkOfIdleSession(t *testing.T) {
	pid := os.Getpid()
	dir := fixture(t, map[int]string{pid: idleSession(pid)})
	cases := []struct {
		name   string
		counts background.Counts
		want   string
	}{
		{"agents running", background.Counts{Agents: 1}, "#[fg=cyan]◐ quiet#[default]\n"},
		{"shells running", background.Counts{Shells: 2}, "#[fg=cyan]◐ quiet#[default]\n"},
		{"unknown counts", background.Counts{Agents: background.Unknown, Shells: background.Unknown}, "#[fg=colour244]○ quiet#[default]\n"},
		{"nothing running", background.Counts{}, "#[fg=colour244]○ quiet#[default]\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := fakeDeps(nil)
			d.background = func(context.Context, *transcript.Finder, []registry.Session) map[int]background.Counts {
				return map[int]background.Counts{pid: c.counts}
			}
			_, out, _ := runArgsDeps([]string{"bar", "--dir", dir}, env(nil), d)
			if out != c.want {
				t.Errorf("got %q want %q", out, c.want)
			}
		})
	}
}

func TestBarCountsBackgroundWorkOfIdleSessionsOnly(t *testing.T) {
	pid, ppid := os.Getpid(), os.Getppid()
	dir := fixture(t, map[int]string{
		pid:  busySession(pid),
		ppid: fmt.Sprintf(`{"pid":%d,"sessionId":"22222222-2222-3333-4444-555555555555","name":"quiet","status":"idle"}`, ppid),
	})
	var counted []int
	d := fakeDeps(nil)
	d.background = func(_ context.Context, _ *transcript.Finder, sessions []registry.Session) map[int]background.Counts {
		for _, s := range sessions {
			counted = append(counted, s.PID)
		}
		return nil
	}
	runArgsDeps([]string{"bar", "--dir", dir}, env(nil), d)
	if len(counted) != 1 || counted[0] != ppid {
		t.Errorf("counted=%v want [%d]", counted, ppid)
	}
}

func TestBarWithoutIdleSessionsCountsNothing(t *testing.T) {
	pid := os.Getpid()
	dir := fixture(t, map[int]string{pid: busySession(pid)})
	d := fakeDeps(nil)
	d.background = func(context.Context, *transcript.Finder, []registry.Session) map[int]background.Counts {
		t.Error("background counted")
		return nil
	}
	runArgsDeps([]string{"bar", "--dir", dir}, env(nil), d)
}

func TestBarUnreadableSessionsDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions are not enforced for root")
	}
	dir := fixture(t, nil)
	sd := filepath.Join(dir, "sessions")
	if err := os.Chmod(sd, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(sd, 0o755) })
	code, out, errOut := runArgs([]string{"bar", "--dir", dir}, env(nil))
	if code != 0 || out != "" || errOut == "" {
		t.Errorf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestList(t *testing.T) {
	pid, ppid := os.Getpid(), os.Getppid()
	dir := fixture(t, map[int]string{
		pid:  busySession(pid),
		ppid: fmt.Sprintf(`{"pid":%d,"sessionId":"22222222-2222-3333-4444-555555555555","name":"zzz","status":"idle","tmux":"0:@0.%%4"}`, ppid),
	})
	writeTranscript(t, dir, tid, `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"make"}}]}}`)
	d := fakeDeps(map[int]string{pid: "hors tmux", ppid: "tmux"})
	code, out, _ := runArgsDeps([]string{"list", "--dir", dir}, env(map[string]string{"HOME": "/srv"}), d)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if code != 0 || len(lines) != 2 {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if want := "●  demo  travaille  5m  -  hors tmux  ~/demo  -  Bash: make"; lines[0] != want {
		t.Errorf("got  %q\nwant %q", lines[0], want)
	}
	if !strings.HasPrefix(lines[1], "○  zzz  fini  -  %4  tmux  ") {
		t.Errorf("second line: %q", lines[1])
	}
}

func TestListShowsBackgroundWork(t *testing.T) {
	pid, ppid := os.Getpid(), os.Getppid()
	dir := fixture(t, map[int]string{
		pid:  fmt.Sprintf(`{"pid":%d,"sessionId":%q,"name":"quiet","status":"idle"}`, pid, tid),
		ppid: fmt.Sprintf(`{"pid":%d,"sessionId":"22222222-2222-3333-4444-555555555555","name":"other","status":"idle"}`, ppid),
	})
	d := fakeDeps(map[int]string{pid: "tmux", ppid: "tmux"})
	d.background = func(_ context.Context, _ *transcript.Finder, sessions []registry.Session) map[int]background.Counts {
		if len(sessions) != 2 {
			t.Errorf("sessions: %+v", sessions)
		}
		return map[int]background.Counts{pid: {Agents: 2, Shells: 1}}
	}
	_, out, _ := runArgsDeps([]string{"list", "--dir", dir}, env(nil), d)
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "◐  quiet  fini · fond  ") || !strings.Contains(lines[0], "  2a·1s  ") {
		t.Errorf("background session: %q", lines)
	}
	if !strings.HasPrefix(lines[1], "○  other  fini  ") {
		t.Errorf("idle session: %q", lines)
	}
}

func TestBackgroundColumn(t *testing.T) {
	cases := []struct {
		c    background.Counts
		want string
	}{
		{background.Counts{}, ""},
		{background.Counts{Agents: 2}, "2a"},
		{background.Counts{Shells: 1}, "1s"},
		{background.Counts{Agents: 2, Shells: 1}, "2a·1s"},
		{background.Counts{Agents: 1, Shells: -1}, "1a·?s"},
		{background.Counts{Shells: -1}, "?s"},
	}
	for _, c := range cases {
		if got := backgroundColumn(c.c); got != c.want {
			t.Errorf("%+v: got %q want %q", c.c, got, c.want)
		}
	}
}

func TestSystemBackgroundReadsTranscript(t *testing.T) {
	dir := t.TempDir()
	writeTranscript(t, dir, tid,
		`{"type":"user","timestamp":"2026-11-09T11:00:00Z","toolUseResult":{"status":"async_launched","agentId":"a1"}}`)
	sessions := []registry.Session{{PID: os.Getpid(), SessionID: tid}, {PID: 1 << 22, SessionID: "22222222-2222-3333-4444-555555555555"}}
	got := systemBackground(context.Background(), transcript.NewFinder(dir), sessions)
	if got[os.Getpid()].Agents != 1 || got[1<<22].Agents != 0 {
		t.Errorf("counts: %+v", got)
	}
}

func TestNoSubcommandIsUsageError(t *testing.T) {
	code, out, errOut := runArgs(nil, env(nil))
	if code != 2 || out != "" || !strings.Contains(errOut, "missing subcommand") {
		t.Errorf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestUnknownSubcommand(t *testing.T) {
	code, out, errOut := runArgs([]string{"nope"}, env(nil))
	if code != 2 || out != "" || !strings.Contains(errOut, "usage") {
		t.Errorf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestUsageError(t *testing.T) {
	if code, _, _ := runArgs([]string{"bar", "list"}, env(nil)); code != 2 {
		t.Errorf("code=%d", code)
	}
}

func TestConfigDirResolution(t *testing.T) {
	pid := os.Getpid()
	fromEnv := fixture(t, map[int]string{pid: strings.Replace(busySession(pid), "demo", "fromenv", 1)})
	fromFlag := fixture(t, map[int]string{pid: strings.Replace(busySession(pid), "demo", "fromflag", 1)})
	_, out, _ := runArgs([]string{"bar"}, env(map[string]string{"CLAUDE_CONFIG_DIR": fromEnv}))
	if !strings.Contains(out, "fromenv") {
		t.Errorf("env dir not used: %q", out)
	}
	_, out, _ = runArgs([]string{"bar", "--dir", fromFlag}, env(map[string]string{"CLAUDE_CONFIG_DIR": fromEnv}))
	if !strings.Contains(out, "fromflag") {
		t.Errorf("flag dir not preferred: %q", out)
	}
}

func TestDefaultConfigDir(t *testing.T) {
	pid := os.Getpid()
	home := t.TempDir()
	sd := filepath.Join(home, ".claude", "sessions")
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sd, fmt.Sprintf("%d.json", pid)), []byte(busySession(pid)), 0o644); err != nil {
		t.Fatal(err)
	}
	_, out, _ := runArgs([]string{"bar"}, env(map[string]string{"HOME": home}))
	if !strings.Contains(out, "demo") {
		t.Errorf("got %q", out)
	}
}

func TestListUnknownLocationWithoutTmux(t *testing.T) {
	pid := os.Getpid()
	dir := fixture(t, map[int]string{pid: busySession(pid)})
	_, out, _ := runArgsDeps([]string{"list", "--dir", dir}, env(nil), fakeDeps(unknownLocations([]registry.Session{{PID: pid}})))
	if !strings.Contains(out, "  ?  ") {
		t.Errorf("location column missing: %q", out)
	}
}

func fixedRunner(out string) tmux.Runner {
	return func(context.Context, io.Reader, string, ...string) ([]byte, error) { return []byte(out), nil }
}

func TestTmuxLocations(t *testing.T) {
	sessions := []registry.Session{{PID: 300}, {PID: 400}, {PID: 500}}
	tree := locate.Tree{300: 200, 200: 100, 400: 450, 450: 1, 500: 600}
	readTree := func(context.Context, tmux.Runner) (locate.Tree, error) { return tree, nil }
	pub := tmux.Server{Socket: "/s/default", Run: fixedRunner("%1\t100\t$1\tmain\t@1\t1\t\t1700000000\t/w\n")}
	priv := tmux.Server{Socket: "/s/private", Run: fixedRunner("%2\t600\t$2\tpriv\t@2\t1\t\t1700000000\t/w\n")}
	dead := tmux.Server{Socket: "/s/dead", Run: func(context.Context, io.Reader, string, ...string) ([]byte, error) {
		return nil, errors.New("no server running")
	}}
	got := tmuxLocations(context.Background(), sessions, []tmux.Server{dead, pub, priv}, "/s/private", readTree, nil)
	want := map[int]string{300: "tmux", 400: "hors tmux", 500: "privé"}
	for pid, w := range want {
		if got[pid] != w {
			t.Errorf("pid %d: got %q want %q", pid, got[pid], w)
		}
	}
}

func TestTmuxLocationsUnknownWhenTreeUnreadable(t *testing.T) {
	readTree := func(context.Context, tmux.Runner) (locate.Tree, error) { return nil, errors.New("no ps") }
	got := tmuxLocations(context.Background(), []registry.Session{{PID: 1}}, nil, "", readTree, nil)
	if got[1] != "?" {
		t.Errorf("got %v", got)
	}
}

func TestRunSubcommandPassesArgumentsUntouched(t *testing.T) {
	var got []string
	called := false
	d := fakeDeps(nil)
	d.run = func(args []string) error { got, called = args, true; return nil }
	args := []string{"run", "--resume", "-c", "--model", "x", "--dir", "y", "bar"}
	code, out, errOut := runArgsDeps(args, env(nil), d)
	if code != 0 || out != "" || errOut != "" || !called {
		t.Fatalf("code=%d out=%q err=%q called=%v", code, out, errOut, called)
	}
	if want := []string{"--resume", "-c", "--model", "x", "--dir", "y", "bar"}; !equal(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestRunSubcommandFailure(t *testing.T) {
	d := fakeDeps(nil)
	d.run = func([]string) error { return errors.New("claude introuvable") }
	code, _, errOut := runArgsDeps([]string{"run"}, env(nil), d)
	if code != 1 || !strings.Contains(errOut, "claude introuvable") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type execCall struct {
	path      string
	argv, env []string
}

func TestExecEnvAppliesFileAndRemovesIt(t *testing.T) {
	file := filepath.Join(t.TempDir(), "env")
	if err := os.WriteFile(file, []byte("PATH=/p\x00FOO=1\x00TERM=stale\x00"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got execCall
	d := fakeDeps(nil)
	d.environ = func() []string { return []string{"TERM=tmux-256color", "TMUX=/s,1,0", "TMUX_PANE=%2", "HOME=/ignored"} }
	d.exec = func(path string, argv, env []string) error { got = execCall{path, argv, env}; return nil }
	code, _, errOut := runArgsDeps([]string{"exec-env", file, "--", "/bin/claude", "--resume", "-c"}, env(nil), d)
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if got.path != "/bin/claude" || !equal(got.argv, []string{"/bin/claude", "--resume", "-c"}) {
		t.Errorf("command: %+v", got)
	}
	for _, want := range []string{"PATH=/p", "FOO=1", "TERM=tmux-256color", "TMUX=/s,1,0", "TMUX_PANE=%2"} {
		if !equal(filter(got.env, want), []string{want}) {
			t.Errorf("%q missing from %v", want, got.env)
		}
	}
	for _, bad := range []string{"TERM=stale", "HOME=/ignored"} {
		if len(filter(got.env, bad)) != 0 {
			t.Errorf("%q leaked into %v", bad, got.env)
		}
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("environment file not removed")
	}
}

func filter(env []string, want string) []string {
	var out []string
	for _, e := range env {
		if e == want {
			out = append(out, e)
		}
	}
	return out
}

func TestExecEnvErrors(t *testing.T) {
	d := fakeDeps(nil)
	d.exec = func(string, []string, []string) error { t.Error("exec called"); return nil }
	d.environ = func() []string { return nil }
	for _, args := range [][]string{
		{"exec-env"},
		{"exec-env", "f", "x", "y"},
		{"exec-env", filepath.Join(t.TempDir(), "missing"), "--", "/bin/claude"},
	} {
		if code, _, errOut := runArgsDeps(args, env(nil), d); code != 1 || errOut == "" {
			t.Errorf("%v: code=%d err=%q", args, code, errOut)
		}
	}
}

func TestExecEnvResolvesBareCommandThroughPATH(t *testing.T) {
	bin := t.TempDir()
	tool := filepath.Join(bin, "mytool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	file := filepath.Join(t.TempDir(), "env")
	if err := os.WriteFile(file, []byte("A=1\x00"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got execCall
	d := fakeDeps(nil)
	d.environ = func() []string { return nil }
	d.exec = func(path string, argv, env []string) error { got = execCall{path, argv, env}; return nil }
	if code, _, errOut := runArgsDeps([]string{"exec-env", file, "--", "mytool", "-x"}, env(nil), d); code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if got.path != tool || !equal(got.argv, []string{"mytool", "-x"}) {
		t.Errorf("got %+v", got)
	}

	if err := os.WriteFile(file, []byte("A=1\x00"), 0o600); err != nil {
		t.Fatal(err)
	}
	got = execCall{}
	if code, _, errOut := runArgsDeps([]string{"exec-env", file, "--", "missing-tool"}, env(nil), d); code != 1 || errOut == "" || got.path != "" {
		t.Errorf("code=%d err=%q exec=%+v", code, errOut, got)
	}
}

func TestSystemLocationsDoNotCreatePrivateDirectory(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	home := t.TempDir()
	runtimeDir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	t.Setenv("TMUX_TMPDIR", t.TempDir())
	sessions := []registry.Session{{PID: 1}}
	got := systemLocations(context.Background(), sessions, os.Getenv)
	if len(got) != 1 {
		t.Fatalf("locations: %v", got)
	}
	for _, dir := range []string{
		filepath.Join(runtimeDir, "claude-tower"),
		filepath.Join(home, ".local", "state", "claude-tower"),
		filepath.Join(home, "Library", "Application Support", "claude-tower"),
	} {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Errorf("%s created (err=%v)", dir, err)
		}
	}
}

func TestIsCLI(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{nil, false},
		{[]string{}, false},
		{[]string{"run"}, true},
		{[]string{"exec-env", "x"}, true},
		{[]string{"bar"}, true},
		{[]string{"list"}, true},
		{[]string{"--dir", "/x", "list"}, true},
		{[]string{"--help"}, true},
		{[]string{"-h"}, true},
		{[]string{"unknown"}, false},
		{[]string{"", "list"}, false},
	}
	for _, c := range cases {
		if got := IsCLI(c.args); got != c.want {
			t.Errorf("IsCLI(%q) = %v, want %v", c.args, got, c.want)
		}
	}
}

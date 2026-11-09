package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
		{"flags without command", []string{"--dir", "X", "--client", "Y"}, opts{dir: "X", client: "Y", maxWidth: 100}, false},
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

func runArgs(args []string, getenv func(string) string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run(args, &out, &errb, getenv)
	return code, out.String(), errb.String()
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
	code, out, _ := runArgs([]string{"list", "--dir", dir}, env(map[string]string{"HOME": "/srv"}))
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if code != 0 || len(lines) != 2 {
		t.Fatalf("code=%d out=%q", code, out)
	}
	if want := "●  demo  travaille  5m  -  ~/demo  Bash: make"; lines[0] != want {
		t.Errorf("got  %q\nwant %q", lines[0], want)
	}
	if !strings.HasPrefix(lines[1], "○  zzz  fini  -  %4  ") {
		t.Errorf("second line: %q", lines[1])
	}
}

func TestNoSubcommandListsSessions(t *testing.T) {
	pid := os.Getpid()
	dir := fixture(t, map[int]string{pid: busySession(pid)})
	code, out, _ := runArgs([]string{"--dir", dir}, env(nil))
	if code != 0 || !strings.Contains(out, "demo") {
		t.Errorf("code=%d out=%q", code, out)
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

package hosting

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type execCall struct {
	path      string
	argv, env []string
}

func TestRunInsideTmuxExecsClaudeDirectly(t *testing.T) {
	rec := &recorder{}
	h := newHosting(t, rec)
	claude := writeExecutable(t, t.TempDir(), "claude")
	writeConfig(t, h, fmt.Sprintf(`{"claudeCommand":[%q,"--base"]}`, claude))
	h.Env = []string{"TMUX=/s,1,0", "TERM=screen"}
	var got execCall
	h.Exec = func(path string, argv, env []string) error {
		got = execCall{path, argv, env}
		return nil
	}
	if err := h.Run([]string{"--resume", "-c", "--model", "x"}); err != nil {
		t.Fatal(err)
	}
	want := execCall{claude, []string{claude, "--base", "--resume", "-c", "--model", "x"}, h.Env}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if len(rec.calls) != 0 {
		t.Errorf("tmux used inside tmux: %v", rec.calls)
	}
}

func TestRunOutsideTmuxStartsPrivateSessionAndAttaches(t *testing.T) {
	rec := sessionRecorder(0)
	h := newHosting(t, rec)
	claude := writeExecutable(t, t.TempDir(), "claude")
	writeConfig(t, h, fmt.Sprintf(`{"claudeCommand":[%q]}`, claude))
	h.Env = []string{"TERM=xterm", "TMUX_PANE=%1", "PATH=/bin", "CLAUDE_CONFIG_DIR=/caller", "LANG=C"}
	h.Bin = "/usr/bin/tmux"
	h.Cwd = func() (string, error) { return "/work/proj", nil }
	var got execCall
	h.Exec = func(path string, argv, env []string) error {
		got = execCall{path, argv, env}
		return nil
	}
	if err := h.Run([]string{"--resume", "-c", "--model", "x"}); err != nil {
		t.Fatal(err)
	}
	call := newSessionCall(rec)
	if call == nil || !reflect.DeepEqual(call[len(call)-4:], []string{"--resume", "-c", "--model", "x"}) {
		t.Fatalf("session command: %q", call)
	}
	if i := indexOf(call, "-c"); i < 0 || call[i+1] != "/work/proj" {
		t.Errorf("working directory missing from %q", call)
	}
	envFile := call[len(call)-7]
	if !strings.HasPrefix(filepath.Base(envFile), "env-") {
		t.Fatalf("env file argument: %q", envFile)
	}
	data, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatal(err)
	}
	fileEnv := parseEnvBlock(data)
	if !contains(fileEnv, "CLAUDE_CONFIG_DIR=/caller") || !contains(fileEnv, "LANG=C.UTF-8") ||
		contains(fileEnv, "TERM=xterm") || contains(fileEnv, "TMUX_PANE=%1") {
		t.Errorf("session environment: %v", fileEnv)
	}
	sock := filepath.Join(h.SocketDir, "tmux.sock")
	if got.path != "/usr/bin/tmux" || !reflect.DeepEqual(got.argv, []string{"/usr/bin/tmux", "-S", sock, "attach", "-t", "$5"}) {
		t.Errorf("attach: %+v", got)
	}
	if !contains(got.env, "TERM=xterm") {
		t.Errorf("attach environment lost TERM: %v", got.env)
	}
}

func TestRunReportsMissingClaude(t *testing.T) {
	rec := &recorder{}
	h := newHosting(t, rec)
	h.Env = []string{"TMUX=/s,1,0", "PATH=/nonexistent"}
	h.Exec = func(string, []string, []string) error { t.Error("exec called"); return nil }
	if err := h.Run(nil); err == nil || !strings.Contains(err.Error(), "claude introuvable") {
		t.Errorf("err=%v", err)
	}
}

func indexOf(list []string, want string) int {
	for i, s := range list {
		if s == want {
			return i
		}
	}
	return -1
}

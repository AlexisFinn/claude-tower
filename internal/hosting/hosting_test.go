package hosting

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPrivateSocketDir(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cases := []struct {
		name, goos string
		env        map[string]string
		want       string
	}{
		{"linux with runtime dir", "linux", map[string]string{"XDG_RUNTIME_DIR": "/run/user/1000"}, "/run/user/1000/claude-tower"},
		{"linux without runtime dir", "linux", nil, "/home/u/.local/state/claude-tower"},
		{"macos", "darwin", map[string]string{"XDG_RUNTIME_DIR": "/ignored"}, "/home/u/Library/Application Support/claude-tower"},
	}
	for _, c := range cases {
		if got := privateSocketDir(c.goos, "/home/u", env(c.env)); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestDefaultConfigDir(t *testing.T) {
	if got := defaultConfigDir("linux", "/home/u"); got != "/home/u/.config/claude-tower" {
		t.Errorf("linux: %q", got)
	}
	if got := defaultConfigDir("darwin", "/home/u"); got != "/home/u/Library/Application Support/claude-tower" {
		t.Errorf("darwin: %q", got)
	}
}

func TestPrivateSocketCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "claude-tower")
	h := &Hosting{SocketDir: dir}
	sock, err := h.PrivateSocket()
	if err != nil || sock != filepath.Join(dir, "tmux.sock") {
		t.Fatalf("sock=%q err=%v", sock, err)
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Errorf("dir mode: %v err=%v", info, err)
	}
	if _, err := h.PrivateSocket(); err != nil {
		t.Errorf("existing directory refused: %v", err)
	}
}

func TestPrivateSocketRefusesWrongMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "d")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Hosting{SocketDir: dir}).PrivateSocket(); err == nil || !strings.Contains(err.Error(), "0700") {
		t.Errorf("err=%v", err)
	}
}

func TestPrivateSocketRefusesSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := (&Hosting{SocketDir: link}).PrivateSocket(); err == nil {
		t.Error("symbolic link accepted")
	}
}

func TestEnsureSocketDirRefusesOtherOwner(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "d")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := ensureSocketDir(dir, os.Getuid()); err != nil {
		t.Fatalf("own directory refused: %v", err)
	}
	if err := ensureSocketDir(dir, os.Getuid()+1); err == nil || !strings.Contains(err.Error(), "appartient") {
		t.Errorf("other owner accepted: %v", err)
	}
}

func TestNewSystem(t *testing.T) {
	home := t.TempDir()
	h, err := NewSystem([]string{"HOME=" + home, "XDG_RUNTIME_DIR=/run/x"})
	if err != nil {
		t.Fatal(err)
	}
	if h.ClaudeConfigDir != filepath.Join(home, ".claude") || h.SocketDir == "" || h.ConfigDir == "" {
		t.Errorf("%+v", h)
	}
	if h.SessionConfigDir != "" {
		t.Errorf("default directory passed to sessions: %q", h.SessionConfigDir)
	}
	if h.StartupCheckDelay != time.Second {
		t.Errorf("startup check delay: %v", h.StartupCheckDelay)
	}
	h, _ = NewSystem([]string{"HOME=" + home, "CLAUDE_CONFIG_DIR=/c"})
	if h.ClaudeConfigDir != "/c" || h.SessionConfigDir != "/c" {
		t.Errorf("env value ignored: %q %q", h.ClaudeConfigDir, h.SessionConfigDir)
	}
}

func TestNewSystemPassesConfiguredClaudeDirToSessions(t *testing.T) {
	home := t.TempDir()
	cfgDir := defaultConfigDir(runtime.GOOS, home)
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"claudeConfigDir":"/cfg"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	h, _ := NewSystem([]string{"HOME=" + home})
	if h.ClaudeConfigDir != "/cfg" || h.SessionConfigDir != "/cfg" {
		t.Errorf("config.json value ignored: %q %q", h.ClaudeConfigDir, h.SessionConfigDir)
	}
	h, _ = NewSystem([]string{"HOME=" + home, "CLAUDE_CONFIG_DIR=/env"})
	if h.ClaudeConfigDir != "/env" || h.SessionConfigDir != "/env" {
		t.Errorf("environment does not override config.json: %q %q", h.ClaudeConfigDir, h.SessionConfigDir)
	}
}

func TestPrivateSocketPathDoesNotCreateDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "claude-tower")
	h := &Hosting{SocketDir: dir}
	sock, err := h.PrivateSocketPath()
	if err != nil || sock != filepath.Join(dir, "tmux.sock") {
		t.Fatalf("sock=%q err=%v", sock, err)
	}
	if _, err := os.Stat(filepath.Dir(dir)); !os.IsNotExist(err) {
		t.Errorf("directory created (err=%v)", err)
	}
}

func TestPrivateSocketPathChecksExistingDirectory(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good")
	bad := filepath.Join(root, "bad")
	if err := os.Mkdir(good, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if sock, err := (&Hosting{SocketDir: good}).PrivateSocketPath(); err != nil || sock != filepath.Join(good, "tmux.sock") {
		t.Errorf("sock=%q err=%v", sock, err)
	}
	if _, err := (&Hosting{SocketDir: bad}).PrivateSocketPath(); err == nil || !strings.Contains(err.Error(), "0700") {
		t.Errorf("err=%v", err)
	}
}

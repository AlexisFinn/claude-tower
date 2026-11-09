package hosting

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	h := newHosting(t, &recorder{})
	cfg, err := h.LoadConfig()
	if err != nil || !reflect.DeepEqual(cfg.ClaudeCommand, []string{"claude"}) {
		t.Fatalf("defaults: %+v err=%v", cfg, err)
	}
	writeConfig(t, h, `{"claudeCommand":["/x/claude","--verbose"],"tmuxTmpDir":"/t","claudeConfigDir":"/c"}`)
	cfg, err = h.LoadConfig()
	want := Config{ClaudeCommand: []string{"/x/claude", "--verbose"}, TmuxTmpDir: "/t", ClaudeConfigDir: "/c"}
	if err != nil || !reflect.DeepEqual(cfg, want) {
		t.Errorf("got %+v err=%v", cfg, err)
	}
	writeConfig(t, h, `{"claudeCommand":[]}`)
	if cfg, _ = h.LoadConfig(); !reflect.DeepEqual(cfg.ClaudeCommand, []string{"claude"}) {
		t.Errorf("empty command not defaulted: %+v", cfg)
	}
	writeConfig(t, h, `{`)
	if _, err = h.LoadConfig(); err == nil {
		t.Error("invalid JSON accepted")
	}
}

func TestEnsureConfigWritesFileWithoutRunningServer(t *testing.T) {
	rec := &recorder{reply: func(name string, args []string) ([]byte, error) {
		if name == "tmux" {
			return nil, errors.New("no server running")
		}
		return nil, nil
	}}
	h := newHosting(t, rec)
	if err := h.EnsureConfig(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(h.ConfigDir, "tmux.conf"))
	if err != nil || !strings.Contains(string(data), "set -g default-terminal tmux-256color\n") || !strings.Contains(string(data), "set -g mouse on\n") {
		t.Fatalf("conf=%q err=%v", data, err)
	}
	for _, c := range rec.tmuxCommands() {
		if c[0] == "source-file" {
			t.Errorf("source-file sent to a stopped server")
		}
	}
}

func TestEnsureConfigFallsBackWithoutTmuxTerminfo(t *testing.T) {
	rec := &recorder{reply: func(name string, args []string) ([]byte, error) {
		if name == "infocmp" {
			return nil, errors.New("not found")
		}
		return nil, errors.New("no server running")
	}}
	h := newHosting(t, rec)
	if err := h.EnsureConfig(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(h.ConfigDir, "tmux.conf"))
	if !strings.Contains(string(data), "default-terminal screen-256color") {
		t.Errorf("conf=%q", data)
	}
}

func TestEnsureConfigReloadsRunningServer(t *testing.T) {
	rec := &recorder{}
	h := newHosting(t, rec)
	if err := h.EnsureConfig(context.Background()); err != nil {
		t.Fatal(err)
	}
	cmds := rec.tmuxCommands()
	want := [][]string{{"list-sessions"}, {"source-file", filepath.Join(h.ConfigDir, "tmux.conf")}}
	if !reflect.DeepEqual(cmds, want) {
		t.Errorf("got %v want %v", cmds, want)
	}
	rec.reply = func(name string, args []string) ([]byte, error) {
		if len(args) > 2 && args[2] == "source-file" {
			return nil, errors.New("syntax error")
		}
		return nil, nil
	}
	if err := h.EnsureConfig(context.Background()); err == nil {
		t.Error("source-file failure ignored")
	}
}

func TestEnsureConfigKeepsUnchangedFile(t *testing.T) {
	h := newHosting(t, &recorder{})
	if err := h.EnsureConfig(context.Background()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(h.ConfigDir, "tmux.conf")
	before, _ := os.Stat(path)
	if err := h.EnsureConfig(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(path)
	if !os.SameFile(before, after) {
		t.Error("file rewritten although unchanged")
	}
	entries, _ := os.ReadDir(h.ConfigDir)
	if len(entries) != 1 {
		t.Errorf("leftover files: %v", entries)
	}
}

func TestUserSocketDir(t *testing.T) {
	h := &Hosting{ConfigDir: t.TempDir(), Env: []string{"TMUX_TMPDIR=/from/env"}}
	uid := os.Getuid()
	if got, want := h.UserSocketDir(uid), fmt.Sprintf("/from/env/tmux-%d", uid); got != want {
		t.Errorf("env: got %q want %q", got, want)
	}
	writeConfig(t, h, `{"tmuxTmpDir":"/from/config"}`)
	if got, want := h.UserSocketDir(uid), fmt.Sprintf("/from/config/tmux-%d", uid); got != want {
		t.Errorf("config before env: got %q want %q", got, want)
	}
	h = &Hosting{ConfigDir: t.TempDir()}
	if got, want := h.UserSocketDir(uid), fmt.Sprintf("/tmp/tmux-%d", uid); got != want {
		t.Errorf("default: got %q want %q", got, want)
	}
}

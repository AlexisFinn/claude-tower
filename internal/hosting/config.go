package hosting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"claude-tower/internal/tmux"
)

const (
	configFile   = "config.json"
	tmuxConfFile = "tmux.conf"
	defaultTerm  = "tmux-256color"
	fallbackTerm = "screen-256color"
)

// Config is the content of config.json.
type Config struct {
	// ClaudeCommand[0] is the command to run, an absolute path or a name to
	// look up; the other elements are arguments placed before the user's.
	ClaudeCommand []string `json:"claudeCommand"`
	// TmuxTmpDir replaces TMUX_TMPDIR to find the user's tmux servers.
	TmuxTmpDir string `json:"tmuxTmpDir"`
	// ClaudeConfigDir overrides ~/.claude when CLAUDE_CONFIG_DIR is not set.
	ClaudeConfigDir string `json:"claudeConfigDir"`
}

// UserSocketDir returns the directory of the user's tmux sockets: tmux-<uid>
// under tmuxTmpDir of config.json, else under $TMUX_TMPDIR of Env, else under
// /tmp.
func (h *Hosting) UserSocketDir(uid int) string {
	tmpDir := envValue(h.Env, "TMUX_TMPDIR")
	if cfg, err := h.LoadConfig(); err == nil && cfg.TmuxTmpDir != "" {
		tmpDir = cfg.TmuxTmpDir
	}
	return tmux.SocketDir(tmpDir, uid)
}

// ConfigPath returns the path of config.json.
func (h *Hosting) ConfigPath() string {
	return filepath.Join(h.ConfigDir, configFile)
}

// LoadConfig reads config.json. A missing file yields the defaults.
func (h *Hosting) LoadConfig() (Config, error) {
	return readConfig(h.ConfigPath())
}

func readConfig(path string) (Config, error) {
	cfg := Config{ClaudeCommand: []string{"claude"}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return Config{}, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("%s : %w", path, err)
	}
	if len(cfg.ClaudeCommand) == 0 || cfg.ClaudeCommand[0] == "" {
		cfg.ClaudeCommand = []string{"claude"}
	}
	return cfg, nil
}

// tmuxConf returns the configuration of the private server, using term as
// the default terminal.
func tmuxConf(term string) string {
	return "set -g status off\n" +
		"set -g prefix None\n" +
		"set -g mouse on\n" +
		"set -g history-limit 50000\n" +
		"set -g escape-time 0\n" +
		"set -g extended-keys on\n" +
		"set -g focus-events on\n" +
		"set -g default-terminal " + term + "\n" +
		"set -g window-size latest\n"
}

// EnsureConfig writes tmux.conf when its content changed and, when the
// private server is running, makes it read the file, whether the file changed
// or not.
func (h *Hosting) EnsureConfig(ctx context.Context) error {
	return h.syncConfig(ctx, true)
}

// ensureConfigChanged is EnsureConfig except that a running private server
// reads the file only when its content changed or a previous reload failed.
func (h *Hosting) ensureConfigChanged(ctx context.Context) error {
	return h.syncConfig(ctx, false)
}

func (h *Hosting) syncConfig(ctx context.Context, reloadUnchanged bool) error {
	path, changed, err := h.writeTmuxConf(ctx)
	if err != nil {
		return err
	}
	h.mu.Lock()
	due := changed || reloadUnchanged || h.reloadDue
	h.reloadDue = h.reloadDue || changed
	h.mu.Unlock()
	if !due {
		return nil
	}
	srv, err := h.server()
	if err != nil {
		return err
	}
	// A server that is not running reads the file when it starts.
	if _, err := srv.Exec(ctx, nil, "list-sessions"); err != nil {
		h.setReloadDue(false)
		return nil
	}
	if _, err := srv.Exec(ctx, nil, "source-file", path); err != nil {
		return fmt.Errorf("source-file : %w", err)
	}
	h.setReloadDue(false)
	return nil
}

func (h *Hosting) setReloadDue(due bool) {
	h.mu.Lock()
	h.reloadDue = due
	h.mu.Unlock()
}

// writeTmuxConf returns the path of tmux.conf, written if it differs from the
// expected content, and whether it was written.
func (h *Hosting) writeTmuxConf(ctx context.Context) (string, bool, error) {
	dir := h.ConfigDir
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return "", false, err
	}
	term := defaultTerm
	if _, err := h.tmuxRun()(ctx, nil, "infocmp", defaultTerm); err != nil {
		term = fallbackTerm
	}
	path := filepath.Join(dir, tmuxConfFile)
	want := []byte(tmuxConf(term))
	if have, err := os.ReadFile(path); err == nil && bytes.Equal(have, want) {
		return path, false, nil
	}
	tmp, err := os.CreateTemp(dir, tmuxConfFile+".*")
	if err != nil {
		return "", false, err
	}
	_, werr := tmp.Write(want)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return "", false, err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return "", false, err
	}
	return path, true, nil
}

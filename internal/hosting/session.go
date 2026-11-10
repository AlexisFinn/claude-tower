package hosting

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"claude-tower/internal/tmux"
)

const maxNameAttempts = 50

// startupCheckDelay is how long a new session must stay alive to count as
// started.
var startupCheckDelay = time.Second

var commandNameRe = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

// findInPath returns the first executable called name in an absolute
// directory of pathVar.
func findInPath(pathVar, name string) (string, bool) {
	for _, dir := range filepath.SplitList(pathVar) {
		if !filepath.IsAbs(dir) {
			continue
		}
		if p := filepath.Join(dir, name); isExecutable(p) {
			return p, true
		}
	}
	return "", false
}

// resolveClaude returns the command to run: the absolute path of claude
// followed by the arguments configured in claudeCommand.
func (h *Hosting) resolveClaude(ctx context.Context, env []string) ([]string, error) {
	cfg, err := h.LoadConfig()
	if err != nil {
		return nil, err
	}
	name, extra := cfg.ClaudeCommand[0], cfg.ClaudeCommand[1:]
	path, ok := h.lookupClaude(ctx, name, env)
	if !ok {
		cfgPath, _ := h.ConfigPath()
		return nil, fmt.Errorf("claude introuvable : réglez claudeCommand dans %s", cfgPath)
	}
	return append([]string{path}, extra...), nil
}

func (h *Hosting) lookupClaude(ctx context.Context, name string, env []string) (string, bool) {
	if filepath.IsAbs(name) {
		return name, isExecutable(name)
	}
	if p, ok := findInPath(envValue(env, "PATH"), name); ok {
		return p, true
	}
	if !commandNameRe.MatchString(name) {
		return "", false
	}
	out, err := h.runShell(ctx, "command -v "+name)
	if err != nil {
		return "", false
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if filepath.IsAbs(last) && isExecutable(last) {
		return last, true
	}
	return "", false
}

// sessionName derives a tmux session name from a directory.
func sessionName(dir string) string {
	name := strings.NewReplacer(".", "-", ":", "-").Replace(filepath.Base(dir))
	if name == "" || name == "-" || name == "/" {
		return "claude"
	}
	return name
}

// ErrSessionExists reports that a session of the requested name already
// exists in the private server.
var ErrSessionExists = errors.New("la session existe déjà")

// NewAttachSession starts `claude attach id` in the session AttachPrefix+id
// of the private server, in directory dir, with env as its environment. Only
// the claude binary of claudeCommand is used: its arguments are for new
// sessions. It fails with ErrSessionExists when that session runs already.
func (h *Hosting) NewAttachSession(ctx context.Context, dir, id string, env []string) (tmux.Pane, error) {
	if h.Self == "" {
		return tmux.Pane{}, errors.New("chemin de claude-tower inconnu")
	}
	if id == "" || strings.ContainsAny(id, " \t\n/") {
		return tmux.Pane{}, fmt.Errorf("identifiant de session invalide : %q", id)
	}
	if err := h.EnsureConfig(ctx); err != nil {
		return tmux.Pane{}, err
	}
	argv, err := h.resolveClaude(ctx, env)
	if err != nil {
		return tmux.Pane{}, err
	}
	srv, err := h.server()
	if err != nil {
		return tmux.Pane{}, err
	}
	envFile, err := h.WriteEnvFile(env)
	if err != nil {
		return tmux.Pane{}, err
	}
	pane, err := h.startSession(ctx, srv, dir, tmux.AttachPrefix+id, false, []string{h.Self, "exec-env", envFile, "--", argv[0], "attach", id})
	if err != nil {
		os.Remove(envFile)
		return tmux.Pane{}, err
	}
	return pane, nil
}

// StopBackground stops the background session id of Claude Code with
// `claude stop`, which keeps its conversation. The command runs through
// exec-env with env, so that it reaches the daemon of the configuration
// directory the sessions use.
func (h *Hosting) StopBackground(ctx context.Context, id string, env []string) error {
	if h.Self == "" {
		return errors.New("chemin de claude-tower inconnu")
	}
	argv, err := h.resolveClaude(ctx, env)
	if err != nil {
		return err
	}
	envFile, err := h.WriteEnvFile(env)
	if err != nil {
		return err
	}
	out, err := h.shellRun()(ctx, nil, h.Self, "exec-env", envFile, "--", argv[0], "stop", id)
	if err != nil {
		os.Remove(envFile)
		return fmt.Errorf("claude stop %s : %w %s", id, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// NewSession starts Claude Code with args in a new session of the private
// server, in directory dir, with env as its environment. The environment
// travels in a file rather than in the arguments, which other users can read.
func (h *Hosting) NewSession(ctx context.Context, dir string, args []string, env []string) (tmux.Pane, error) {
	if h.Self == "" {
		return tmux.Pane{}, errors.New("chemin de claude-tower inconnu")
	}
	if err := h.EnsureConfig(ctx); err != nil {
		return tmux.Pane{}, err
	}
	argv, err := h.resolveClaude(ctx, env)
	if err != nil {
		return tmux.Pane{}, err
	}
	srv, err := h.server()
	if err != nil {
		return tmux.Pane{}, err
	}
	envFile, err := h.WriteEnvFile(env)
	if err != nil {
		return tmux.Pane{}, err
	}
	pane, err := h.startSession(ctx, srv, dir, sessionName(dir), true, append([]string{h.Self, "exec-env", envFile, "--"}, append(argv, args...)...))
	if err != nil {
		os.Remove(envFile)
		return tmux.Pane{}, err
	}
	return pane, nil
}

// startSession creates a session called name running command. With unique, a
// taken name gets a numeric suffix; without, it fails with ErrSessionExists.
func (h *Hosting) startSession(ctx context.Context, srv tmux.Server, dir, name string, unique bool, command []string) (tmux.Pane, error) {
	confDir, err := h.configDir()
	if err != nil {
		return tmux.Pane{}, err
	}
	base := []string{"-f", filepath.Join(confDir, tmuxConfFile), "new-session", "-d", "-P",
		"-F", "#{session_id}\t#{pane_id}", "-x", "200", "-y", "50"}
	if dir != "" {
		base = append(base, "-c", dir)
	}
	var out []byte
	for attempt := 1; ; attempt++ {
		candidate := name
		if attempt > 1 {
			candidate = fmt.Sprintf("%s-%d", name, attempt)
		}
		args := append(append(append([]string(nil), base...), "-s", candidate, "--"), command...)
		if out, err = srv.Exec(ctx, nil, args...); err == nil {
			break
		}
		duplicate := strings.Contains(err.Error(), "duplicate session")
		if duplicate && !unique {
			return tmux.Pane{}, ErrSessionExists
		}
		if !duplicate || attempt >= maxNameAttempts {
			return tmux.Pane{}, fmt.Errorf("new-session : %w", err)
		}
	}
	sessionID, paneID, ok := strings.Cut(strings.TrimSpace(string(out)), "\t")
	if !ok {
		return tmux.Pane{}, fmt.Errorf("réponse inattendue de new-session : %q", out)
	}
	return h.confirmStarted(ctx, srv, sessionID, paneID)
}

// confirmStarted waits, then returns the pane if it still exists.
func (h *Hosting) confirmStarted(ctx context.Context, srv tmux.Server, sessionID, paneID string) (tmux.Pane, error) {
	select {
	case <-time.After(startupCheckDelay):
	case <-ctx.Done():
		return tmux.Pane{}, ctx.Err()
	}
	panes, err := srv.Panes(ctx)
	for _, p := range panes {
		if p.ID == paneID {
			return p, nil
		}
	}
	if err != nil {
		return tmux.Pane{}, fmt.Errorf("la session s'est arrêtée immédiatement : %w", err)
	}
	return tmux.Pane{}, fmt.Errorf("la session s'est arrêtée immédiatement (%s)", sessionID)
}

// Kill destroys the session of a pane of the private server, view sessions
// grouped with it first. The group is read from the server, not from the
// pane, which may predate the view sessions. Panes of other servers are
// refused.
func (h *Hosting) Kill(ctx context.Context, pane tmux.Pane) error {
	sock, err := h.PrivateSocket()
	if err != nil {
		return err
	}
	if pane.Server.Socket != sock {
		return errors.New("seules les sessions lancées par claude-tower peuvent être arrêtées")
	}
	views, err := viewSessionsOf(ctx, pane)
	if err != nil {
		return err
	}
	for _, id := range views {
		if err := pane.Server.KillSession(ctx, id); err != nil {
			return fmt.Errorf("kill-session : %w", err)
		}
	}
	if err := pane.Server.KillSession(ctx, pane.SessionID); err != nil {
		return fmt.Errorf("kill-session : %w", err)
	}
	return nil
}

// viewSessionsOf lists the IDs of the view sessions that the server groups,
// at this moment, with the pane's session.
func viewSessionsOf(ctx context.Context, pane tmux.Pane) ([]string, error) {
	out, err := pane.Server.Exec(ctx, nil, "list-sessions", "-F", "#{session_id}\t#{session_name}\t#{session_group}")
	if err != nil {
		return nil, fmt.Errorf("list-sessions : %w", err)
	}
	type entry struct{ id, name, group string }
	var entries []entry
	group := ""
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			continue
		}
		entries = append(entries, entry{f[0], f[1], f[2]})
		if f[0] == pane.SessionID {
			group = f[2]
		}
	}
	if group == "" {
		return nil, nil
	}
	var ids []string
	for _, e := range entries {
		if strings.HasPrefix(e.name, tmux.ViewPrefix) && e.group == group {
			ids = append(ids, e.id)
		}
	}
	return ids, nil
}

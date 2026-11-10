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
	"unicode"

	"claude-tower/internal/tmux"
)

const maxNameAttempts = 50

var commandNameRe = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

// backgroundIDRe accepts the ids of background sessions, which become
// command arguments and part of a tmux session name.
var backgroundIDRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)

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
		return nil, fmt.Errorf("claude introuvable : réglez claudeCommand dans %s", h.ConfigPath())
	}
	return append([]string{path}, extra...), nil
}

// ClaudePath returns the absolute path of the claude executable, found as a
// session would find it with env as its environment.
func (h *Hosting) ClaudePath(ctx context.Context, env []string) (string, error) {
	argv, err := h.resolveClaude(ctx, env)
	if err != nil {
		return "", err
	}
	return argv[0], nil
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

// errSessionExists reports that a session of the requested name already
// exists in the private server.
var errSessionExists = errors.New("la session existe déjà")

// NewAttachSession starts `claude attach id` in the session AttachPrefix+id
// of the private server, in directory dir, with env as its environment. Only
// the claude binary of claudeCommand is used: its arguments are for new
// sessions. It fails with errSessionExists when that session runs already.
func (h *Hosting) NewAttachSession(ctx context.Context, dir, id string, env []string) (tmux.Pane, error) {
	if h.Self == "" {
		return tmux.Pane{}, errors.New("chemin de claude-tower inconnu")
	}
	if !backgroundIDRe.MatchString(id) {
		return tmux.Pane{}, fmt.Errorf("identifiant de session invalide : %q", id)
	}
	if err := h.ensureConfigChanged(ctx); err != nil {
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

// maxNameRunes is the length of the longest name given to a new session.
const maxNameRunes = 60

// cleanName returns name without control characters nor surrounding spaces,
// cut to maxNameRunes characters.
func cleanName(name string) string {
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name))
	if r := []rune(name); len(r) > maxNameRunes {
		name = strings.TrimSpace(string(r[:maxNameRunes]))
	}
	return name
}

// NameArgs returns the arguments that give a new session its name, none when
// the cleaned name is empty. The name stays one argument, which the command
// does not pass through a shell, and is joined to its flag so that a name
// starting with "-" is not read as an option.
func NameArgs(name string) []string {
	if name = cleanName(name); name == "" {
		return nil
	}
	return []string{"--name=" + name}
}

// NewSession starts Claude Code with args in a new session of the private
// server, in directory dir, with env as its environment. The environment
// travels in a file rather than in the arguments, which other users can read.
func (h *Hosting) NewSession(ctx context.Context, dir string, args []string, env []string) (tmux.Pane, error) {
	if h.Self == "" {
		return tmux.Pane{}, errors.New("chemin de claude-tower inconnu")
	}
	if err := h.ensureConfigChanged(ctx); err != nil {
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
// taken name gets a numeric suffix; without, it fails with errSessionExists.
func (h *Hosting) startSession(ctx context.Context, srv tmux.Server, dir, name string, unique bool, command []string) (tmux.Pane, error) {
	base := []string{"-f", filepath.Join(h.ConfigDir, tmuxConfFile), "new-session", "-d", "-P",
		"-F", "#{session_id}\t#{pane_id}", "-x", "200", "-y", "50"}
	if dir != "" {
		base = append(base, "-c", dir)
	}
	var (
		out []byte
		err error
	)
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
			return tmux.Pane{}, errSessionExists
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

// confirmStarted waits StartupCheckDelay, then returns the pane if it still
// exists.
func (h *Hosting) confirmStarted(ctx context.Context, srv tmux.Server, sessionID, paneID string) (tmux.Pane, error) {
	select {
	case <-time.After(h.StartupCheckDelay):
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
	views, err := pane.Server.ViewSessionsOf(ctx, pane)
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

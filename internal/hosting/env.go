package hosting

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// terminalVars are the variables SessionEnv removes from the caller's
// environment: those describing the caller's terminal or tmux client, and the
// terminal size, which the session does not inherit.
var terminalVars = map[string]bool{
	"TMUX": true, "TMUX_PANE": true, "TERM": true, "TERM_PROGRAM": true,
	"TERM_PROGRAM_VERSION": true, "COLUMNS": true, "LINES": true,
}

// paneVars are the variables ExecEnv takes from the environment tmux gave to
// the pane, replacing those of the stored environment. A variable the pane
// does not have is not set.
var paneVars = []string{"TERM", "TERM_PROGRAM", "TERM_PROGRAM_VERSION", "TMUX", "TMUX_PANE", "COLORTERM"}

const (
	envFilePrefix = "env-"
	envFileMaxAge = time.Minute
)

var envNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

func envName(entry string) string {
	name, _, _ := strings.Cut(entry, "=")
	return name
}

func envValue(env []string, name string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if n, v, ok := strings.Cut(env[i], "="); ok && n == name {
			return v
		}
	}
	return ""
}

func envHas(env []string, name string) bool {
	for _, e := range env {
		if envName(e) == name {
			return true
		}
	}
	return false
}

// unsetEnv returns env without the entries of name.
func unsetEnv(env []string, name string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		if envName(e) != name {
			out = append(out, e)
		}
	}
	return out
}

// setEnv returns env with name set to value, replacing any earlier entry.
func setEnv(env []string, name, value string) []string {
	return append(unsetEnv(env, name), name+"="+value)
}

// SessionEnv returns base without the variables that describe the terminal of
// the caller, nor an empty CLAUDE_CONFIG_DIR. A non-empty configDir becomes
// CLAUDE_CONFIG_DIR.
func SessionEnv(base []string, configDir string) []string {
	out := make([]string, 0, len(base)+1)
	for _, e := range base {
		if !terminalVars[envName(e)] && e != "CLAUDE_CONFIG_DIR=" {
			out = append(out, e)
		}
	}
	if configDir != "" {
		out = setEnv(out, "CLAUDE_CONFIG_DIR", configDir)
	}
	return out
}

func isUTF8(locale string) bool {
	u := strings.ToUpper(locale)
	return strings.Contains(u, "UTF-8") || strings.Contains(u, "UTF8")
}

// utf8Locale reports whether one of LC_ALL, LC_CTYPE or LANG, the first set
// being the one that counts, names a UTF-8 locale.
func utf8Locale(env []string) bool {
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := envValue(env, name); v != "" {
			return isUTF8(v)
		}
	}
	return false
}

// WithUTF8Locale returns env with a UTF-8 locale when the one in effect is
// not.
func WithUTF8Locale(env []string) []string {
	return withUTF8Locale(env, runtime.GOOS)
}

// withUTF8Locale returns env unchanged when its locale is UTF-8. Otherwise
// LANG, when it is not UTF-8, and every non-UTF-8 LC_ALL or LC_CTYPE become a
// UTF-8 locale, so that the one in effect is UTF-8 whichever variable counts.
func withUTF8Locale(env []string, goos string) []string {
	if utf8Locale(env) {
		return env
	}
	locale := "C.UTF-8"
	if goos == "darwin" {
		locale = "en_US.UTF-8"
	}
	out := env
	if !isUTF8(envValue(env, "LANG")) {
		out = setEnv(out, "LANG", locale)
	}
	for _, name := range []string{"LC_ALL", "LC_CTYPE"} {
		if v := envValue(out, name); v != "" && !isUTF8(v) {
			out = setEnv(out, name, locale)
		}
	}
	return out
}

// parseEnvBlock parses NUL-separated NAME=value entries and skips anything
// that is not one, such as text a shell prints at startup.
func parseEnvBlock(data []byte) []string {
	var out []string
	for _, e := range bytes.Split(data, []byte{0}) {
		if envNameRe.Match(e) {
			out = append(out, string(e))
		}
	}
	return out
}

func encodeEnvBlock(env []string) []byte {
	var buf bytes.Buffer
	for _, e := range env {
		if strings.IndexByte(e, 0) >= 0 || !strings.Contains(e, "=") {
			continue
		}
		buf.WriteString(e)
		buf.WriteByte(0)
	}
	return buf.Bytes()
}

// mergeEnv returns base completed with the variables of under that base lacks.
func mergeEnv(base, under []string) []string {
	out := append([]string(nil), base...)
	for _, e := range under {
		if !envHas(base, envName(e)) {
			out = append(out, e)
		}
	}
	return out
}

// mergeLoginEnv returns app completed with the variables of the login shell
// that app lacks. PATH lists the entries of the shell's PATH first, then those
// of app's PATH the shell's lacks.
func mergeLoginEnv(app, shell []string) []string {
	// An empty CLAUDE_CONFIG_DIR counts as unset and leaves the login shell's.
	if envValue(app, "CLAUDE_CONFIG_DIR") == "" {
		app = unsetEnv(app, "CLAUDE_CONFIG_DIR")
	}
	out := mergeEnv(app, shell)
	if !envHas(app, "PATH") || !envHas(shell, "PATH") {
		return out
	}
	path := joinPaths(envValue(shell, "PATH"), envValue(app, "PATH"))
	for i, e := range out {
		if envName(e) == "PATH" {
			out[i] = "PATH=" + path
		}
	}
	return out
}

// joinPaths returns the entries of first followed by the non-empty entries of
// second that first lacks.
func joinPaths(first, second string) string {
	entries := filepath.SplitList(first)
	seen := make(map[string]bool, len(entries))
	for _, d := range entries {
		seen[d] = true
	}
	for _, d := range filepath.SplitList(second) {
		if d != "" && !seen[d] {
			entries = append(entries, d)
			seen[d] = true
		}
	}
	return strings.Join(entries, string(os.PathListSeparator))
}

// runShell runs the login shell with a command string. A shell whose startup
// files leave a daemon holding its output still counts as successful when it
// produced output.
func (h *Hosting) runShell(ctx context.Context, command string) ([]byte, error) {
	out, err := h.shellRun()(ctx, nil, h.shell(), "-ilc", command)
	if err != nil && !(errors.Is(err, exec.ErrWaitDelay) && len(bytes.TrimSpace(out)) > 0) {
		return nil, err
	}
	return out, nil
}

// loginRetry is how long a failure of the login shell is kept before the
// shell is run again.
const loginRetry = time.Minute

// loginCall is one run of the login shell that concurrent callers wait for.
type loginCall struct {
	done chan struct{} // closed when the run ended
}

func (h *Hosting) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}

// loginShellEnv returns the environment of the calling process completed with
// that of an interactive login shell: the variables of the calling process
// take precedence, except PATH, which lists the shell's entries first and then
// the calling process's other ones. The result is computed once. A failure is
// kept for loginRetry, during which calls return it without running the shell.
// Concurrent calls share one run of the shell and do not hold h.mu while it
// runs.
func (h *Hosting) loginShellEnv(ctx context.Context) ([]string, error) {
	for {
		h.mu.Lock()
		if h.loginDone {
			env := h.loginEnv
			h.mu.Unlock()
			return env, nil
		}
		if h.loginErr != nil && h.now().Sub(h.loginFailedAt) < loginRetry {
			err := h.loginErr
			h.mu.Unlock()
			return nil, err
		}
		if call := h.loginCall; call != nil {
			h.mu.Unlock()
			select {
			case <-call.done:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			continue
		}
		call := &loginCall{done: make(chan struct{})}
		h.loginCall = call
		h.mu.Unlock()

		out, err := h.runShell(ctx, "env -0")

		h.mu.Lock()
		h.loginCall = nil
		var env []string
		switch {
		case err == nil:
			env = mergeLoginEnv(h.Env, parseEnvBlock(out))
			h.loginEnv, h.loginDone, h.loginErr = env, true, nil
		case ctx.Err() != nil:
			// A canceled caller says nothing about the shell: no failure kept.
			err = fmt.Errorf("environnement du shell de connexion : %w", err)
		default:
			err = fmt.Errorf("environnement du shell de connexion : %w", err)
			h.loginErr, h.loginFailedAt = err, h.now()
		}
		h.mu.Unlock()
		close(call.done)
		return env, err
	}
}

// AppEnv returns the environment sessions started by the application inherit:
// the caller's own, or, when the application has no terminal (desktop
// launcher), completed with the login shell's as loginShellEnv does.
func (h *Hosting) AppEnv(ctx context.Context) []string {
	if envHas(h.Env, "TERM") {
		return h.Env
	}
	if env, err := h.loginShellEnv(ctx); err == nil {
		return env
	}
	return h.Env
}

// ResolveClaudeConfigDir sets ClaudeConfigDir and SessionConfigDir to the
// non-empty CLAUDE_CONFIG_DIR of AppEnv when neither the environment of the
// application nor config.json sets one. Without a terminal, this runs the
// login shell.
func (h *Hosting) ResolveClaudeConfigDir(ctx context.Context) {
	if h.SessionConfigDir != "" {
		return
	}
	if dir := envValue(h.AppEnv(ctx), "CLAUDE_CONFIG_DIR"); dir != "" {
		h.ClaudeConfigDir = dir
		h.SessionConfigDir = dir
	}
}

// ClaudeDir returns the Claude Code configuration directory read by the
// application, ClaudeConfigDir.
func (h *Hosting) ClaudeDir() string {
	return h.ClaudeConfigDir
}

// NewSessionEnv returns the environment of a session started by the
// application: AppEnv with a UTF-8 locale and without the terminal variables
// of the caller. CLAUDE_CONFIG_DIR is SessionConfigDir, or absent when that is
// empty, so that the session always writes to the registry the application
// reads, even when the login shell sets it after ResolveClaudeConfigDir failed.
func (h *Hosting) NewSessionEnv(ctx context.Context) []string {
	env := SessionEnv(WithUTF8Locale(h.AppEnv(ctx)), h.SessionConfigDir)
	if h.SessionConfigDir == "" {
		env = unsetEnv(env, "CLAUDE_CONFIG_DIR")
	}
	return env
}

// WriteEnvFile writes env to a new file in the private socket directory and
// returns its path. The file is readable by its owner only.
func (h *Hosting) WriteEnvFile(env []string) (string, error) {
	sock, err := h.PrivateSocket()
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(sock)
	f, err := os.CreateTemp(dir, envFilePrefix+"*")
	if err != nil {
		return "", err
	}
	_, werr := f.Write(encodeEnvBlock(env))
	if err := errors.Join(werr, f.Close()); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// SweepEnvFiles removes the environment files of the private socket directory
// that are older than a minute at now, left by sessions that never started.
func (h *Hosting) SweepEnvFiles(now time.Time) error {
	sock, err := h.PrivateSocket()
	if err != nil {
		return err
	}
	dir := filepath.Dir(sock)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), envFilePrefix) || !e.Type().IsRegular() {
			continue
		}
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > envFileMaxAge {
			if err := os.Remove(filepath.Join(dir, e.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// ReadEnvFile reads an environment file written by WriteEnvFile and removes
// it.
func ReadEnvFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	os.Remove(path)
	if err != nil {
		return nil, err
	}
	return parseEnvBlock(data), nil
}

// ExecEnv returns the environment of the session process: the entries of the
// file, except for the terminal variables, which come from own, the
// environment tmux gave to the pane.
func ExecEnv(fileEnv, own []string) []string {
	keep := make(map[string]bool, len(paneVars))
	for _, n := range paneVars {
		keep[n] = true
	}
	out := make([]string, 0, len(fileEnv)+len(paneVars))
	for _, e := range fileEnv {
		if !keep[envName(e)] {
			out = append(out, e)
		}
	}
	for _, n := range paneVars {
		if envHas(own, n) {
			out = append(out, n+"="+envValue(own, n))
		}
	}
	return out
}

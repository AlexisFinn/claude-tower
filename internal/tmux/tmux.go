// Package tmux drives tmux servers through their command line: it finds the
// binary, lists panes and clients, captures and pastes into panes, and manages
// the temporary sessions used to show a pane in an embedded terminal.
package tmux

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// defaultTimeout bounds every command run by DefaultRunner.
const defaultTimeout = 2 * time.Second

// Runner runs an external command and returns its standard output. The error
// of a failed command includes its standard error.
type Runner func(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error)

// ExecRunner returns a Runner that runs commands without a shell and kills
// them after timeout. A child left running by the command cannot hold the
// call longer than a short grace period after the kill. When a command exits
// successfully but leaves a child holding its output, the error wraps
// exec.ErrWaitDelay and the output read so far is still returned.
func ExecRunner(timeout time.Duration) Runner {
	return func(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Stdin = stdin
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		cmd.WaitDelay = 100 * time.Millisecond
		if err := cmd.Run(); err != nil {
			if errors.Is(err, exec.ErrWaitDelay) && ctx.Err() == nil {
				return stdout.Bytes(), fmt.Errorf("%s: %w", name, err)
			}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return nil, fmt.Errorf("%s: délai de %s dépassé: %w", name, timeout, err)
			}
			if msg := strings.TrimSpace(stderr.String()); msg != "" {
				return nil, fmt.Errorf("%s: %w: %s", name, err, msg)
			}
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		return stdout.Bytes(), nil
	}
}

// DefaultRunner is the Runner used when a Server has none.
var DefaultRunner = ExecRunner(defaultTimeout)

// Server is one tmux server, identified by its socket. Every command uses -S.
type Server struct {
	Socket string
	Bin    string // default: "tmux"
	Run    Runner // default: DefaultRunner
}

// Exec runs a tmux command on the server and returns its standard output.
func (s Server) Exec(ctx context.Context, stdin io.Reader, args ...string) ([]byte, error) {
	run, bin := s.Run, s.Bin
	if run == nil {
		run = DefaultRunner
	}
	if bin == "" {
		bin = "tmux"
	}
	return run(ctx, stdin, bin, append([]string{"-S", s.Socket}, args...)...)
}

// ServerNotRunning reports whether err tells that no tmux server listens on
// the socket, as opposed to a server that answers too slowly.
func ServerNotRunning(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no server running") || strings.Contains(msg, "error connecting")
}

// fallbackDirs are searched after PATH, which is reduced when the application
// starts from a desktop launcher or the Finder.
var fallbackDirs = []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin"}

// FindBinary locates the tmux binary: PATH first, then the usual install
// directories.
func FindBinary(lookPath func(string) (string, error)) (string, error) {
	if p, err := lookPath("tmux"); err == nil {
		return p, nil
	}
	for _, dir := range fallbackDirs {
		if p, err := lookPath(filepath.Join(dir, "tmux")); err == nil {
			return p, nil
		}
	}
	return "", errors.New("tmux introuvable : installez tmux 3.2 ou plus")
}

var versionRe = regexp.MustCompile(`^(?:next-)?([0-9]+)\.([0-9]+)`)

// CheckVersion fails unless the tmux binary is version 3.2 or later. Builds
// from the development branch ("master") are accepted.
func CheckVersion(ctx context.Context, bin string, run Runner) error {
	out, err := run(ctx, nil, bin, "-V")
	if err != nil {
		return fmt.Errorf("tmux -V : %w", err)
	}
	version := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "tmux"))
	if version == "master" {
		return nil
	}
	m := versionRe.FindStringSubmatch(version)
	if m == nil {
		return fmt.Errorf("version de tmux non reconnue : %q", version)
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	if major < 3 || (major == 3 && minor < 2) {
		return fmt.Errorf("tmux %s est trop ancien : tmux 3.2 ou plus est requis", version)
	}
	return nil
}

// SocketDir returns the directory holding the sockets of the user's tmux
// servers: $TMUX_TMPDIR (default /tmp) plus tmux-<uid>.
func SocketDir(tmuxTmpDir string, uid int) string {
	if tmuxTmpDir == "" {
		tmuxTmpDir = "/tmp"
	}
	return filepath.Join(tmuxTmpDir, "tmux-"+strconv.Itoa(uid))
}

// Servers returns one Server per socket found in dir. They are only
// candidates: a socket may be orphaned, and callers ignore the servers whose
// commands fail.
func Servers(dir string, bin string, run Runner) []Server {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Server
	for _, e := range entries {
		if e.Type()&fs.ModeSocket != 0 {
			out = append(out, Server{Socket: filepath.Join(dir, e.Name()), Bin: bin, Run: run})
		}
	}
	return out
}

// CandidateServers returns the servers found in the user's socket directory
// and, when privateSocket is not empty, in the directory of the private
// server's socket.
func CandidateServers(userDir, privateSocket, bin string, run Runner) []Server {
	servers := Servers(userDir, bin, run)
	if privateSocket != "" {
		servers = append(servers, Servers(filepath.Dir(privateSocket), bin, run)...)
	}
	return servers
}

// Package hosting runs Claude Code sessions in a private tmux server that
// outlives the application, and prepares their environment.
package hosting

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"claude-tower/internal/tmux"
)

const (
	socketName   = "tmux.sock"
	appDirName   = "claude-tower"
	shellTimeout = 10 * time.Second
	dirMode      = 0o700
)

// Hosting holds everything the private server needs from the system. Every
// field can be replaced, so that tests never touch the real directories,
// shell or environment. A Hosting must not be copied once used.
type Hosting struct {
	SocketDir       string                                      // directory of the private socket; default: PrivateSocketDir
	ConfigDir       string                                      // directory of config.json and tmux.conf; default: DefaultConfigDir
	Self            string                                      // path of the claude-tower binary run by exec-env
	Tmux            tmux.Runner                                 // runs tmux and infocmp; default: tmux.DefaultRunner
	Bin             string                                      // tmux binary; default: "tmux"
	Env             []string                                    // environment of the calling process
	ClaudeConfigDir string                                      // Claude Code configuration directory read by the application
	Shell           string                                      // login shell; default: $SHELL from Env, then /bin/sh
	ShellRun        tmux.Runner                                 // runs the login shell; default: a runner limited to 10 seconds
	Exec            func(path string, argv, env []string) error // replaces the process; default: syscall.Exec
	Cwd             func() (string, error)                      // default: os.Getwd

	mu        sync.Mutex
	loginEnv  []string
	loginDone bool
}

// NewSystem returns a Hosting wired to the real system, with environ as the
// environment of the calling process.
func NewSystem(environ []string) (*Hosting, error) {
	home := envValue(environ, "HOME")
	if home == "" {
		var err error
		if home, err = os.UserHomeDir(); err != nil {
			return nil, err
		}
	}
	self, _ := os.Executable()
	bin, _ := tmux.FindBinary(exec.LookPath)
	h := &Hosting{
		SocketDir: PrivateSocketDir(runtime.GOOS, home, func(k string) string { return envValue(environ, k) }),
		ConfigDir: DefaultConfigDir(runtime.GOOS, home),
		Self:      self,
		Bin:       bin,
		Env:       environ,
	}
	h.ClaudeConfigDir = envValue(environ, "CLAUDE_CONFIG_DIR")
	if cfg, err := h.LoadConfig(); h.ClaudeConfigDir == "" && err == nil {
		h.ClaudeConfigDir = cfg.ClaudeConfigDir
	}
	if h.ClaudeConfigDir == "" {
		h.ClaudeConfigDir = filepath.Join(home, ".claude")
	}
	return h, nil
}

// PrivateSocketDir returns the default directory of the private socket for
// the given system.
func PrivateSocketDir(goos, home string, getenv func(string) string) string {
	if goos == "darwin" {
		return filepath.Join(home, "Library", "Application Support", appDirName)
	}
	if dir := getenv("XDG_RUNTIME_DIR"); dir != "" {
		return filepath.Join(dir, appDirName)
	}
	return filepath.Join(home, ".local", "state", appDirName)
}

// DefaultConfigDir returns the directory of config.json and tmux.conf for the
// given system.
func DefaultConfigDir(goos, home string) string {
	if goos == "darwin" {
		return filepath.Join(home, "Library", "Application Support", appDirName)
	}
	return filepath.Join(home, ".config", appDirName)
}

// PrivateSocket returns the socket of the private server at its default
// location, creating and checking its directory.
func PrivateSocket() (string, error) {
	dir, err := defaultSocketDir()
	if err != nil {
		return "", err
	}
	return socketIn(dir)
}

func defaultSocketDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return PrivateSocketDir(runtime.GOOS, home, os.Getenv), nil
}

// PrivateSocket returns the socket of the private server, creating and
// checking its directory.
func (h *Hosting) PrivateSocket() (string, error) {
	if h.SocketDir == "" {
		return PrivateSocket()
	}
	return socketIn(h.SocketDir)
}

// PrivateSocketPath returns the socket of the private server without creating
// its directory. An existing directory is checked like PrivateSocket does.
func (h *Hosting) PrivateSocketPath() (string, error) {
	dir := h.SocketDir
	if dir == "" {
		var err error
		if dir, err = defaultSocketDir(); err != nil {
			return "", err
		}
	}
	info, err := os.Lstat(dir)
	if err == nil {
		err = checkSocketDir(dir, info, os.Getuid())
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	return filepath.Join(dir, socketName), nil
}

func socketIn(dir string) (string, error) {
	if err := ensureSocketDir(dir, os.Getuid()); err != nil {
		return "", err
	}
	return filepath.Join(dir, socketName), nil
}

// ensureSocketDir creates dir with mode 0700, or checks that an existing one
// is a real directory owned by uid that only its owner can access.
func ensureSocketDir(dir string, uid int) error {
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	return checkSocketDir(dir, info, uid)
}

func checkSocketDir(dir string, info os.FileInfo, uid int) error {
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%s n'est pas un dossier ordinaire", dir)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("propriétaire du dossier de la socket illisible")
	}
	if int(st.Uid) != uid {
		return fmt.Errorf("%s n'appartient pas à l'utilisateur courant", dir)
	}
	if info.Mode().Perm() != dirMode {
		return fmt.Errorf("%s doit avoir le mode 0700 (mode actuel : %04o)", dir, info.Mode().Perm())
	}
	return nil
}

func (h *Hosting) tmuxRun() tmux.Runner {
	if h.Tmux != nil {
		return h.Tmux
	}
	return tmux.DefaultRunner
}

func (h *Hosting) shellRun() tmux.Runner {
	if h.ShellRun != nil {
		return h.ShellRun
	}
	return tmux.ExecRunner(shellTimeout)
}

func (h *Hosting) tmuxBin() string {
	if h.Bin != "" {
		return h.Bin
	}
	return "tmux"
}

func (h *Hosting) shell() string {
	if h.Shell != "" {
		return h.Shell
	}
	if s := envValue(h.Env, "SHELL"); s != "" {
		return s
	}
	return "/bin/sh"
}

func (h *Hosting) configDir() (string, error) {
	if h.ConfigDir != "" {
		return h.ConfigDir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return DefaultConfigDir(runtime.GOOS, home), nil
}

func (h *Hosting) execFn() func(string, []string, []string) error {
	if h.Exec != nil {
		return h.Exec
	}
	return syscall.Exec
}

// server returns the private tmux server.
func (h *Hosting) server() (tmux.Server, error) {
	sock, err := h.PrivateSocket()
	if err != nil {
		return tmux.Server{}, err
	}
	return tmux.Server{Socket: sock, Bin: h.tmuxBin(), Run: h.tmuxRun()}, nil
}

// Package core builds the picture of the Claude Code sessions of the machine
// and acts on them: cards, screens, prompts, focus, new and killed sessions.
package core

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"claude-tower/internal/background"
	"claude-tower/internal/focus"
	"claude-tower/internal/hosting"
	"claude-tower/internal/locate"
	"claude-tower/internal/registry"
	"claude-tower/internal/tmux"
	"claude-tower/internal/transcript"
)

// Options configures a Core. A zero field takes the default noted beside it.
type Options struct {
	// Registry reads the sessions of Claude Code. An empty Dir becomes the
	// directory of Hosting when it has one, else $CLAUDE_CONFIG_DIR, else
	// .claude in Home. Alive and ReadFile keep the defaults of registry.Reader.
	Registry registry.Reader
	// Hosting runs and stops the sessions of the private server. Without it,
	// there is no private server and NewSession and Kill fail.
	Hosting *hosting.Hosting
	// Home shortens the paths shown. Default: the user's home directory.
	Home string
	// UserSocketDir holds the sockets of the user's tmux servers. Default:
	// Hosting.UserSocketDir when Hosting is set, else tmux-<uid> under
	// $TMUX_TMPDIR (/tmp when unset).
	UserSocketDir string
	// Bin is the tmux binary. Default: found on PATH and in the usual
	// directories.
	Bin string
	// Run runs tmux. Default: tmux.DefaultRunner.
	Run tmux.Runner
	// ReadTree reads the process tree. Default: locate.ReadTree.
	ReadTree func(context.Context, tmux.Runner) (locate.Tree, error)
	// ReadAttachments lists the `claude attach` clients. Default:
	// locate.ReadAttachments.
	ReadAttachments func(context.Context, tmux.Runner) (locate.Attachments, error)
	// ReadFDs lists the targets of the open file descriptors of a process, to
	// find the background shells still running. Default:
	// background.DefaultFDReader, nil on systems without /proc.
	ReadFDs background.FDReader
	// Raise brings the window owning one of the processes to the foreground.
	// Default: focus.Raise.
	Raise func(pids []int) error
	// Supported reports whether Raise can work. Default: focus.Supported.
	Supported func() bool
	// Now returns the current time. Default: time.Now.
	Now func() time.Time
}

// Core is safe for concurrent use.
type Core struct {
	opts   Options
	finder *transcript.Finder

	mu        sync.Mutex
	snap      snapshot
	launches  map[string]*launch // keyed by pane ID
	launchSeq uint64
	failures  []StartFailure

	attaching   map[string]bool      // background ids whose attach session is being created
	attachTried map[string]time.Time // last attach attempt, per background id

	bgMu     sync.Mutex
	trackers map[string]*background.Tracker // keyed by session ID
}

// New checks that tmux is installed and recent enough, and returns a Core.
func New(opts Options) (*Core, error) {
	if opts.Run == nil {
		opts.Run = tmux.DefaultRunner
	}
	if opts.Bin == "" {
		bin, err := tmux.FindBinary(exec.LookPath)
		if err != nil {
			return nil, err
		}
		opts.Bin = bin
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := tmux.CheckVersion(ctx, opts.Bin, opts.Run); err != nil {
		return nil, err
	}
	if opts.ReadTree == nil {
		opts.ReadTree = locate.ReadTree
	}
	if opts.ReadAttachments == nil {
		opts.ReadAttachments = locate.ReadAttachments
	}
	if opts.ReadFDs == nil {
		opts.ReadFDs = background.DefaultFDReader()
	}
	if opts.Raise == nil {
		opts.Raise = focus.Raise
	}
	if opts.Supported == nil {
		opts.Supported = focus.Supported
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Home == "" {
		opts.Home, _ = os.UserHomeDir()
	}
	if opts.UserSocketDir == "" {
		opts.UserSocketDir = defaultUserSocketDir(opts.Hosting)
	}
	if opts.Registry.Dir == "" {
		opts.Registry.Dir = defaultRegistryDir(opts.Hosting, opts.Home)
	}
	return &Core{
		opts:     opts,
		finder:   transcript.NewFinder(opts.Registry.Dir),
		trackers: map[string]*background.Tracker{},
		launches: map[string]*launch{},

		attaching:   map[string]bool{},
		attachTried: map[string]time.Time{},
	}, nil
}

func defaultUserSocketDir(h *hosting.Hosting) string {
	if h != nil {
		return h.UserSocketDir(os.Getuid())
	}
	return tmux.SocketDir(os.Getenv("TMUX_TMPDIR"), os.Getuid())
}

func defaultRegistryDir(h *hosting.Hosting, home string) string {
	if h != nil && h.ClaudeConfigDir != "" {
		return h.ClaudeConfigDir
	}
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir
	}
	return filepath.Join(home, ".claude")
}

// privateSocket returns the socket of the private server, "" when there is
// none to use.
func (c *Core) privateSocket() string {
	if c.opts.Hosting == nil {
		return ""
	}
	sock, err := c.opts.Hosting.PrivateSocket()
	if err != nil {
		return ""
	}
	return sock
}

// servers lists the candidate tmux servers: the user's and the private one.
func (c *Core) servers(private string) []tmux.Server {
	return tmux.CandidateServers(c.opts.UserSocketDir, private, c.opts.Bin, c.opts.Run)
}

// Sweep destroys the view sessions left without client on every server, and
// the environment files of sessions that never started.
func (c *Core) Sweep(ctx context.Context) {
	for _, srv := range c.servers(c.privateSocket()) {
		srv.SweepViewSessions(ctx)
	}
	if c.opts.Hosting != nil {
		c.opts.Hosting.SweepEnvFiles(c.opts.Now())
	}
}

// PaneOf returns the pane hosting the session with the given PID, as of the
// last snapshot.
func (c *Core) PaneOf(pid int) (tmux.Pane, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	p, ok := c.snap.panes[pid]
	return p, ok
}

var errNotInTmux = errors.New("session hors tmux ou inconnue")

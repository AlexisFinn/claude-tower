// Package cli implements the claude-tower subcommands that run in a terminal.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"claude-tower/internal/hosting"
	"claude-tower/internal/locate"
	"claude-tower/internal/registry"
	"claude-tower/internal/tmux"
	"claude-tower/internal/transcript"
	"claude-tower/internal/view"
)

const (
	usage = "usage: claude-tower [bar|list] [--dir <config dir>] [--max-width <n>] [--client <name>]\n" +
		"       claude-tower run [claude arguments...]"

	defaultMaxWidth = 100

	// Values of the location column of list.
	locPrivate = "privé"
	locTmux    = "tmux"
	locOutside = "hors tmux"
	locUnknown = "?"
)

// now is the clock used for durations; tests replace it.
var now = time.Now

// deps holds what the subcommands need from the system; tests replace it.
type deps struct {
	// locations maps session PIDs to a location column value.
	locations func(ctx context.Context, sessions []registry.Session) map[int]string
	// run replaces the process with Claude Code hosted in the private server.
	run func(args []string) error
	// exec replaces the process with a command.
	exec func(path string, argv, env []string) error
	// environ returns the environment of the process.
	environ func() []string
}

type opts struct {
	cmd      string
	dir      string
	maxWidth int
	client   string
}

// parseArgs accepts the flags before and after the optional subcommand.
func parseArgs(args []string) (opts, error) {
	var o opts
	fs := flag.NewFlagSet("claude-tower", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.dir, "dir", "", "")
	fs.IntVar(&o.maxWidth, "max-width", defaultMaxWidth, "")
	fs.StringVar(&o.client, "client", "", "")
	if err := fs.Parse(args); err != nil {
		return opts{}, err
	}
	rest := fs.Args()
	if len(rest) > 0 {
		o.cmd = rest[0]
		if err := fs.Parse(rest[1:]); err != nil {
			return opts{}, err
		}
		rest = fs.Args()
	}
	if len(rest) > 0 {
		return opts{}, fmt.Errorf("unexpected argument %q", rest[0])
	}
	return o, nil
}

// Subcommands lists the subcommands that Main runs.
var Subcommands = []string{"run", "exec-env", "bar", "list"}

// IsCLI reports whether args, the command line without the program name, is
// for Main rather than for the desktop application: it starts with a
// subcommand or with an option.
func IsCLI(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return slices.Contains(Subcommands, args[0]) || strings.HasPrefix(args[0], "-")
}

// Main runs the command line and returns the process exit code.
func Main(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	return run(args, stdout, stderr, getenv, systemDeps(getenv))
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string, d deps) int {
	if len(args) > 0 {
		switch args[0] {
		case "run":
			return report(stderr, d.run(args[1:]))
		case "exec-env":
			return report(stderr, runExecEnv(args[1:], d))
		}
	}
	o, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "claude-tower: %v\n%s\n", err, usage)
		return 2
	}
	home := homeDir(getenv)
	dir := configDir(o.dir, home, getenv)
	switch o.cmd {
	case "bar":
		runBar(dir, o.maxWidth, stdout, stderr)
		return 0
	case "list":
		return runList(dir, home, d, stdout, stderr)
	case "":
		fmt.Fprintf(stderr, "claude-tower: missing subcommand\n%s\n", usage)
		return 2
	}
	fmt.Fprintf(stderr, "claude-tower: unknown subcommand %q\n%s\n", o.cmd, usage)
	return 2
}

func report(stderr io.Writer, err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintf(stderr, "claude-tower: %v\n", err)
	return 1
}

// runExecEnv implements `exec-env <file> -- <command> [args...]`: it loads the
// environment file, deletes it, and runs the command with that environment
// plus the terminal variables of the current process.
func runExecEnv(args []string, d deps) error {
	file, command, err := hosting.ExecEnvArgs(args)
	if err != nil {
		return err
	}
	fileEnv, err := hosting.ReadEnvFile(file)
	if err != nil {
		return err
	}
	path := command[0]
	if !strings.Contains(path, "/") {
		if path, err = exec.LookPath(path); err != nil {
			return err
		}
	}
	return d.exec(path, command, hosting.ExecEnv(fileEnv, d.environ()))
}

func homeDir(getenv func(string) string) string {
	if h := getenv("HOME"); h != "" {
		return h
	}
	h, _ := os.UserHomeDir()
	return h
}

func configDir(flagDir, home string, getenv func(string) string) string {
	if flagDir != "" {
		return flagDir
	}
	if d := getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return d
	}
	return filepath.Join(home, ".claude")
}

// runBar never fails: errors go to stderr so that the tmux status line stays clean.
func runBar(dir string, maxWidth int, stdout, stderr io.Writer) {
	sessions, err := registry.Reader{Dir: dir}.Sessions()
	if err != nil {
		fmt.Fprintf(stderr, "claude-tower: %v\n", err)
		return
	}
	finder := transcript.NewFinder(dir)
	for i, s := range sessions {
		if s.State == registry.StateBusy {
			sessions[i].TurnEnded = finder.Last(s.SessionID).TurnEnded
		}
	}
	fmt.Fprintln(stdout, view.Bar(sessions, now(), maxWidth))
}

func runList(dir, home string, d deps, stdout, stderr io.Writer) int {
	sessions, err := registry.Reader{Dir: dir}.Sessions()
	if err != nil {
		fmt.Fprintf(stderr, "claude-tower: %v\n", err)
		return 1
	}
	finder := transcript.NewFinder(dir)
	activity := make(map[int]string, len(sessions))
	for i, s := range sessions {
		act := finder.Last(s.SessionID)
		sessions[i].TurnEnded = act.TurnEnded
		activity[s.PID] = act.Text
	}
	locations := d.locations(context.Background(), sessions)
	t := now()
	for _, s := range view.SortByUrgency(sessions) {
		fmt.Fprintln(stdout, listLine(s, locations[s.PID], activity[s.PID], home, t))
	}
	return 0
}

func listLine(s registry.Session, location, activity, home string, t time.Time) string {
	fields := []string{
		view.Icon(view.DisplayOf(s)),
		view.DisplayName(s),
		view.Label(s),
		view.Duration(s.StatusSince, t),
		s.TmuxPane,
		location,
		view.ShortPath(s.Cwd, home),
		activity,
	}
	for i, f := range fields {
		if f == "" {
			fields[i] = "-"
		}
	}
	return strings.Join(fields, "  ")
}

// systemDeps wires the subcommands to the real system.
func systemDeps(getenv func(string) string) deps {
	return deps{
		locations: func(ctx context.Context, sessions []registry.Session) map[int]string {
			return systemLocations(ctx, sessions, getenv)
		},
		run: func(args []string) error {
			h, err := hosting.NewSystem(os.Environ())
			if err != nil {
				return err
			}
			return h.Run(args)
		},
		exec:    syscall.Exec,
		environ: os.Environ,
	}
}

// systemLocations looks for the sessions in the tmux servers of the user and
// in the private server.
func systemLocations(ctx context.Context, sessions []registry.Session, getenv func(string) string) map[int]string {
	bin, err := tmux.FindBinary(exec.LookPath)
	if err != nil {
		return unknownLocations(sessions)
	}
	userDir := tmux.SocketDir(getenv("TMUX_TMPDIR"), os.Getuid())
	var private string
	if h, err := hosting.NewSystem(os.Environ()); err == nil {
		userDir = h.UserSocketDir(os.Getuid())
		private, _ = h.PrivateSocketPath()
	}
	servers := tmux.CandidateServers(userDir, private, bin, tmux.DefaultRunner)
	return tmuxLocations(ctx, sessions, servers, private, locate.ReadTree, tmux.DefaultRunner)
}

func unknownLocations(sessions []registry.Session) map[int]string {
	out := make(map[int]string, len(sessions))
	for _, s := range sessions {
		out[s.PID] = locUnknown
	}
	return out
}

// tmuxLocations labels each session "tmux" or "privé" (its pane lives on the
// private server) when a pane hosts it, and "hors tmux" otherwise. Servers
// that fail are ignored. When the process tree cannot be read, every location
// is unknown.
func tmuxLocations(
	ctx context.Context,
	sessions []registry.Session,
	servers []tmux.Server,
	privateSocket string,
	readTree func(context.Context, tmux.Runner) (locate.Tree, error),
	run tmux.Runner,
) map[int]string {
	tree, err := readTree(ctx, run)
	if err != nil {
		return unknownLocations(sessions)
	}
	var panes []tmux.Pane
	for _, srv := range servers {
		if ps, err := srv.Panes(ctx); err == nil {
			panes = append(panes, ps...)
		}
	}
	matched := locate.Match(sessions, panes, tree)
	out := make(map[int]string, len(sessions))
	for _, s := range sessions {
		p, ok := matched[s.PID]
		switch {
		case !ok:
			out[s.PID] = locOutside
		case privateSocket != "" && p.Server.Socket == privateSocket:
			out[s.PID] = locPrivate
		default:
			out[s.PID] = locTmux
		}
	}
	return out
}

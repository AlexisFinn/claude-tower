// Command claude-tower shows the Claude Code sessions running on this machine.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"claude-tower/internal/registry"
	"claude-tower/internal/transcript"
	"claude-tower/internal/tui"
	"claude-tower/internal/view"
)

const (
	usage           = "usage: claude-tower [bar|list] [--dir <config dir>] [--max-width <n>] [--client <name>]"
	defaultMaxWidth = 100
)

// now is the clock used for durations; tests replace it.
var now = time.Now

// runTUI starts the terminal interface; tests replace it.
var runTUI = tui.Run

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

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
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
		return runList(dir, home, stdout, stderr)
	case "":
		if err := runTUI(dir, home, o.client); err != nil {
			fmt.Fprintf(stderr, "claude-tower: %v\n", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(stderr, "claude-tower: unknown subcommand %q\n%s\n", o.cmd, usage)
	return 2
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

func runList(dir, home string, stdout, stderr io.Writer) int {
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
	t := now()
	for _, s := range view.SortByUrgency(sessions) {
		fmt.Fprintln(stdout, listLine(s, activity[s.PID], home, t))
	}
	return 0
}

func listLine(s registry.Session, activity, home string, t time.Time) string {
	fields := []string{
		view.Icon(view.DisplayOf(s)),
		view.DisplayName(s),
		view.Label(s),
		view.Duration(s.StatusSince, t),
		s.TmuxPane,
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

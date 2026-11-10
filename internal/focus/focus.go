// Package focus brings the window of a terminal to the foreground: through
// the X11 window manager protocol, or through System Events on macOS.
package focus

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"claude-tower/internal/tmux"
)

// errUnsupported means that no window can be raised on this system, such as
// under Wayland.
var errUnsupported = errors.New("mise au premier plan non prise en charge sur ce système")

var run = tmux.ExecRunner(10 * time.Second)

// Supported reports whether Raise can work on this system.
func Supported() bool {
	return supportedOn(runtime.GOOS, os.Getenv("DISPLAY"), os.Getenv("WAYLAND_DISPLAY"))
}

func supportedOn(goos, display, wayland string) bool {
	switch goos {
	case "darwin":
		return true
	case "linux":
		return isX11(display, wayland)
	}
	return false
}

// isX11 reports whether the session is an X11 one: a display is set and no
// Wayland compositor is.
func isX11(display, wayland string) bool {
	return display != "" && wayland == ""
}

// Raise brings to the foreground the window of the first process of pids that
// owns one.
func Raise(pids []int) error {
	if !Supported() {
		return errUnsupported
	}
	if runtime.GOOS == "darwin" {
		return raiseMacOS(context.Background(), pids, run)
	}
	return raiseX11(pids)
}

// appProcess is a process whose executable lies in an application bundle.
type appProcess struct {
	pid    int
	bundle string // path of the outermost bundle, such as /Applications/iTerm.app
}

// appProcesses returns, in the order of pids, the processes whose executable
// lies in an application bundle. Several can belong to one terminal: a
// helper, such as the server of iTerm2, runs from the bundle of its
// application without owning any window.
func appProcesses(ctx context.Context, pids []int, run tmux.Runner) []appProcess {
	var apps []appProcess
	for _, pid := range pids {
		out, err := run(ctx, nil, "ps", "-o", "comm=", "-p", strconv.Itoa(pid))
		if err != nil {
			continue
		}
		comm := strings.TrimSpace(string(out))
		if i := strings.Index(comm, ".app/"); i >= 0 {
			apps = append(apps, appProcess{pid: pid, bundle: comm[:i+len(".app")]})
		}
	}
	return apps
}

// raiseScript brings to the foreground the first process of its arguments
// that System Events knows as an application process, and prints "raised";
// it prints "none" when it knows none of them.
var raiseScript = []string{
	"on run argv",
	`tell application "System Events"`,
	"repeat with p in argv",
	"set found to (every process whose unix id is ((contents of p) as integer))",
	"if found is not {} then",
	"set frontmost of (item 1 of found) to true",
	`return "raised"`,
	"end if",
	"end repeat",
	"end tell",
	`return "none"`,
	"end run",
}

func osascriptArgs(apps []appProcess) []string {
	args := make([]string, 0, 2*len(raiseScript)+len(apps))
	for _, line := range raiseScript {
		args = append(args, "-e", line)
	}
	for _, a := range apps {
		args = append(args, strconv.Itoa(a.pid))
	}
	return args
}

// raiseMacOS brings to the foreground the nearest process of pids that is an
// application process, and otherwise activates the application whose bundle
// holds the nearest of them.
func raiseMacOS(ctx context.Context, pids []int, run tmux.Runner) error {
	apps := appProcesses(ctx, pids, run)
	if len(apps) == 0 {
		return errors.New("aucune fenêtre de terminal trouvée pour cette session")
	}
	out, err := run(ctx, nil, "osascript", osascriptArgs(apps)...)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) == "raised" {
		return nil
	}
	_, err = run(ctx, nil, "open", "-a", apps[0].bundle)
	return err
}

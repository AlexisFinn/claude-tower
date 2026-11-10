// Package focus brings the window of a terminal to the foreground: through
// the X11 window manager protocol, or through System Events on macOS.
package focus

import (
	"context"
	"errors"
	"fmt"
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

// firstAppPID returns the first pid whose executable lies in an application
// bundle, which is what owns windows on macOS.
func firstAppPID(ctx context.Context, pids []int, run tmux.Runner) (int, bool) {
	for _, pid := range pids {
		out, err := run(ctx, nil, "ps", "-o", "comm=", "-p", strconv.Itoa(pid))
		if err == nil && strings.Contains(string(out), ".app/") {
			return pid, true
		}
	}
	return 0, false
}

func osascriptArgs(pid int) []string {
	return []string{"-e", fmt.Sprintf(
		`tell application "System Events" to set frontmost of (first process whose unix id is %d) to true`, pid)}
}

func raiseMacOS(ctx context.Context, pids []int, run tmux.Runner) error {
	pid, ok := firstAppPID(ctx, pids, run)
	if !ok {
		return errors.New("aucune fenêtre de terminal trouvée pour cette session")
	}
	_, err := run(ctx, nil, "osascript", osascriptArgs(pid)...)
	return err
}

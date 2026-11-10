package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// appBundle returns the application bundle that holds the executable exe, ""
// when exe lies outside one.
func appBundle(exe string) string {
	i := strings.Index(exe, ".app/Contents/MacOS/")
	if i < 0 {
		return ""
	}
	return exe[:i+len(".app")]
}

// bundleToOpen returns the bundle to start through LaunchServices instead of
// running the window in this process, "" to run it here. On macOS, a window
// started by a shell rather than by launchd can leave its sheets, such as the
// folder dialog, unanswered; started with open, it gets the activation of a
// regular application.
func bundleToOpen(goos, exe string, ppid int) string {
	if goos != "darwin" || ppid == 1 {
		return ""
	}
	return appBundle(exe)
}

// openBundle starts the application of bundle through LaunchServices, or
// brings it forward when it runs already, and reports whether it did.
func openBundle(bundle string) bool {
	return exec.Command("/usr/bin/open", bundle).Run() == nil
}

// executable returns the path of the running executable with its symbolic
// links resolved, such as the link that make install puts in ~/.local/bin.
func executable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

// Package tmuxctl moves a tmux client to the pane of a Claude Code session.
package tmuxctl

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

const maxAncestors = 32

var paneRe = regexp.MustCompile(`^%[0-9]+$`)

// runCmd runs a command without a shell and returns its standard output.
var runCmd = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// parentPID returns the parent process ID read from /proc.
var parentPID = procParentPID

// Jump switches client to the pane of the session whose process is
// sessionPID. An empty client, or an unexpanded tmux format, targets the
// current client. The pane must exist on this tmux server and must hold the
// session process, directly or as an ancestor of it.
func Jump(client, pane string, sessionPID int) error {
	if !paneRe.MatchString(pane) {
		return errors.New("session hors tmux")
	}
	if strings.HasPrefix(client, "#{") {
		client = ""
	}
	out, err := runCmd("tmux", "display-message", "-p", "-t", pane, "#{pane_pid}")
	if err != nil {
		return errors.New("volet introuvable sur ce serveur tmux")
	}
	panePID, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil || !hasAncestor(sessionPID, panePID) {
		return errors.New("volet introuvable sur ce serveur tmux")
	}
	args := []string{"switch-client"}
	if client != "" {
		args = append(args, "-c", client)
	}
	args = append(args, "-t", pane)
	if _, err := runCmd("tmux", args...); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if msg := strings.TrimSpace(string(exitErr.Stderr)); msg != "" {
				return fmt.Errorf("tmux switch-client: %s", msg)
			}
		}
		return fmt.Errorf("tmux switch-client: %w", err)
	}
	return nil
}

// hasAncestor reports whether target is pid or one of its ancestors.
func hasAncestor(pid, target int) bool {
	for range maxAncestors {
		if pid == target {
			return true
		}
		if pid <= 1 {
			return false
		}
		parent, err := parentPID(pid)
		if err != nil {
			return false
		}
		pid = parent
	}
	return false
}

func procParentPID(pid int) (int, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0, err
	}
	return parsePPID(string(data))
}

// parsePPID extracts the ppid, the field after the state, from a stat line
// whose command name may itself contain spaces and parentheses.
func parsePPID(stat string) (int, error) {
	i := strings.LastIndex(stat, ")")
	if i < 0 {
		return 0, errors.New("malformed stat line")
	}
	fields := strings.Fields(stat[i+1:])
	if len(fields) < 2 {
		return 0, errors.New("short stat line")
	}
	return strconv.Atoi(fields[1])
}

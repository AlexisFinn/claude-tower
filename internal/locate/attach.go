package locate

import (
	"bytes"
	"context"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"claude-tower/internal/tmux"
)

// Attachments maps the PID of each `claude attach <id>` client to the id it
// shows.
type Attachments map[int]string

// ReadAttachments lists the clients attached to background sessions of Claude
// Code. Linux reads /proc; other systems run ps through run.
func ReadAttachments(ctx context.Context, run tmux.Runner) (Attachments, error) {
	return readAttachments(ctx, run, runtime.GOOS, "/proc")
}

func readAttachments(ctx context.Context, run tmux.Runner, goos, procDir string) (Attachments, error) {
	if goos == "linux" {
		return readProcAttachments(procDir), nil
	}
	out, err := run(ctx, nil, "ps", "-A", "-o", "pid=,args=")
	if err != nil {
		return nil, err
	}
	return parsePSAttachments(string(out)), nil
}

// readProcAttachments reads the attach clients from the proc directory dir. An
// unreadable dir yields no attachment.
func readProcAttachments(dir string) Attachments {
	out := Attachments{}
	forEachProc(dir, "cmdline", func(pid int, data []byte) {
		if !bytes.Contains(data, []byte("attach")) {
			return
		}
		if id, ok := attachTarget(strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")); ok {
			out[pid] = id
		}
	})
	return out
}

// parsePSAttachments parses the output of `ps -A -o pid=,args=`. Arguments
// are split on spaces, which ps does not quote.
func parsePSAttachments(out string) Attachments {
	res := Attachments{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		if id, ok := attachTarget(f[1:]); ok {
			res[pid] = id
		}
	}
	return res
}

// attachTarget returns the session id of the command line of a claude
// attach client: a program whose name contains "claude", possibly run by
// node, bun or deno, options only, the word attach, then the id.
func attachTarget(argv []string) (string, bool) {
	if len(argv) < 3 {
		return "", false
	}
	// A script launcher runs the claude script given as its first argument,
	// named claude or installed in the claude-code package.
	name := strings.ToLower(filepath.Base(argv[0]))
	switch name {
	case "node", "bun", "deno":
		argv = argv[1:]
		name = strings.ToLower(filepath.Base(argv[0]))
		if strings.Contains(filepath.ToSlash(argv[0]), "/claude-code/") {
			name = "claude"
		}
	}
	if len(argv) < 3 || !strings.Contains(name, "claude") {
		return "", false
	}
	for i := 1; i < len(argv)-1; i++ {
		switch {
		case argv[i] == "attach":
			id := argv[i+1]
			if id == "" || strings.HasPrefix(id, "-") {
				return "", false
			}
			return id, true
		case !strings.HasPrefix(argv[i], "-"):
			return "", false
		}
	}
	return "", false
}

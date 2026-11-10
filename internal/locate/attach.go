package locate

import (
	"bytes"
	"context"
	"os"
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
	if runtime.GOOS == "linux" {
		return readProcAttachments("/proc")
	}
	out, err := run(ctx, nil, "ps", "-A", "-o", "pid=,args=")
	if err != nil {
		return nil, err
	}
	return parsePSAttachments(string(out)), nil
}

func readProcAttachments(dir string) (Attachments, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "[0-9]*", "cmdline"))
	if err != nil {
		return nil, err
	}
	out := Attachments{}
	for _, path := range paths {
		pid, err := strconv.Atoi(filepath.Base(filepath.Dir(path)))
		if err != nil {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil || !bytes.Contains(data, []byte("attach")) {
			continue
		}
		if id, ok := AttachTarget(strings.Split(strings.TrimRight(string(data), "\x00"), "\x00")); ok {
			out[pid] = id
		}
	}
	return out, nil
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
		if id, ok := AttachTarget(f[1:]); ok {
			res[pid] = id
		}
	}
	return res
}

// AttachTarget returns the session id of the command line of a claude
// attach client: a program whose name contains "claude", possibly run by
// node, bun or deno, options only, the word attach, then the id.
func AttachTarget(argv []string) (string, bool) {
	// A script launcher runs the claude script given as its first argument,
	// whose name is often cli.js in a claude directory: the whole path counts.
	name := filepath.Base(argv[0])
	if len(argv) > 1 {
		switch name {
		case "node", "bun", "deno":
			argv = argv[1:]
			name = argv[0]
		}
	}
	if len(argv) < 3 || !strings.Contains(strings.ToLower(name), "claude") {
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

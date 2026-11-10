// Package locate finds the tmux pane that hosts a Claude Code session by
// walking the process tree upwards from the session process.
package locate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"claude-tower/internal/registry"
	"claude-tower/internal/tmux"
)

// maxAncestors bounds every walk up the process tree.
const maxAncestors = 32

// Tree maps a process ID to its parent's process ID.
type Tree map[int]int

// ReadTree reads the process tree once. Linux reads /proc; other systems run
// ps through run.
func ReadTree(ctx context.Context, run tmux.Runner) (Tree, error) {
	return readTree(ctx, run, runtime.GOOS, "/proc")
}

func readTree(ctx context.Context, run tmux.Runner, goos, procDir string) (Tree, error) {
	if goos == "linux" {
		return readProc(procDir), nil
	}
	out, err := run(ctx, nil, "ps", "-A", "-o", "pid=,ppid=")
	if err != nil {
		return nil, err
	}
	return parsePS(string(out)), nil
}

// readProc reads the process tree from the proc directory dir. An unreadable
// dir yields an empty tree.
func readProc(dir string) Tree {
	tree := Tree{}
	forEachProc(dir, "stat", func(pid int, data []byte) {
		if ppid, err := parsePPID(string(data)); err == nil {
			tree[pid] = ppid
		}
	})
	return tree
}

// forEachProc calls fn with the PID and the content of the file named file
// of each process listed in the proc directory dir. A process whose file
// cannot be read, such as one that just exited, is skipped.
func forEachProc(dir, file string, fn func(pid int, data []byte)) {
	// A dir that cannot be listed is treated as having no process.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name(), file))
		if err != nil {
			continue
		}
		fn(pid, data)
	}
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

// parsePS parses the output of `ps -A -o pid=,ppid=`; malformed lines are
// skipped.
func parsePS(out string) Tree {
	tree := Tree{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 == nil && err2 == nil {
			tree[pid] = ppid
		}
	}
	return tree
}

// Ancestors returns pid followed by its ancestors, nearest first, stopping
// before PID 1, at an unknown process, on a cycle, or after 32 entries.
func (t Tree) Ancestors(pid int) []int {
	var chain []int
	seen := map[int]bool{}
	for len(chain) < maxAncestors && pid > 1 && !seen[pid] {
		chain = append(chain, pid)
		seen[pid] = true
		parent, ok := t[pid]
		if !ok {
			break
		}
		pid = parent
	}
	return chain
}

// Descendants returns every process below pid, in no particular order. pid
// itself is excluded.
func (t Tree) Descendants(pid int) []int {
	children := make(map[int][]int)
	for child, parent := range t {
		children[parent] = append(children[parent], child)
	}
	var out []int
	seen := map[int]bool{pid: true}
	stack := []int{pid}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, c := range children[cur] {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
				stack = append(stack, c)
			}
		}
	}
	return out
}

// Match attaches each session to the pane whose process is the session
// process or one of its ancestors. The result is keyed by session PID;
// sessions outside every pane are absent.
func Match(sessions []registry.Session, panes []tmux.Pane, tree Tree) map[int]tmux.Pane {
	byPID := make(map[int]tmux.Pane, len(panes))
	for _, p := range panes {
		byPID[p.PID] = p
	}
	out := make(map[int]tmux.Pane, len(sessions))
	for _, s := range sessions {
		for _, pid := range tree.Ancestors(s.PID) {
			if p, ok := byPID[pid]; ok {
				out[s.PID] = p
				break
			}
		}
	}
	return out
}

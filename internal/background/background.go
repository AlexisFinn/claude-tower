// Package background counts the background agents and shells of a Claude Code
// session that are still running. It reads the session transcript
// incrementally and checks the shells against the files that their processes
// hold open.
package background

import (
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"claude-tower/internal/transcript"
)

const (
	// Unknown is the count reported when it cannot be determined.
	Unknown = -1

	// maxAgentAge bounds how long an agent without a recorded end counts as
	// running.
	maxAgentAge = 6 * time.Hour

	// maxEnded bounds how many ended task ids are remembered.
	maxEnded = 4096
)

// Counts is the number of background agents and shells still running, or
// Unknown.
type Counts struct {
	Agents, Shells int
}

// FDReader lists the targets of the open file descriptors of a process. It
// returns nothing for a process it cannot inspect.
type FDReader func(pid int) []string

// DefaultFDReader returns the descriptor reader of the running system: Linux
// reads /proc, other systems have none and get nil.
func DefaultFDReader() FDReader {
	if runtime.GOOS == "linux" {
		return procFDs
	}
	return nil
}

func procFDs(pid int) []string {
	dir := "/proc/" + strconv.Itoa(pid) + "/fd"
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	targets := make([]string, 0, len(entries))
	for _, e := range entries {
		if target, err := os.Readlink(dir + "/" + e.Name()); err == nil {
			targets = append(targets, target)
		}
	}
	return targets
}

// Query describes the session whose background work is counted.
type Query struct {
	// StartedAt is the start of the process of the session: work launched
	// before it belongs to an earlier run of a resumed session. Zero accepts
	// every launch.
	StartedAt time.Time
	// Now is the current time.
	Now time.Time
	// Descendants returns the processes below the session process. It is
	// called only when a shell needs checking.
	Descendants func() []int
	// ReadFDs reads the descriptors of a process. Nil makes the count of
	// shells Unknown.
	ReadFDs FDReader
}

// Tracker follows the background tasks of one session. It is safe for
// concurrent use.
type Tracker struct {
	mu      sync.Mutex
	reader  transcript.Follower
	agents  map[string]time.Time
	shells  map[string]time.Time
	ended   map[string]struct{}
	endedQ  []string // ring of the ended ids
	endedAt int      // index of the oldest id of endedQ once it is full
}

// NewTracker returns an empty Tracker.
func NewTracker() *Tracker {
	return &Tracker{agents: map[string]time.Time{}, shells: map[string]time.Time{}, ended: map[string]struct{}{}}
}

func (t *Tracker) reset() {
	clear(t.agents)
	clear(t.shells)
	clear(t.ended)
	t.endedQ, t.endedAt = t.endedQ[:0], 0
}

// Update reads what the transcript at path gained since the last call. A
// transcript that shrank, was replaced by another file, or moved starts a
// new read from the beginning. A failed read keeps the state known so far.
func (t *Tracker) Update(transcriptPath string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.reader.Read(transcriptPath, t.reset, t.apply)
}

// apply updates the state with one transcript line.
func (t *Tracker) apply(line []byte) {
	for _, ev := range transcript.TaskEvents(line) {
		if ev.Launched {
			t.launch(ev)
		} else {
			t.end(ev.Kind, ev.ID)
		}
	}
}

// end forgets a task and remembers its id so that a launch recorded later in
// the file does not revive it. An event that does not say the kind of the task
// ends it whatever its kind.
func (t *Tracker) end(kind transcript.TaskKind, id string) {
	if kind != transcript.TaskShell {
		delete(t.agents, id)
	}
	if kind != transcript.TaskAgent {
		delete(t.shells, id)
	}
	t.remember(id)
}

// remember keeps the id of an ended task, dropping the oldest ids beyond
// maxEnded.
func (t *Tracker) remember(id string) {
	if _, ok := t.ended[id]; ok {
		return
	}
	if len(t.endedQ) < maxEnded {
		t.endedQ = append(t.endedQ, id)
	} else {
		delete(t.ended, t.endedQ[t.endedAt])
		t.endedQ[t.endedAt] = id
		t.endedAt = (t.endedAt + 1) % maxEnded
	}
	t.ended[id] = struct{}{}
}

// launch records the first launch of a task that has not ended.
func (t *Tracker) launch(ev transcript.TaskEvent) {
	if _, done := t.ended[ev.ID]; done {
		return
	}
	set := t.agents
	if ev.Kind == transcript.TaskShell {
		set = t.shells
	}
	if _, ok := set[ev.ID]; !ok {
		set[ev.ID] = ev.At
	}
}

// Counts returns the background work still running. The agents are the
// launched ones without an end that started after the session process and not
// more than six hours ago. The shells are the launched ones without a stop
// whose output file a descendant of the session process holds open; without
// a descriptor reader their count is Unknown when there are candidates.
func (t *Tracker) Counts(q Query) Counts {
	t.mu.Lock()
	agents := launchedAfter(t.agents, q.StartedAt, q.Now.Add(-maxAgentAge))
	shells := launchedAfter(t.shells, q.StartedAt, time.Time{})
	t.mu.Unlock()
	c := Counts{Agents: len(agents)}
	if len(shells) == 0 {
		return c
	}
	if q.ReadFDs == nil || q.Descendants == nil {
		c.Shells = Unknown
		return c
	}
	open := openOutputs(q.Descendants(), q.ReadFDs)
	for _, id := range shells {
		if open[id] {
			c.Shells++
		}
	}
	return c
}

// launchedAfter lists the ids launched after startedAt and after notBefore;
// a zero bound is ignored. A launch without a readable time never qualifies.
func launchedAfter(set map[string]time.Time, startedAt, notBefore time.Time) []string {
	var ids []string
	for id, ts := range set {
		if ts.IsZero() || (!startedAt.IsZero() && !ts.After(startedAt)) || (!notBefore.IsZero() && !ts.After(notBefore)) {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// openOutputs returns the ids of the task output files that the processes
// hold open.
func openOutputs(pids []int, read FDReader) map[string]bool {
	open := map[string]bool{}
	for _, pid := range pids {
		for _, target := range read(pid) {
			if id, ok := outputID(target); ok {
				open[id] = true
			}
		}
	}
	return open
}

// outputID extracts the task id from the target of a descriptor that points
// to .../tasks/<id>.output.
func outputID(target string) (string, bool) {
	base := path.Base(target)
	id, ok := strings.CutSuffix(base, ".output")
	if !ok || id == "" || path.Base(path.Dir(target)) != "tasks" {
		return "", false
	}
	return id, true
}

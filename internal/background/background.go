// Package background counts the background agents and shells of a Claude Code
// session that are still running. It reads the session transcript
// incrementally and checks the shells against the files that their processes
// hold open.
package background

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// Unknown is the count reported when it cannot be determined.
	Unknown = -1

	// maxAgentAge bounds how long an agent without a recorded end counts as
	// running.
	maxAgentAge = 6 * time.Hour

	readChunk = 256 << 10

	// maxEnded bounds how many ended task ids are remembered.
	maxEnded = 4096
)

var taskIDRe = regexp.MustCompile(`<task-id>\s*([^<\s]+)\s*</task-id>`)

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
	path    string
	info    os.FileInfo
	off     int64
	partial []byte
	buf     []byte
	agents  map[string]time.Time
	shells  map[string]time.Time
	ended   map[string]struct{}
	endedQ  []string // ring of the ended ids, in order of arrival once full
	endedAt int      // index of the oldest id of endedQ once it is full
}

// NewTracker returns an empty Tracker.
func NewTracker() *Tracker {
	return &Tracker{agents: map[string]time.Time{}, shells: map[string]time.Time{}, ended: map[string]struct{}{}}
}

func (t *Tracker) reset() {
	t.info, t.off, t.partial = nil, 0, nil
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
	fh, err := os.Open(transcriptPath)
	if err != nil {
		return err
	}
	defer fh.Close()
	info, err := fh.Stat()
	if err != nil {
		return err
	}
	if t.path != transcriptPath || (t.info != nil && !os.SameFile(t.info, info)) || info.Size() < t.off {
		t.reset()
	}
	t.path, t.info = transcriptPath, info
	if info.Size() == t.off {
		return nil
	}
	if _, err := fh.Seek(t.off, io.SeekStart); err != nil {
		return err
	}
	return t.consume(fh)
}

func (t *Tracker) consume(r io.Reader) error {
	if t.buf == nil {
		t.buf = make([]byte, readChunk)
	}
	for {
		n, err := r.Read(t.buf)
		if n > 0 {
			t.off += int64(n)
			t.feed(t.buf[:n])
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// feed processes the complete lines of data, after the partial line kept from
// the previous read, and keeps the trailing incomplete line.
func (t *Tracker) feed(data []byte) {
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			t.partial = append(t.partial, data...)
			return
		}
		line := data[:i]
		if len(t.partial) > 0 {
			line = append(t.partial, line...)
			t.partial = nil
		}
		t.apply(line)
		data = data[i+1:]
	}
}

type toolResult struct {
	Status           string `json:"status"`
	AgentID          string `json:"agentId"`
	TaskID           string `json:"task_id"`
	TaskType         string `json:"task_type"`
	BackgroundTaskID string `json:"backgroundTaskId"`
}

type record struct {
	Type          string          `json:"type"`
	Operation     string          `json:"operation"`
	Content       string          `json:"content"`
	Timestamp     string          `json:"timestamp"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
}

var markers = [][]byte{
	[]byte(`"async_launched"`),
	[]byte(`"backgroundTaskId"`),
	[]byte(`"task_type"`),
	[]byte(`"queue-operation"`),
}

// mayCarryTask reports whether a line contains one of the strings that every
// launch or end record holds.
func mayCarryTask(line []byte) bool {
	for _, m := range markers {
		if bytes.Contains(line, m) {
			return true
		}
	}
	return false
}

// apply updates the state with one transcript line. Lines that cannot carry a
// launch or an end are skipped without being decoded.
func (t *Tracker) apply(line []byte) {
	if !mayCarryTask(line) {
		return
	}
	var rec record
	var typeErr *json.UnmarshalTypeError
	if err := json.Unmarshal(line, &rec); err != nil && !errors.As(err, &typeErr) {
		return
	}
	switch {
	case rec.Type == "queue-operation" && rec.Operation == "enqueue":
		t.applyNotification(rec.Content)
	case len(rec.ToolUseResult) > 0 && rec.ToolUseResult[0] == '{':
		var res toolResult
		if err := json.Unmarshal(rec.ToolUseResult, &res); err != nil && !errors.As(err, &typeErr) {
			return
		}
		t.applyResult(res, parseTime(rec.Timestamp))
	}
}

// applyNotification ends the tasks reported by the task notifications of a
// queued message.
func (t *Tracker) applyNotification(content string) {
	for _, block := range strings.Split(content, "<task-notification>")[1:] {
		if !strings.Contains(block, "<status>") {
			continue
		}
		if m := taskIDRe.FindStringSubmatch(block); m != nil {
			t.end(m[1])
		}
	}
}

// end forgets a task and remembers its id so that a launch recorded later in
// the file does not revive it.
func (t *Tracker) end(id string) {
	delete(t.agents, id)
	delete(t.shells, id)
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

func (t *Tracker) applyResult(res toolResult, ts time.Time) {
	if res.TaskID != "" {
		switch res.TaskType {
		case "local_agent":
			delete(t.agents, res.TaskID)
			t.remember(res.TaskID)
		case "local_bash":
			delete(t.shells, res.TaskID)
			t.remember(res.TaskID)
		}
	}
	if res.Status == "async_launched" && res.AgentID != "" {
		t.launch(t.agents, res.AgentID, ts)
	}
	if res.BackgroundTaskID != "" {
		t.launch(t.shells, res.BackgroundTaskID, ts)
	}
}

// launch records the first launch of an id that has not ended.
func (t *Tracker) launch(set map[string]time.Time, id string, ts time.Time) {
	if _, done := t.ended[id]; done {
		return
	}
	if _, ok := set[id]; !ok {
		set[id] = ts
	}
}

func parseTime(s string) time.Time {
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return ts
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

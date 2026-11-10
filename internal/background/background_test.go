package background

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

var (
	t0  = time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	now = t0.Add(time.Hour)
)

func stamp(d time.Duration) string { return t0.Add(d).Format(time.RFC3339Nano) }

func launchAgent(id string, at time.Duration) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"toolUseResult":{"status":"async_launched","agentId":%q}}`, stamp(at), id)
}

func foregroundAgent(id string, at time.Duration) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"toolUseResult":{"status":"completed","agentId":%q}}`, stamp(at), id)
}

func launchShell(id string, at time.Duration) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"toolUseResult":{"backgroundTaskId":%q}}`, stamp(at), id)
}

func notification(id, status string) string {
	content := fmt.Sprintf("<task-notification>\\n<task-id>%s</task-id>\\n<tool-use-id>toolu_1</tool-use-id>\\n<status>%s</status>\\n</task-notification>", id, status)
	return fmt.Sprintf(`{"type":"queue-operation","operation":"enqueue","content":"%s"}`, content)
}

func stop(id, taskType string) string {
	return fmt.Sprintf(`{"type":"user","toolUseResult":{"task_id":%q,"task_type":%q}}`, id, taskType)
}

func write(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendTo(t *testing.T, path, data string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(data); err != nil {
		t.Fatal(err)
	}
}

func transcriptOf(t *testing.T, lines ...string) (string, *Tracker) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	write(t, path, lines...)
	tr := NewTracker()
	if err := tr.Update(path); err != nil {
		t.Fatal(err)
	}
	return path, tr
}

func agentsOf(tr *Tracker, startedAt time.Time) int {
	return tr.Counts(Query{StartedAt: startedAt, Now: now}).Agents
}

func ids(set map[string]time.Time) []string {
	var out []string
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func TestAgentLifecycle(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		want  int
	}{
		{"launched", []string{launchAgent("aaa", 0)}, 1},
		{"foreground agent is not counted", []string{foregroundAgent("aaa", 0)}, 0},
		{"ended by notification", []string{launchAgent("aaa", 0), notification("aaa", "completed")}, 0},
		{"any status ends", []string{launchAgent("aaa", 0), notification("aaa", "killed")}, 0},
		{"duplicate notification", []string{launchAgent("aaa", 0), launchAgent("bbb", 0), notification("aaa", "completed"), notification("aaa", "completed")}, 1},
		{"notification without status", []string{launchAgent("aaa", 0), `{"type":"queue-operation","operation":"enqueue","content":"<task-notification>\n<task-id>aaa</task-id>\n</task-notification>"}`}, 1},
		{"dequeue is not an end", []string{launchAgent("aaa", 0), `{"type":"queue-operation","operation":"dequeue","content":"<task-notification><task-id>aaa</task-id><status>completed</status></task-notification>"}`}, 1},
		{"attachment copy is not an end", []string{launchAgent("aaa", 0), `{"type":"attachment","attachment":{"content":"<task-notification><task-id>aaa</task-id><status>completed</status></task-notification>"}}`}, 1},
		{"stopped explicitly", []string{launchAgent("aaa", 0), stop("aaa", "local_agent")}, 0},
		{"stop of a shell leaves the agent", []string{launchAgent("aaa", 0), stop("aaa", "local_bash")}, 1},
		{"two agents, one ends", []string{launchAgent("aaa", 0), launchAgent("bbb", time.Second), notification("bbb", "completed")}, 1},
		{"tool result that is a string", []string{launchAgent("aaa", 0), `{"type":"user","toolUseResult":"Error: boom"}`}, 1},
		{"corrupt line", []string{launchAgent("aaa", 0), `{"toolUseResult": {oops`}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, tr := transcriptOf(t, c.lines...)
			if got := agentsOf(tr, time.Time{}); got != c.want {
				t.Errorf("agents=%d want %d", got, c.want)
			}
		})
	}
}

func TestAgentsBeforeStartAreIgnored(t *testing.T) {
	_, tr := transcriptOf(t, launchAgent("old", -time.Minute), launchAgent("new", time.Minute))
	if got := agentsOf(tr, t0); got != 1 {
		t.Errorf("agents=%d want 1", got)
	}
	if got := agentsOf(tr, time.Time{}); got != 2 {
		t.Errorf("without start: agents=%d want 2", got)
	}
}

func TestAgentTooOldIsIgnored(t *testing.T) {
	_, tr := transcriptOf(t, launchAgent("stale", -7*time.Hour), launchAgent("fresh", -5*time.Hour))
	q := Query{Now: now.Add(-time.Hour)}
	if got := tr.Counts(q).Agents; got != 1 {
		t.Errorf("agents=%d want 1 (the 7 h old one ignored)", got)
	}
}

func TestLaunchWithoutTimestampIsIgnored(t *testing.T) {
	_, tr := transcriptOf(t, `{"type":"user","toolUseResult":{"status":"async_launched","agentId":"aaa"}}`)
	if got := agentsOf(tr, time.Time{}); got != 0 {
		t.Errorf("agents=%d want 0", got)
	}
}

func fds(m map[int][]string) FDReader {
	return func(pid int) []string { return m[pid] }
}

func shellQuery(read FDReader, pids ...int) Query {
	return Query{Now: now, Descendants: func() []int { return pids }, ReadFDs: read}
}

func TestShellAliveFollowsOpenDescriptors(t *testing.T) {
	_, tr := transcriptOf(t, launchShell("bone", 0), launchShell("btwo", 0), launchShell("bthree", 0))
	read := fds(map[int][]string{
		10: {"/dev/null", "/tmp/claude/-slug/s/tasks/bone.output"},
		11: {"pipe:[123]", "/tmp/claude/tasks/btwo.output (deleted)"},
		12: {"/tmp/other/bthree.output", "/tmp/x/tasks/bthreeX.output"},
	})
	if got := tr.Counts(shellQuery(read, 10, 11, 12)).Shells; got != 1 {
		t.Errorf("shells=%d want 1", got)
	}
	if got := tr.Counts(shellQuery(read)).Shells; got != 0 {
		t.Errorf("no descendants: shells=%d want 0", got)
	}
}

func TestShellStoppedOrBeforeStartIsNotCounted(t *testing.T) {
	_, tr := transcriptOf(t, launchShell("bstop", time.Minute), stop("bstop", "local_bash"), launchShell("bold", -time.Minute), launchShell("blive", time.Minute))
	read := fds(map[int][]string{10: {"/t/tasks/bstop.output", "/t/tasks/bold.output", "/t/tasks/blive.output"}})
	q := shellQuery(read, 10)
	q.StartedAt = t0
	if got := tr.Counts(q).Shells; got != 1 {
		t.Errorf("shells=%d want 1", got)
	}
}

func TestShellWithoutTimestampIsNeverCounted(t *testing.T) {
	_, tr := transcriptOf(t, `{"type":"user","toolUseResult":{"backgroundTaskId":"bnots"}}`)
	read := fds(map[int][]string{10: {"/t/tasks/bnots.output"}})
	if got := tr.Counts(shellQuery(read, 10)).Shells; got != 0 {
		t.Errorf("shells=%d want 0", got)
	}
}

func TestShellOfAnyAgeCounts(t *testing.T) {
	_, tr := transcriptOf(t, launchShell("bold", -48*time.Hour))
	read := fds(map[int][]string{10: {"/t/tasks/bold.output"}})
	if got := tr.Counts(shellQuery(read, 10)).Shells; got != 1 {
		t.Errorf("shells=%d want 1", got)
	}
}

func TestShellsUnknownWithoutDescriptorReader(t *testing.T) {
	_, tr := transcriptOf(t, launchAgent("aaa", 0), launchShell("bone", 0))
	got := tr.Counts(Query{Now: now, Descendants: func() []int { return nil }})
	if got.Shells != Unknown || got.Agents != 1 {
		t.Errorf("counts=%+v", got)
	}
}

func TestShellsWithoutCandidatesAreZeroWithoutDescriptorReader(t *testing.T) {
	_, tr := transcriptOf(t, launchAgent("aaa", time.Minute), launchShell("bold", -time.Hour))
	q := Query{StartedAt: t0, Now: now, Descendants: func() []int { return nil }}
	if got := tr.Counts(q); got.Shells != 0 || got.Agents != 1 {
		t.Errorf("counts=%+v", got)
	}
}

func TestDescendantsReadOnlyWhenShellsNeedChecking(t *testing.T) {
	_, tr := transcriptOf(t, launchAgent("aaa", 0), launchShell("bold", -time.Hour))
	called := 0
	q := Query{
		StartedAt: t0, Now: now,
		Descendants: func() []int { called++; return nil },
		ReadFDs:     fds(nil),
	}
	if got := tr.Counts(q); got.Shells != 0 || called != 0 {
		t.Errorf("counts=%+v called=%d", got, called)
	}
}

func TestIncrementalRead(t *testing.T) {
	path, tr := transcriptOf(t, launchAgent("aaa", 0))
	appendTo(t, path, launchAgent("bbb", time.Second)+"\n")
	if err := tr.Update(path); err != nil {
		t.Fatal(err)
	}
	if got := agentsOf(tr, time.Time{}); got != 2 {
		t.Fatalf("agents=%d want 2", got)
	}
	appendTo(t, path, notification("aaa", "completed")+"\n")
	if err := tr.Update(path); err != nil {
		t.Fatal(err)
	}
	if got := ids(tr.agents); !reflect.DeepEqual(got, []string{"bbb"}) {
		t.Errorf("agents=%v", got)
	}
}

func TestIncrementalReadDoesNotReprocess(t *testing.T) {
	path, tr := transcriptOf(t, launchAgent("aaa", 0), notification("aaa", "completed"))
	// A launch relisted after its end would revive the agent if the file were read again.
	tr.agents["aaa"] = t0
	if err := tr.Update(path); err != nil {
		t.Fatal(err)
	}
	tr.mu.Lock()
	off := tr.off
	tr.mu.Unlock()
	fi, _ := os.Stat(path)
	if off != fi.Size() {
		t.Errorf("offset=%d size=%d", off, fi.Size())
	}
	if got := ids(tr.agents); !reflect.DeepEqual(got, []string{"aaa"}) {
		t.Errorf("the unchanged file was read again: %v", got)
	}
}

func TestPartialTrailingLineWaitsForItsEnd(t *testing.T) {
	path, tr := transcriptOf(t)
	line := launchAgent("aaa", 0)
	half := len(line) / 2
	appendTo(t, path, line[:half])
	if err := tr.Update(path); err != nil {
		t.Fatal(err)
	}
	if got := agentsOf(tr, time.Time{}); got != 0 {
		t.Fatalf("partial line counted: %d", got)
	}
	appendTo(t, path, line[half:])
	if err := tr.Update(path); err != nil {
		t.Fatal(err)
	}
	if got := agentsOf(tr, time.Time{}); got != 0 {
		t.Fatalf("line without its newline counted: %d", got)
	}
	appendTo(t, path, "\n")
	if err := tr.Update(path); err != nil {
		t.Fatal(err)
	}
	if got := agentsOf(tr, time.Time{}); got != 1 {
		t.Errorf("agents=%d want 1", got)
	}
}

func TestPartialLineFollowedByCompleteLineInTheSameRead(t *testing.T) {
	path, tr := transcriptOf(t)
	line := launchAgent("aaa", 0)
	half := len(line) / 2
	appendTo(t, path, line[:half])
	if err := tr.Update(path); err != nil {
		t.Fatal(err)
	}
	appendTo(t, path, line[half:]+"\n"+launchAgent("bbb", time.Second)+"\n")
	if err := tr.Update(path); err != nil {
		t.Fatal(err)
	}
	if got := ids(tr.agents); !reflect.DeepEqual(got, []string{"aaa", "bbb"}) {
		t.Errorf("agents=%v", got)
	}
}

func TestFirstLaunchTimeIsKept(t *testing.T) {
	_, tr := transcriptOf(t, launchAgent("aaa", 0), launchAgent("aaa", 30*time.Minute), launchShell("bone", 0), launchShell("bone", 30*time.Minute))
	if got := tr.agents["aaa"]; !got.Equal(t0) {
		t.Errorf("agent time=%v want %v", got, t0)
	}
	if got := tr.shells["bone"]; !got.Equal(t0) {
		t.Errorf("shell time=%v want %v", got, t0)
	}
}

func TestEnqueueOperationOfAnotherRecordTypeIsNotAnEnd(t *testing.T) {
	line := `{"type":"attachment","operation":"enqueue","source":"queue-operation","content":"<task-notification><task-id>aaa</task-id><status>completed</status></task-notification>"}`
	_, tr := transcriptOf(t, launchAgent("aaa", 0), line)
	if got := agentsOf(tr, time.Time{}); got != 1 {
		t.Errorf("agents=%d want 1", got)
	}
}

func TestEndBeforeLaunchIsNotRevived(t *testing.T) {
	_, tr := transcriptOf(t, notification("aaa", "completed"), launchAgent("aaa", 0), stop("bone", "local_bash"), launchShell("bone", 0), launchAgent("bbb", 0))
	if got := ids(tr.agents); !reflect.DeepEqual(got, []string{"bbb"}) {
		t.Errorf("agents=%v", got)
	}
	if len(tr.shells) != 0 {
		t.Errorf("shells=%v", tr.shells)
	}
}

func TestEndedIdsAreBounded(t *testing.T) {
	tr := NewTracker()
	for i := 0; i < maxEnded+10; i++ {
		tr.end(fmt.Sprintf("id%d", i))
	}
	if len(tr.ended) != maxEnded || len(tr.endedQ) != maxEnded {
		t.Fatalf("ended=%d queue=%d want %d", len(tr.ended), len(tr.endedQ), maxEnded)
	}
	if _, ok := tr.ended["id0"]; ok {
		t.Error("the oldest id is still remembered")
	}
	if _, ok := tr.ended[fmt.Sprintf("id%d", maxEnded+9)]; !ok {
		t.Error("the newest id is forgotten")
	}
	tr.apply([]byte(launchAgent("id0", 0)))
	tr.apply([]byte(launchAgent(fmt.Sprintf("id%d", maxEnded+9), 0)))
	if got := ids(tr.agents); !reflect.DeepEqual(got, []string{"id0"}) {
		t.Errorf("agents=%v", got)
	}
}

func TestEndedIdsAreForgottenOldestFirstAfterWrapAround(t *testing.T) {
	tr := NewTracker()
	for i := 0; i < 2*maxEnded+5; i++ {
		tr.end(fmt.Sprintf("id%d", i))
	}
	for i := 0; i < maxEnded+5; i++ {
		if _, ok := tr.ended[fmt.Sprintf("id%d", i)]; ok {
			t.Fatalf("id%d is still remembered", i)
		}
	}
	for i := maxEnded + 5; i < 2*maxEnded+5; i++ {
		if _, ok := tr.ended[fmt.Sprintf("id%d", i)]; !ok {
			t.Fatalf("id%d is forgotten", i)
		}
	}
	tr.reset()
	tr.end("a")
	tr.end("b")
	if len(tr.ended) != 2 || len(tr.endedQ) != 2 || tr.endedAt != 0 {
		t.Errorf("after reset: ended=%d queue=%d at=%d", len(tr.ended), len(tr.endedQ), tr.endedAt)
	}
}

func TestReadBufferIsAllocatedOnDemandAndKept(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	tr := NewTracker()
	if err := tr.Update(path); err != nil {
		t.Fatal(err)
	}
	if tr.buf != nil {
		t.Fatal("buffer allocated for an empty transcript")
	}
	appendTo(t, path, launchAgent("aaa", 0)+"\n")
	if err := tr.Update(path); err != nil {
		t.Fatal(err)
	}
	if tr.buf == nil {
		t.Fatal("buffer not allocated")
	}
	first := &tr.buf[0]
	appendTo(t, path, launchAgent("bbb", 0)+"\n")
	if err := tr.Update(path); err != nil {
		t.Fatal(err)
	}
	if &tr.buf[0] != first {
		t.Error("buffer reallocated")
	}
}

func TestLineLargerThanTheReadChunk(t *testing.T) {
	big := fmt.Sprintf(`{"type":"user","timestamp":%q,"note":%q,"toolUseResult":{"status":"async_launched","agentId":"aaa"}}`,
		stamp(0), strings.Repeat("x", 3*readChunk))
	_, tr := transcriptOf(t, big)
	if got := agentsOf(tr, time.Time{}); got != 1 {
		t.Errorf("agents=%d want 1", got)
	}
}

func TestTruncatedThenRewrittenFileStartsOver(t *testing.T) {
	path, tr := transcriptOf(t, launchAgent("aaa", 0), launchAgent("bbb", 0), launchAgent("ccc", 0))
	write(t, path, launchAgent("ddd", 0))
	if err := tr.Update(path); err != nil {
		t.Fatal(err)
	}
	if got := ids(tr.agents); !reflect.DeepEqual(got, []string{"ddd"}) {
		t.Errorf("agents=%v", got)
	}
}

func TestReplacedFileStartsOver(t *testing.T) {
	path, tr := transcriptOf(t, launchAgent("aaa", 0))
	other := filepath.Join(filepath.Dir(path), "other")
	write(t, other, launchAgent("bbb", 0), launchAgent("ccc", 0), launchAgent("ddd", 0))
	if err := os.Rename(other, path); err != nil {
		t.Fatal(err)
	}
	if err := tr.Update(path); err != nil {
		t.Fatal(err)
	}
	if got := ids(tr.agents); !reflect.DeepEqual(got, []string{"bbb", "ccc", "ddd"}) {
		t.Errorf("agents=%v", got)
	}
}

func TestOtherPathStartsOver(t *testing.T) {
	path, tr := transcriptOf(t, launchAgent("aaa", 0))
	other := filepath.Join(filepath.Dir(path), "other.jsonl")
	write(t, other, launchAgent("bbb", 0), launchAgent("ccc", 0))
	if err := tr.Update(other); err != nil {
		t.Fatal(err)
	}
	if got := ids(tr.agents); !reflect.DeepEqual(got, []string{"bbb", "ccc"}) {
		t.Errorf("agents=%v", got)
	}
}

func TestMissingFileKeepsState(t *testing.T) {
	path, tr := transcriptOf(t, launchAgent("aaa", 0))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := tr.Update(path); err == nil {
		t.Error("expected an error")
	}
	if got := agentsOf(tr, time.Time{}); got != 1 {
		t.Errorf("agents=%d want 1", got)
	}
}

func TestPrefilterSkipsDecoding(t *testing.T) {
	skipped := []string{
		`{"type":"user","message":"hello"}`,
		`{"type":"user","toolUseResult":{"agentId":"aaa","status":"completed"}}`,
		`{"type":"assistant","content":"background"}`,
	}
	for _, l := range skipped {
		if mayCarryTask([]byte(l)) {
			t.Errorf("decoded: %s", l)
		}
	}
	decoded := []string{launchAgent("aaa", 0), launchShell("bone", 0), stop("aaa", "local_agent"), notification("aaa", "completed")}
	for _, l := range decoded {
		if !mayCarryTask([]byte(l)) {
			t.Errorf("skipped: %s", l)
		}
	}
	tr := NewTracker()
	tr.apply([]byte(launchAgent("aaa", 0)))
	if len(tr.agents) != 1 {
		t.Errorf("agents=%v", tr.agents)
	}
}

func TestOutputID(t *testing.T) {
	cases := []struct {
		target string
		id     string
		ok     bool
	}{
		{"/tmp/claude-1000/-slug/sess/tasks/bab12.output", "bab12", true},
		{"/tmp/x/tasks/a0123456789abcdef.output", "a0123456789abcdef", true},
		{"/tmp/x/tasks/.output", "", false},
		{"/tmp/x/other/bab12.output", "", false},
		{"/tmp/x/tasks/bab12.log", "", false},
		{"pipe:[1]", "", false},
	}
	for _, c := range cases {
		id, ok := outputID(c.target)
		if id != c.id || ok != c.ok {
			t.Errorf("%q: got %q,%v", c.target, id, ok)
		}
	}
}

func TestDefaultFDReader(t *testing.T) {
	r := DefaultFDReader()
	if r == nil {
		t.Skip("no /proc on this system")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "tasks", "bxyz.output")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(target)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if !openOutputs([]int{os.Getpid()}, r)["bxyz"] {
		t.Error("the output file held open by this process is not seen")
	}
	if fds := r(1 << 30); len(fds) != 0 {
		t.Errorf("unknown process: %v", fds)
	}
}

package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sid = "75f334f0-4dc9-41db-85e2-1d96e0dbaaf1"

func writeTranscript(t *testing.T, dir, slug, id string, lines ...string) string {
	t.Helper()
	p := filepath.Join(dir, "projects", slug, id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func last(t *testing.T, lines ...string) Activity {
	t.Helper()
	dir := t.TempDir()
	writeTranscript(t, dir, "p", sid, lines...)
	return NewFinder(dir).Last(sid)
}

func asst(blocks string) string {
	return `{"type":"assistant","message":{"content":[` + blocks + `]}}`
}

func TestToolUse(t *testing.T) {
	cases := []struct{ name, line, want string }{
		{"bash", asst(`{"type":"tool_use","name":"Bash","input":{"command":"go  test\n./..."}}`), "Bash: go test ./..."},
		{"read", asst(`{"type":"tool_use","name":"Read","input":{"file_path":"/a/b.go"}}`), "Read: /a/b.go"},
		{"grep", asst(`{"type":"tool_use","name":"Grep","input":{"pattern":"foo"}}`), "Grep: foo"},
		{"agent", asst(`{"type":"tool_use","name":"Agent","input":{"description":"explore"}}`), "Agent: explore"},
		{"unknown tool", asst(`{"type":"tool_use","name":"Weird","input":{"command":"x"}}`), "Weird"},
		{"missing arg", asst(`{"type":"tool_use","name":"Bash","input":{}}`), "Bash"},
		{"last tool_use wins", asst(`{"type":"tool_use","name":"Read","input":{"file_path":"/1"}},{"type":"text","text":"hi"},{"type":"tool_use","name":"Edit","input":{"file_path":"/2"}}`), "Edit: /2"},
		{"number value", asst(`{"type":"tool_use","name":"Bash","input":{"command":5}}`), "Bash"},
		{"object value", asst(`{"type":"tool_use","name":"Read","input":{"file_path":{"a":1}}}`), "Read"},
		{"null value", asst(`{"type":"tool_use","name":"Grep","input":{"pattern":null}}`), "Grep"},
		{"other keys of wrong type", asst(`{"type":"tool_use","name":"Bash","input":{"timeout":"x","env":[1,{"a":2}],"command":"ls"}}`), "Bash: ls"},
		{"escaped string", asst(`{"type":"tool_use","name":"Bash","input":{"command":"echo \"a\"\u00e9"}}`), `Bash: echo "a"é`},
		{"input is not an object", asst(`{"type":"tool_use","name":"Bash","input":"ls"}`), ""},
	}
	for _, c := range cases {
		if got := last(t, c.line).Text; got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestAssistantText(t *testing.T) {
	got := last(t, asst(`{"type":"thinking","thinking":"hmm"},{"type":"text","text":"first"},{"type":"text","text":"\n\n  second   line \nthird"}`))
	if got.Text != "second line" {
		t.Errorf("got %q", got.Text)
	}
}

func TestUserPrompts(t *testing.T) {
	if got := last(t, `{"type":"user","message":{"content":"  fix the bug\nplease"}}`).Text; got != "› fix the bug" {
		t.Errorf("string prompt: %q", got)
	}
	arr := `{"type":"user","message":{"content":[{"type":"tool_result","content":"x"},{"type":"text","text":"do it"}]}}`
	if got := last(t, arr).Text; got != "› do it" {
		t.Errorf("array prompt: %q", got)
	}
}

func TestIgnoredRecords(t *testing.T) {
	keep := asst(`{"type":"text","text":"kept"}`)
	ignored := []string{
		`{"type":"user","isMeta":true,"message":{"content":"meta"}}`,
		`{"type":"user","isCompactSummary":true,"message":{"content":"summary"}}`,
		`{"type":"user","isVisibleInTranscriptOnly":true,"message":{"content":"summary"}}`,
		`{"type":"user","message":{"content":"<task-notification>x</task-notification>"}}`,
		`{"type":"user","message":{"content":"[Request interrupted by user]"}}`,
		`{"type":"user","message":{"content":"[Image: foo.png]"}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","content":"out"},{"type":"image"}]}}`,
		`{"type":"user","message":{"content":[{"type":"text","text":"<command-name>x"}]}}`,
		`{"type":"system","subtype":"info"}`,
		`{"type":"attachment"}`,
		`not json at all`,
		``,
	}
	for _, ig := range ignored {
		if got := last(t, keep, ig).Text; got != "kept" {
			t.Errorf("%q not ignored: got %q", ig, got)
		}
	}
}

func TestNothingRetained(t *testing.T) {
	if got := last(t, `{"type":"attachment"}`, `garbage`); got != (Activity{}) {
		t.Errorf("got %+v", got)
	}
}

func TestTurnEnded(t *testing.T) {
	turn := `{"type":"system","subtype":"turn_duration","durationMs":5}`
	a := asst(`{"type":"text","text":"done"}`)
	cases := []struct {
		name  string
		lines []string
		want  bool
	}{
		{"marker last", []string{a, turn}, true},
		{"marker then noise", []string{a, turn, `{"type":"attachment"}`, `{"type":"mode"}`, `bad`}, true},
		{"assistant after marker", []string{turn, a}, false},
		{"user after marker", []string{turn, `{"type":"user","message":{"content":"hi"}}`}, false},
		{"meta user after marker", []string{turn, `{"type":"user","isMeta":true,"message":{"content":"x"}}`}, false},
		{"other system record", []string{a, `{"type":"system","subtype":"info"}`}, false},
	}
	for _, c := range cases {
		if got := last(t, c.lines...).TurnEnded; got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
	got := last(t, a, turn)
	if got.Text != "done" || !got.TurnEnded {
		t.Errorf("got %+v", got)
	}
	if NewFinder(t.TempDir()).Last(sid).TurnEnded {
		t.Error("TurnEnded without transcript")
	}
}

func TestWindows(t *testing.T) {
	short := asst(`{"type":"text","text":"old"}`)
	long := asst(`{"type":"text","text":"` + strings.Repeat("x", 300) + `"}`)
	noise := `{"type":"attachment","pad":"` + strings.Repeat("y", 100) + `"}`

	run := func(windows []int64, lines ...string) (Activity, int) {
		dir := t.TempDir()
		writeTranscript(t, dir, "p", sid, lines...)
		f := NewFinder(dir)
		f.windows = windows
		calls := 0
		f.readAt = func(p string, off, n int64) ([]byte, error) {
			calls++
			return readAtFile(p, off, n)
		}
		return f.Last(sid), calls
	}

	newest := asst(`{"type":"text","text":"new"}`)

	// The window starts in the middle of an older line: the partial line does not hide the newest record.
	act, calls := run([]int64{int64(len(newest)) + 6}, short, newest)
	if act.Text != "new" || calls != 1 {
		t.Errorf("mid-line start: %+v calls=%d", act, calls)
	}

	// The window starts exactly at the beginning of the last line: it is kept.
	act, calls = run([]int64{int64(len(newest)) + 1}, short, newest)
	if act.Text != "new" || calls != 1 {
		t.Errorf("line-aligned start: %+v calls=%d", act, calls)
	}

	// The last record is longer than the first window.
	act, calls = run([]int64{50, 1 << 20}, short, long)
	if act.Text != strings.Repeat("x", 300) || calls != 2 {
		t.Errorf("widening: %d chars calls=%d", len(act.Text), calls)
	}

	// Nothing retained even in the largest window.
	act, calls = run([]int64{50, 100, 200}, short, noise, noise, noise)
	if act.Text != "" || calls != 3 {
		t.Errorf("nothing retained: %+v calls=%d", act, calls)
	}

	// A file shorter than the first window is read once.
	act, calls = run([]int64{1 << 20, 4 << 20}, noise)
	if act.Text != "" || calls != 1 {
		t.Errorf("short file: %+v calls=%d", act, calls)
	}
}

func TestTranscriptCreatedLater(t *testing.T) {
	dir := t.TempDir()
	f := NewFinder(dir)
	if got := f.Last(sid); got != (Activity{}) {
		t.Fatalf("got %+v", got)
	}
	writeTranscript(t, dir, "p", sid, asst(`{"type":"text","text":"hello"}`))
	if got := f.Last(sid).Text; got != "hello" {
		t.Errorf("got %q", got)
	}
}

func TestTranscriptMoved(t *testing.T) {
	dir := t.TempDir()
	p := writeTranscript(t, dir, "a", sid, asst(`{"type":"text","text":"first"}`))
	f := NewFinder(dir)
	if got := f.Last(sid).Text; got != "first" {
		t.Fatalf("got %q", got)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	writeTranscript(t, dir, "b", sid, asst(`{"type":"text","text":"second"}`))
	if got := f.Last(sid).Text; got != "second" {
		t.Errorf("got %q", got)
	}
}

func TestCache(t *testing.T) {
	dir := t.TempDir()
	p := writeTranscript(t, dir, "p", sid, asst(`{"type":"text","text":"aaaa"}`))
	f := NewFinder(dir)
	calls := 0
	f.readAt = func(path string, off, n int64) ([]byte, error) {
		calls++
		return readAtFile(path, off, n)
	}
	if f.Last(sid).Text != "aaaa" || f.Last(sid).Text != "aaaa" || calls != 1 {
		t.Fatalf("expected one read, got %d", calls)
	}
	writeTranscript(t, dir, "p", sid, asst(`{"type":"text","text":"bbbb"}`))
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(p, future, future); err != nil {
		t.Fatal(err)
	}
	if got := f.Last(sid).Text; got != "bbbb" || calls != 2 {
		t.Errorf("got %q calls=%d", got, calls)
	}
}

func TestInvalidSessionID(t *testing.T) {
	dir := t.TempDir()
	writeTranscript(t, dir, "p", "short", asst(`{"type":"text","text":"x"}`))
	f := NewFinder(dir)
	for _, id := range []string{"", "short", "../../etc/passwd", "zzzzzzzz-zzzz", strings.Repeat("a", 65), sid + "/x"} {
		if got := f.Last(id); got != (Activity{}) {
			t.Errorf("id %q: got %+v", id, got)
		}
	}
}

func TestConfigDirWithGlobCharacters(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cfg[1]*?")
	writeTranscript(t, dir, "p", sid, asst(`{"type":"text","text":"ok"}`))
	if got := NewFinder(dir).Last(sid).Text; got != "ok" {
		t.Errorf("got %q", got)
	}
}

func TestDropFirstLine(t *testing.T) {
	if got := dropFirstLine([]byte("partial")); got != nil {
		t.Errorf("no newline: got %q", got)
	}
	if got := string(dropFirstLine([]byte("partial\nkept\n"))); got != "kept\n" {
		t.Errorf("got %q", got)
	}
}

func TestPartialFirstLineIsDropped(t *testing.T) {
	ghost := `{"type":"user","message":{"content":"ghost"}}`
	older := "garbage" + ghost
	newest := `{"type":"attachment"}`
	dir := t.TempDir()
	writeTranscript(t, dir, "p", sid, older, newest)
	f := NewFinder(dir)
	// The first window starts right after "garbage", where the cut line happens
	// to be a valid retained record; it must still be dropped.
	f.windows = []int64{int64(len(ghost) + 1 + len(newest) + 1), 1 << 20}
	if got := f.Last(sid); got.Text != "" {
		t.Errorf("partial line retained: %+v", got)
	}
}

func TestRetain(t *testing.T) {
	other := "11111111-2222-3333-4444-555555555555"
	dir := t.TempDir()
	writeTranscript(t, dir, "p", sid, asst(`{"type":"text","text":"a"}`))
	writeTranscript(t, dir, "p", other, asst(`{"type":"text","text":"b"}`))
	f := NewFinder(dir)
	f.Last(sid)
	f.Last(other)
	f.Retain([]string{other})
	f.mu.Lock()
	_, hasSid := f.cache[sid]
	_, hasOther := f.cache[other]
	f.mu.Unlock()
	if hasSid || !hasOther {
		t.Errorf("hasSid=%v hasOther=%v", hasSid, hasOther)
	}
	if got := f.Last(sid).Text; got != "a" {
		t.Errorf("dropped entry not rescanned: %q", got)
	}
}

func TestPath(t *testing.T) {
	dir := t.TempDir()
	want := writeTranscript(t, dir, "p", sid, asst(`{"type":"text","text":"a"}`))
	f := NewFinder(dir)
	if got := f.Path(sid); got != want {
		t.Errorf("uncached: got %q want %q", got, want)
	}
	f.Last(sid)
	if got := f.Path(sid); got != want {
		t.Errorf("cached: got %q want %q", got, want)
	}
	if got := f.Path("11111111-2222-3333-4444-555555555555"); got != "" {
		t.Errorf("unknown session: %q", got)
	}
	if got := f.Path("../x"); got != "" {
		t.Errorf("invalid session ID: %q", got)
	}
}

func TestScanLinesFromTheEnd(t *testing.T) {
	first := `{"type":"user","message":{"content":"first"}}`
	second := `{"type":"user","message":{"content":"second"}}`
	cases := []struct {
		name string
		data string
		want string
	}{
		{"last line without newline", first + "\n" + second, "› second"},
		{"trailing blank lines", first + "\n" + second + "\n\n\n", "› second"},
		{"blank lines between", first + "\n\n\n" + second + "\nbad\n", "› second"},
		{"only the first line", first, "› first"},
		{"unretained last line", first + "\n" + `{"type":"attachment"}` + "\n", "› first"},
		{"CRLF", first + "\r\n" + second + "\r\n", "› second"},
		{"empty", "", ""},
		{"newlines only", "\n\n", ""},
	}
	for _, c := range cases {
		act, found := scanLines([]byte(c.data))
		if act.Text != c.want || found != (c.want != "") {
			t.Errorf("%s: got %q %v", c.name, act.Text, found)
		}
	}
}

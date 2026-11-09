package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeSession(t *testing.T, dir, name, content string) {
	t.Helper()
	sd := filepath.Join(dir, "sessions")
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sd, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func aliveAll(int, string) bool { return true }

func one(t *testing.T, content string) (Session, bool) {
	t.Helper()
	dir := t.TempDir()
	writeSession(t, dir, "100.json", content)
	ss, err := Reader{Dir: dir, Alive: aliveAll}.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(ss) == 0 {
		return Session{}, false
	}
	return ss[0], true
}

const full = `{"pid":100,"sessionId":"abc-123","cwd":"/home/x/proj","startedAt":1794216804002,
"procStart":"555","kind":"interactive","tmux":"0:@3.%7","name":"proj-a","status":"idle",
"updatedAt":1794222510000,"statusUpdatedAt":1794222511000}`

func TestValidSession(t *testing.T) {
	s, ok := one(t, full)
	if !ok {
		t.Fatal("session skipped")
	}
	want := Session{
		PID: 100, SessionID: "abc-123", Name: "proj-a", Cwd: "/home/x/proj", Kind: "interactive",
		State: StateIdle, RawStatus: "idle", StartedAt: time.UnixMilli(1794216804002),
		StatusSince: time.UnixMilli(1794222511000), TmuxPane: "%7",
	}
	if s != want {
		t.Errorf("got %+v\nwant %+v", s, want)
	}
}

func TestWaitingFor(t *testing.T) {
	s, _ := one(t, `{"pid":100,"status":"waiting","waitingFor":"dialog open"}`)
	if s.State != StateWaiting || s.WaitingFor != "dialog open" {
		t.Errorf("got %+v", s)
	}
	s, _ = one(t, `{"pid":100,"status":"waiting"}`)
	if s.State != StateWaiting || s.WaitingFor != "" {
		t.Errorf("got %+v", s)
	}
}

func TestStatusNormalization(t *testing.T) {
	cases := map[string]State{
		"busy": StateBusy, "shell": StateBusy, "idle": StateIdle,
		"waiting": StateWaiting, "weird": StateUnknown, "": StateUnknown,
	}
	for status, want := range cases {
		s, ok := one(t, fmt.Sprintf(`{"pid":100,"status":%q}`, status))
		if !ok || s.State != want || s.RawStatus != status {
			t.Errorf("status %q: got %+v, want state %v", status, s, want)
		}
	}
}

func TestZeroStateIsUnknown(t *testing.T) {
	var s Session
	if s.State != StateUnknown {
		t.Error("zero State is not StateUnknown")
	}
}

func TestInvalidJSONRetry(t *testing.T) {
	dir := t.TempDir()
	writeSession(t, dir, "100.json", "{}")
	calls := 0
	r := Reader{Dir: dir, Alive: aliveAll, ReadFile: func(string) ([]byte, error) {
		calls++
		if calls == 1 {
			return []byte(`{"pid":100,"sta`), nil
		}
		return []byte(`{"pid":100,"status":"busy"}`), nil
	}}
	ss, err := r.Sessions()
	if err != nil || len(ss) != 1 || ss[0].State != StateBusy || calls != 2 {
		t.Errorf("ss=%+v err=%v calls=%d", ss, err, calls)
	}
}

func TestInvalidJSONSkipped(t *testing.T) {
	dir := t.TempDir()
	writeSession(t, dir, "100.json", `{"pid":100,`)
	calls := 0
	r := Reader{Dir: dir, Alive: aliveAll, ReadFile: func(p string) ([]byte, error) {
		calls++
		return os.ReadFile(p)
	}}
	ss, err := r.Sessions()
	if err != nil || len(ss) != 0 || calls != 2 {
		t.Errorf("ss=%+v err=%v calls=%d", ss, err, calls)
	}
}

func TestOnlyPidJSONFilesRead(t *testing.T) {
	dir := t.TempDir()
	writeSession(t, dir, "100.json", `{"pid":100,"status":"idle"}`)
	writeSession(t, dir, "100.abcdef.key", `{"pid":100,"status":"busy"}`)
	writeSession(t, dir, "notes.json", `{"pid":5}`)
	var read []string
	r := Reader{Dir: dir, Alive: aliveAll, ReadFile: func(p string) ([]byte, error) {
		read = append(read, filepath.Base(p))
		return os.ReadFile(p)
	}}
	ss, err := r.Sessions()
	if err != nil || len(ss) != 1 || len(read) != 1 || read[0] != "100.json" {
		t.Errorf("ss=%+v err=%v read=%v", ss, err, read)
	}
}

func TestMissingSessionsDir(t *testing.T) {
	ss, err := Reader{Dir: t.TempDir()}.Sessions()
	if err != nil || len(ss) != 0 {
		t.Errorf("ss=%+v err=%v", ss, err)
	}
}

func TestDeadOrMismatchedProcess(t *testing.T) {
	dir := t.TempDir()
	writeSession(t, dir, "100.json", `{"pid":100,"procStart":"555"}`)
	writeSession(t, dir, "200.json", `{"pid":200,"procStart":"777"}`)
	var got []string
	r := Reader{Dir: dir, Alive: func(pid int, ps string) bool {
		got = append(got, fmt.Sprintf("%d/%s", pid, ps))
		return pid == 100 && ps == "555"
	}}
	ss, _ := r.Sessions()
	if len(ss) != 1 || ss[0].PID != 100 {
		t.Errorf("ss=%+v", ss)
	}
	if strings.Join(got, ",") != "100/555,200/777" {
		t.Errorf("Alive calls: %v", got)
	}
}

func TestKindAndSpareFiltering(t *testing.T) {
	for _, c := range []struct {
		body string
		keep bool
	}{
		{`{"pid":100,"kind":"daemon"}`, false},
		{`{"pid":100,"kind":"daemon-worker"}`, false},
		{`{"pid":100,"kind":"interactive","spare":true}`, false},
		{`{"pid":100,"kind":"interactive","spare":false}`, true},
		{`{"pid":100,"kind":"bg"}`, true},
		{`{"pid":100}`, true},
		{`{"pid":100,"kind":"other"}`, true},
		{`{"pid":100,"kind":"interactive","entrypoint":"sdk-cli"}`, false},
		{`{"pid":100,"kind":"interactive","entrypoint":"sdk-ts"}`, false},
		{`{"pid":100,"kind":"interactive","entrypoint":"cli"}`, true},
	} {
		if _, ok := one(t, c.body); ok != c.keep {
			t.Errorf("%s: kept=%v", c.body, ok)
		}
	}
}

func TestTmuxPane(t *testing.T) {
	cases := map[string]string{
		`"0:@0.%0"`: "%0", `"main:@12.%345"`: "%345",
		`"0:@0.pane"`: "", `"0:@0"`: "", `"0:@0.%"`: "", `"0:@0.%1x"`: "", `""`: "",
	}
	for tmux, want := range cases {
		s, _ := one(t, `{"pid":100,"tmux":`+tmux+`}`)
		if s.TmuxPane != want {
			t.Errorf("%s: got %q want %q", tmux, s.TmuxPane, want)
		}
	}
	if s, _ := one(t, `{"pid":100}`); s.TmuxPane != "" {
		t.Errorf("absent tmux: %q", s.TmuxPane)
	}
}

func TestStatusSinceFallback(t *testing.T) {
	s, _ := one(t, `{"pid":100,"updatedAt":1794222510000}`)
	if !s.StatusSince.Equal(time.UnixMilli(1794222510000)) {
		t.Errorf("updatedAt fallback: %v", s.StatusSince)
	}
	s, _ = one(t, `{"pid":100}`)
	if !s.StatusSince.IsZero() || !s.StartedAt.IsZero() {
		t.Errorf("absent timestamps: %+v", s)
	}
	s, _ = one(t, `{"pid":100,"startedAt":1794216804002.0}`)
	if !s.StartedAt.Equal(time.UnixMilli(1794216804002)) {
		t.Errorf("float timestamp: %v", s.StartedAt)
	}
}

func TestPidMismatch(t *testing.T) {
	if _, ok := one(t, `{"pid":101}`); ok {
		t.Error("mismatched pid kept")
	}
	if _, ok := one(t, `{"pid":0}`); !ok {
		t.Error("zero pid skipped")
	}
}

func TestProcStartAsNumber(t *testing.T) {
	dir := t.TempDir()
	writeSession(t, dir, "100.json", `{"pid":100,"procStart":136546471}`)
	var got string
	Reader{Dir: dir, Alive: func(_ int, ps string) bool { got = ps; return true }}.Sessions()
	if got != "136546471" {
		t.Errorf("procStart = %q", got)
	}
}

func TestUnexpectedFieldTypes(t *testing.T) {
	s, ok := one(t, `{"pid":100,"name":42,"cwd":["x"],"startedAt":"soon","status":{"a":1},
"statusUpdatedAt":true,"spare":"yes","waitingFor":7,"tmux":3,"procStart":{}}`)
	if !ok {
		t.Fatal("session skipped")
	}
	if s.Name != "" || s.Cwd != "" || !s.StartedAt.IsZero() || s.State != StateUnknown ||
		!s.StatusSince.IsZero() || s.WaitingFor != "" || s.TmuxPane != "" {
		t.Errorf("got %+v", s)
	}
}

func TestControlCharactersRemoved(t *testing.T) {
	long := strings.Repeat("é", 40)
	s, _ := one(t, `{"pid":100,"name":"a\u001b[31mb","cwd":"/x\u0007y","kind":"in\u0000t",
"status":"id\nle","waitingFor":"`+long+`"}`)
	if s.Name != "a[31mb" || s.Cwd != "/xy" || s.Kind != "int" || s.RawStatus != "idle" {
		t.Errorf("got %+v", s)
	}
	if s.WaitingFor != strings.Repeat("é", 32) {
		t.Errorf("WaitingFor = %q", s.WaitingFor)
	}
}

func TestParseStartTime(t *testing.T) {
	line := "824693 (my ) (cmd x) S 1 824693 824693 0 -1 4194560 100 0 0 0 5 6 0 0 20 0 11 0 136546471 1000 200 18446744073709551615"
	got, ok := parseStartTime(line)
	if !ok || got != "136546471" {
		t.Errorf("got %q %v", got, ok)
	}
	if _, ok := parseStartTime("1 (a) S 1 2 3"); ok {
		t.Error("short line accepted")
	}
	if _, ok := parseStartTime("no parenthesis"); ok {
		t.Error("line without parenthesis accepted")
	}
}

func TestProcAlive(t *testing.T) {
	pid := os.Getpid()
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		t.Skip("no /proc")
	}
	start, ok := parseStartTime(string(data))
	if !ok {
		t.Fatal("cannot parse own stat")
	}
	if !procAlive(pid, start) {
		t.Error("own pid with matching start reported dead")
	}
	if procAlive(pid, start+"1") {
		t.Error("own pid with different start reported alive")
	}
	if !procAlive(pid, "") {
		t.Error("own pid with empty procStart reported dead")
	}
	if procAlive(1<<30, "") {
		t.Error("nonexistent pid reported alive")
	}
}

func TestDefaultAliveByGOOS(t *testing.T) {
	pid := os.Getpid()
	if !DefaultAlive("darwin")(pid, "not-a-start-time") {
		t.Error("darwin: own pid with any procStart must be alive")
	}
	if DefaultAlive("darwin")(1<<30, "") {
		t.Error("darwin: nonexistent pid reported alive")
	}
	if DefaultAlive("darwin")(0, "") || DefaultAlive("darwin")(-1, "") {
		t.Error("darwin: non-positive pid reported alive")
	}
	if _, err := os.ReadFile("/proc/self/stat"); err != nil {
		t.Skip("no /proc")
	}
	if DefaultAlive("linux")(pid, "not-a-start-time") {
		t.Error("linux: mismatching procStart must be dead")
	}
	if !DefaultAlive("linux")(pid, "") {
		t.Error("linux: own pid reported dead")
	}
}

func TestSessionsDefaultsToDefaultAlive(t *testing.T) {
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		t.Skip("no /proc")
	}
	start, ok := parseStartTime(string(data))
	if !ok {
		t.Fatal("cannot parse own stat")
	}
	dir := t.TempDir()
	writeSession(t, dir, fmt.Sprintf("%d.json", os.Getpid()),
		fmt.Sprintf(`{"pid":%d,"procStart":"%s"}`, os.Getpid(), start))
	writeSession(t, dir, "7.json", fmt.Sprintf(`{"pid":7,"procStart":"x"}`))
	writeSession(t, dir, "99999999.json", `{"pid":99999999}`)
	ss, err := Reader{Dir: dir}.Sessions()
	if err != nil || len(ss) != 1 || ss[0].PID != os.Getpid() {
		t.Fatalf("sessions=%+v err=%v", ss, err)
	}
	writeSession(t, dir, fmt.Sprintf("%d.json", os.Getpid()),
		fmt.Sprintf(`{"pid":%d,"procStart":"%s1"}`, os.Getpid(), start))
	if ss, _ := (Reader{Dir: dir}).Sessions(); len(ss) != 0 {
		t.Errorf("live pid with a wrong procStart kept: %+v", ss)
	}
}

func TestPidAliveEPERM(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can signal PID 1")
	}
	if !pidAlive(1, "") {
		t.Error("PID 1 (EPERM) reported dead")
	}
}

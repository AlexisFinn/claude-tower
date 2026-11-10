package titles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

// fixture is a Store with a fake clock and a fake claude.
type fixture struct {
	t     *testing.T
	store *Store
	path  string // cache file
	now   time.Time
	mu    sync.Mutex
	calls []Call
	out   string
	err   error
	block chan struct{} // when set, the fake claude waits for it to be closed
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, path: filepath.Join(t.TempDir(), "cache", "titles.json"), now: t0, out: "Titre un"}
	f.store = New(Options{
		Path:   f.path,
		Dir:    filepath.Join(t.TempDir(), "work"),
		Claude: func(context.Context, []string) (string, error) { return "/bin/claude", nil },
		Env:    func(context.Context) []string { return nil },
		Run: func(_ context.Context, c Call) ([]byte, error) {
			f.mu.Lock()
			f.calls = append(f.calls, c)
			block, out, err := f.block, f.out, f.err
			f.mu.Unlock()
			if block != nil {
				<-block
			}
			return []byte(out), err
		},
		Now: func() time.Time { f.mu.Lock(); defer f.mu.Unlock(); return f.now },
	})
	return f
}

func (f *fixture) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

func (f *fixture) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// transcript writes a transcript of n requests and makes its file differ from
// the previous version.
func (f *fixture) transcript(path string, n int) {
	f.t.Helper()
	var lines []string
	for i := 1; i <= n; i++ {
		lines = append(lines, userLine(fmt.Sprintf("demande %d", i)))
	}
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}

// title asks for the title and waits for the generation it may start.
func (f *fixture) title(id, path string) string {
	f.t.Helper()
	got := f.store.Title(id, path)
	f.store.Wait()
	return got
}

func TestTitleGeneratedOnceFromFirstRequest(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(t.TempDir(), "s.jsonl")
	f.transcript(path, 1)
	if got := f.store.Title("s1", path); got != "" {
		t.Errorf("title before generation: %q", got)
	}
	f.store.Wait()
	if got := f.store.Title("s1", path); got != "Titre un" {
		t.Errorf("title: %q", got)
	}
	f.store.Wait()
	f.advance(time.Hour)
	f.title("s1", path)
	if n := f.callCount(); n != 1 {
		t.Errorf("claude run %d times", n)
	}
}

func TestNoTitleWithoutRequest(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(t.TempDir(), "s.jsonl")
	f.transcript(path, 0)
	f.title("s1", path)
	f.title("s1", "")
	if n := f.callCount(); n != 0 {
		t.Errorf("claude run %d times", n)
	}
	f.transcript(path, 1)
	f.advance(recheckAfter)
	f.title("s1", path)
	if n := f.callCount(); n != 1 {
		t.Errorf("claude run %d times after the first request", n)
	}
}

func TestTitleRegeneratedAfterFiveMoreRequests(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(t.TempDir(), "s.jsonl")
	f.transcript(path, 2)
	f.title("s1", path)
	if got := f.store.Title("s1", ""); got != "Titre un" {
		t.Fatalf("title: %q", got)
	}
	f.out = "Titre deux"
	f.transcript(path, 6)
	f.advance(time.Minute)
	if got := f.title("s1", path); got != "Titre un" {
		t.Errorf("the old title is shown during the check: %q", got)
	}
	if n := f.callCount(); n != 1 {
		t.Errorf("claude run %d times with 4 more requests", n)
	}
	f.transcript(path, 7)
	f.advance(time.Minute)
	if got := f.title("s1", path); got != "Titre un" {
		t.Errorf("the old title is shown during the generation: %q", got)
	}
	if n := f.callCount(); n != 2 {
		t.Fatalf("claude run %d times with 5 more requests", n)
	}
	if got := f.store.Title("s1", ""); got != "Titre deux" {
		t.Errorf("title: %q", got)
	}
}

func TestTranscriptReadAgainOnlyWhenChangedAndNotTooOften(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(t.TempDir(), "s.jsonl")
	f.transcript(path, 1)
	f.title("s1", path)
	f.out = "Titre deux"
	f.transcript(path, 6)
	f.advance(recheckAfter - time.Second)
	f.title("s1", path)
	if n := f.callCount(); n != 1 {
		t.Errorf("transcript read again after %v: %d runs", recheckAfter-time.Second, n)
	}
	f.advance(time.Second)
	f.title("s1", path)
	if n := f.callCount(); n != 2 {
		t.Errorf("runs: %d", n)
	}
}

func TestFailureWaitsTenMinutes(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(t.TempDir(), "s.jsonl")
	f.transcript(path, 1)
	f.err = errors.New("boom")
	f.title("s1", path)
	for _, wait := range []time.Duration{time.Minute, 8 * time.Minute} {
		f.advance(wait)
		f.title("s1", path)
		if n := f.callCount(); n != 1 {
			t.Fatalf("retried before 10 minutes: %d runs", n)
		}
	}
	f.advance(time.Minute + time.Second)
	f.err = nil
	f.title("s1", path)
	if n := f.callCount(); n != 2 {
		t.Fatalf("not retried after 10 minutes: %d runs", n)
	}
	if got := f.store.Title("s1", ""); got != "Titre un" {
		t.Errorf("title: %q", got)
	}
}

func TestEmptyOutputIsAFailure(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(t.TempDir(), "s.jsonl")
	f.transcript(path, 1)
	f.out = "\n"
	f.title("s1", path)
	f.advance(time.Minute)
	f.title("s1", path)
	if n := f.callCount(); n != 1 {
		t.Errorf("runs: %d", n)
	}
	if got := f.store.Title("s1", ""); got != "" {
		t.Errorf("title: %q", got)
	}
}

func TestFailureKeepsOldTitle(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(t.TempDir(), "s.jsonl")
	f.transcript(path, 1)
	f.title("s1", path)
	f.err = errors.New("boom")
	f.transcript(path, 9)
	f.advance(time.Minute)
	f.title("s1", path)
	if got := f.store.Title("s1", ""); got != "Titre un" {
		t.Errorf("title: %q", got)
	}
}

func TestOneGenerationAtATime(t *testing.T) {
	f := newFixture(t)
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.jsonl"), filepath.Join(dir, "b.jsonl")
	f.transcript(a, 1)
	f.transcript(b, 1)
	f.block = make(chan struct{})
	f.store.Title("a", a)
	if got := f.store.Title("b", b); got != "" {
		t.Errorf("title: %q", got)
	}
	close(f.block)
	f.store.Wait()
	if n := f.callCount(); n != 1 {
		t.Fatalf("runs while one was running: %d", n)
	}
	f.title("b", b)
	if n := f.callCount(); n != 2 {
		t.Errorf("runs: %d", n)
	}
}

func TestTitleNeverWaitsForGeneration(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(t.TempDir(), "s.jsonl")
	f.transcript(path, 1)
	f.block = make(chan struct{})
	done := make(chan string)
	go func() { done <- f.store.Title("s1", path) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Title waited for the generation")
	}
	close(f.block)
	f.store.Wait()
}

func TestMissingTranscriptGivesNoTitle(t *testing.T) {
	f := newFixture(t)
	if got := f.title("s1", filepath.Join(t.TempDir(), "absent.jsonl")); got != "" {
		t.Errorf("title: %q", got)
	}
	if n := f.callCount(); n != 0 {
		t.Errorf("runs: %d", n)
	}
}

func readCache(t *testing.T, path string) map[string]entry {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]entry
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCacheIsWrittenAtomicallyAndRead(t *testing.T) {
	f := newFixture(t)
	path := filepath.Join(t.TempDir(), "s.jsonl")
	f.transcript(path, 3)
	f.title("s1", path)
	want := map[string]entry{"s1": {Title: "Titre un", Requests: 3}}
	if got := readCache(t, f.path); !reflect.DeepEqual(got, want) {
		t.Errorf("cache: %v", got)
	}
	entries, err := os.ReadDir(filepath.Dir(f.path))
	if err != nil || len(entries) != 1 {
		t.Errorf("files left in the cache directory: %v %v", entries, err)
	}
	if info, err := os.Stat(f.path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("cache file mode: %v %v", info, err)
	}

	again := New(Options{Path: f.path})
	if got := again.Title("s1", ""); got != "Titre un" {
		t.Errorf("title read from the cache: %q", got)
	}
	if got := again.Title("other", ""); got != "" {
		t.Errorf("title of another session: %q", got)
	}
}

func TestCacheCorruptOrMissingIsEmpty(t *testing.T) {
	dir := t.TempDir()
	if got := New(Options{Path: filepath.Join(dir, "absent.json")}).Title("s1", ""); got != "" {
		t.Errorf("title: %q", got)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := New(Options{Path: bad}).Title("s1", ""); got != "" {
		t.Errorf("title: %q", got)
	}
}

func TestSyncDropsSessionsAbsentOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "titles.json")
	data := `{"live":{"title":"Vivante","requests":2},"gone":{"title":"Partie","requests":4}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(Options{Path: path})
	s.Sync([]string{"live"})
	if got := s.Title("gone", ""); got != "" {
		t.Errorf("title of a gone session: %q", got)
	}
	if got := s.Title("live", ""); got != "Vivante" {
		t.Errorf("title: %q", got)
	}
	if got := readCache(t, path); !reflect.DeepEqual(got, map[string]entry{"live": {Title: "Vivante", Requests: 2}}) {
		t.Errorf("cache after purge: %v", got)
	}
	s.Sync(nil)
	if got := s.Title("live", ""); got != "Vivante" {
		t.Errorf("a later Sync purged: %q", got)
	}
}

func TestSyncLeavesUnchangedCacheAlone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "titles.json")
	s := New(Options{Path: path})
	s.Sync([]string{"a"})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cache file written without change: %v", err)
	}
}

func TestCacheDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/xdg")
	if got := CacheDir(); got != "/xdg/claude-tower" {
		t.Errorf("CacheDir = %q", got)
	}
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "/home/u")
	if got := CacheDir(); got != "/home/u/.cache/claude-tower" {
		t.Errorf("CacheDir = %q", got)
	}
	s := New(Options{})
	if s.opts.Path != "/home/u/.cache/claude-tower/titles.json" || s.opts.Dir != "/home/u/.cache/claude-tower/titles" {
		t.Errorf("defaults: %q %q", s.opts.Path, s.opts.Dir)
	}
}

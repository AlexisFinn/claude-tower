package titles

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const (
	// regenerateAfter is the number of new requests that makes a title stale.
	regenerateAfter = 5
	// retryAfter is how long a session waits after a failed generation.
	retryAfter = 10 * time.Minute
	// recheckAfter is the least time between two reads of the transcript of a
	// session.
	recheckAfter = 30 * time.Second
)

// Options configures a Store. Claude is required for titles to be generated;
// the other fields have defaults.
type Options struct {
	// Path is the cache file. Default: titles.json in the cache directory of
	// the application.
	Path string
	// Dir is the working directory of the claude process, which Claude Code
	// remembers as a project. Default: titles in the cache directory of the
	// application.
	Dir string
	// Claude returns the claude executable to run, given the environment of
	// the process. Default: claude found on PATH.
	Claude func(ctx context.Context, env []string) (string, error)
	// Env returns the environment of the claude process. Default: the
	// environment of the calling process.
	Env func(ctx context.Context) []string
	// Run runs the claude process. Default: os/exec.
	Run Runner
	// Now returns the current time. Default: time.Now.
	Now func() time.Time
}

type entry struct {
	Title    string `json:"title"`
	Requests int    `json:"requests"` // number of requests when the title was written
}

// stamp identifies the state of a transcript file.
type stamp struct {
	size  int64
	mtime time.Time
}

// Store holds the generated titles of the sessions and generates the missing
// or stale ones in the background, one at a time. It is safe for concurrent
// use.
type Store struct {
	opts Options

	mu      sync.Mutex
	loaded  bool
	synced  bool
	entries map[string]entry
	running bool
	wg      sync.WaitGroup
	failed  map[string]time.Time // end of the last failed generation, per session ID
	checked map[string]checkMark // last read of the transcript, per session ID
}

type checkMark struct {
	at   time.Time
	file stamp
}

// CacheDir returns the directory where the application keeps its cache:
// claude-tower in $XDG_CACHE_HOME, else in ~/.cache. It is empty when the home
// directory is unknown, in which case titles are neither generated nor kept.
func CacheDir() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "claude-tower")
}

// New returns a Store.
func New(opts Options) *Store {
	if cache := CacheDir(); cache != "" {
		if opts.Path == "" {
			opts.Path = filepath.Join(cache, "titles.json")
		}
		if opts.Dir == "" {
			opts.Dir = filepath.Join(cache, "titles")
		}
	}
	if opts.Claude == nil {
		opts.Claude = func(context.Context, []string) (string, error) { return exec.LookPath("claude") }
	}
	if opts.Env == nil {
		opts.Env = func(context.Context) []string { return os.Environ() }
	}
	if opts.Run == nil {
		opts.Run = execRunner
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Store{opts: opts, entries: map[string]entry{}, failed: map[string]time.Time{}, checked: map[string]checkMark{}}
}

// load reads the cache file once. A missing or corrupt file leaves the store
// empty. The caller holds s.mu.
func (s *Store) load() {
	if s.loaded {
		return
	}
	s.loaded = true
	data, err := os.ReadFile(s.opts.Path)
	if err != nil {
		return
	}
	var entries map[string]entry
	if json.Unmarshal(data, &entries) == nil {
		for id, e := range entries {
			if e.Title != "" {
				s.entries[id] = e
			}
		}
	}
}

// save writes the cache file through a temporary file and a rename, so that a
// reader never sees a partial file. A failure leaves the titles in memory
// only. The caller holds s.mu.
func (s *Store) save() {
	if s.opts.Path == "" {
		return
	}
	data, err := json.Marshal(s.entries)
	if err != nil {
		return
	}
	dir := filepath.Dir(s.opts.Path)
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	f, err := os.CreateTemp(dir, ".titles-*.tmp")
	if err != nil {
		return
	}
	_, werr := f.Write(data)
	if werr != nil || f.Close() != nil || os.Rename(f.Name(), s.opts.Path) != nil {
		f.Close()
		os.Remove(f.Name())
	}
}

// Sync loads the cache the first time it is called and drops the titles of the
// sessions that are not in live: a session absent from the registry when the
// application starts is gone, and a session that comes back is titled again.
// Later calls do nothing.
func (s *Store) Sync(live []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	if s.synced {
		return
	}
	s.synced = true
	keep := make(map[string]bool, len(live))
	for _, id := range live {
		keep[id] = true
	}
	changed := false
	for id := range s.entries {
		if !keep[id] {
			delete(s.entries, id)
			changed = true
		}
	}
	if changed {
		s.save()
	}
}

// Title returns the generated title of a session, "" when it has none. It
// never waits for a generation: when the transcript at path may call for a new
// title, the check and the generation run in the background. An empty path
// only reads the title.
func (s *Store) Title(sessionID, path string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.load()
	title := s.entries[sessionID].Title
	if path == "" || s.running {
		return title
	}
	now := s.opts.Now()
	if at, ok := s.failed[sessionID]; ok && now.Sub(at) < retryAfter {
		return title
	}
	info, err := os.Stat(path)
	if err != nil {
		return title
	}
	file := stamp{size: info.Size(), mtime: info.ModTime()}
	if mark, ok := s.checked[sessionID]; ok && (mark.file == file || now.Sub(mark.at) < recheckAfter) {
		return title
	}
	s.running = true
	s.wg.Add(1)
	go s.refresh(sessionID, path, file)
	return title
}

// Wait blocks until the running generation, if any, ends.
func (s *Store) Wait() {
	s.wg.Wait()
}

// needsTitle reports whether a session with count requests calls for a new
// title, given the one it has.
func needsTitle(count int, current entry) bool {
	if count < 1 {
		return false
	}
	return current.Title == "" || count >= current.Requests+regenerateAfter
}

// refresh reads the transcript of a session and generates a title when the
// policy asks for one.
func (s *Store) refresh(sessionID, path string, file stamp) {
	defer s.wg.Done()
	reqs, err := ScanRequests(path)
	s.mu.Lock()
	current := s.entries[sessionID]
	s.mu.Unlock()
	var title string
	if err == nil && needsTitle(reqs.Count, current) {
		title, err = s.generate(reqs.Kept)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	now := s.opts.Now()
	if err != nil {
		s.failed[sessionID] = now
		return
	}
	s.checked[sessionID] = checkMark{at: now, file: file}
	if title != "" {
		s.entries[sessionID] = entry{Title: title, Requests: reqs.Count}
		s.save()
	}
}

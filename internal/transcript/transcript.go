// Package transcript extracts the last visible activity of a Claude Code
// session from its JSONL transcript.
package transcript

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Activity describes the tail of a session transcript.
type Activity struct {
	Text      string // "" when nothing retained
	TurnEnded bool   // the last turn-relevant record is an end-of-turn marker
}

type cacheEntry struct {
	path  string
	size  int64
	mtime time.Time
	act   Activity
}

// Finder locates transcripts and caches their last activity. It is safe for
// concurrent use.
type Finder struct {
	configDir string
	windows   []int64
	readAt    func(path string, off, n int64) ([]byte, error)

	mu    sync.Mutex
	cache map[string]*cacheEntry
}

var sessionIDRe = regexp.MustCompile(`^[0-9a-fA-F-]{8,64}$`)

// NewFinder returns a Finder for the transcripts under configDir/projects.
func NewFinder(configDir string) *Finder {
	return &Finder{
		configDir: configDir,
		windows:   []int64{256 << 10, 1 << 20, 4 << 20},
		readAt:    readAtFile,
		cache:     map[string]*cacheEntry{},
	}
}

// Last returns the last activity of a session. It returns the zero Activity
// when the session ID is invalid or no transcript exists.
func (f *Finder) Last(sessionID string) Activity {
	if !sessionIDRe.MatchString(sessionID) {
		return Activity{}
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	e := f.cache[sessionID]
	var info os.FileInfo
	if e != nil {
		var err error
		if info, err = os.Stat(e.path); err != nil {
			delete(f.cache, sessionID)
			e = nil
		}
	}
	if e == nil {
		path, fi := f.find(sessionID)
		if path == "" {
			return Activity{}
		}
		info = fi
		e = &cacheEntry{path: path, size: -1}
	}
	if info.Size() == e.size && info.ModTime().Equal(e.mtime) {
		return e.act
	}
	act, err := f.scanTail(e.path, info.Size())
	if err != nil {
		return Activity{}
	}
	e.size, e.mtime, e.act = info.Size(), info.ModTime(), act
	f.cache[sessionID] = e
	return act
}

// find searches the project directories one by one rather than using a glob,
// because the config directory path may contain glob metacharacters.
func (f *Finder) find(sessionID string) (string, os.FileInfo) {
	root := filepath.Join(f.configDir, "projects")
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", nil
	}
	for _, e := range entries {
		p := filepath.Join(root, e.Name(), sessionID+".jsonl")
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p, fi
		}
	}
	return "", nil
}

func readAtFile(path string, off, n int64) ([]byte, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	buf := make([]byte, n)
	read, err := fh.ReadAt(buf, off)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buf[:read], nil
}

// scanTail reads growing windows from the end of the file until a retained
// record is found or the start of the file is reached.
func (f *Finder) scanTail(path string, size int64) (Activity, error) {
	var act Activity
	for _, w := range f.windows {
		start := max(size-w, 0)
		readStart := start
		if start > 0 {
			readStart = start - 1
		}
		data, err := f.readAt(path, readStart, size-readStart)
		if err != nil {
			return Activity{}, err
		}
		if start > 0 && len(data) > 0 {
			prev := data[0]
			data = data[1:]
			if prev != '\n' {
				data = dropFirstLine(data)
			}
		}
		var found bool
		act, found = scanLines(data)
		if found || start == 0 {
			return act, nil
		}
	}
	return act, nil
}

func dropFirstLine(data []byte) []byte {
	i := bytes.IndexByte(data, '\n')
	if i < 0 {
		return nil
	}
	return data[i+1:]
}

type record struct {
	Type                      string `json:"type"`
	Subtype                   string `json:"subtype"`
	IsMeta                    bool   `json:"isMeta"`
	IsCompactSummary          bool   `json:"isCompactSummary"`
	IsVisibleInTranscriptOnly bool   `json:"isVisibleInTranscriptOnly"`
	Message                   struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type block struct {
	Type  string         `json:"type"`
	Text  string         `json:"text"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// scanLines walks the lines from the last to the first. found reports whether
// a retained record produced the text.
func scanLines(data []byte) (act Activity, found bool) {
	lines := bytes.Split(data, []byte("\n"))
	turnKnown := false
	for i := len(lines) - 1; i >= 0; i-- {
		var rec record
		if len(bytes.TrimSpace(lines[i])) == 0 || json.Unmarshal(lines[i], &rec) != nil {
			continue
		}
		isTurnMarker := rec.Type == "system" && rec.Subtype == "turn_duration"
		if !turnKnown && (rec.Type == "assistant" || rec.Type == "user" || isTurnMarker) {
			turnKnown = true
			act.TurnEnded = isTurnMarker
		}
		if text := retainedText(rec); text != "" {
			act.Text = text
			return act, true
		}
	}
	return act, false
}

func retainedText(rec record) string {
	if rec.IsMeta || rec.IsCompactSummary || rec.IsVisibleInTranscriptOnly {
		return ""
	}
	switch rec.Type {
	case "assistant":
		return assistantText(rec.Message.Content)
	case "user":
		return userText(rec.Message.Content)
	}
	return ""
}

func decodeBlocks(raw json.RawMessage) []block {
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	return blocks
}

func assistantText(raw json.RawMessage) string {
	blocks := decodeBlocks(raw)
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].Type == "tool_use" {
			return toolText(blocks[i])
		}
	}
	for i := len(blocks) - 1; i >= 0; i-- {
		if blocks[i].Type == "text" {
			if line := firstLine(blocks[i].Text); line != "" {
				return line
			}
		}
	}
	return ""
}

func toolText(b block) string {
	key := ""
	switch b.Name {
	case "Bash":
		key = "command"
	case "Read", "Edit", "Write":
		key = "file_path"
	case "Grep", "Glob":
		key = "pattern"
	case "Agent":
		key = "description"
	}
	if key != "" {
		if arg, _ := b.Input[key].(string); normalize(arg) != "" {
			return normalize(b.Name + ": " + arg)
		}
	}
	return normalize(b.Name)
}

func userText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return promptText(s)
	}
	for _, b := range decodeBlocks(raw) {
		if b.Type == "text" {
			if text := promptText(b.Text); text != "" {
				return text
			}
		}
	}
	return ""
}

func promptText(s string) string {
	s = strings.TrimLeftFunc(s, unicode.IsSpace)
	for _, prefix := range []string{"<", "[Request interrupted", "[Image"} {
		if strings.HasPrefix(s, prefix) {
			return ""
		}
	}
	if line := firstLine(s); line != "" {
		return "› " + line
	}
	return ""
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if line = normalize(line); line != "" {
			return line
		}
	}
	return ""
}

// normalize collapses runs of whitespace and control characters into a single
// space and trims the result.
func normalize(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}), " ")
}

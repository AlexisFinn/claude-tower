// Package registry reads the per-session files that Claude Code keeps in
// <configDir>/sessions. The format is internal to Claude Code: everything that
// depends on it lives in this package.
package registry

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// State is the normalized activity state of a session.
type State int

// Normalized states. The zero value is StateUnknown.
const (
	StateUnknown State = iota
	StateBusy
	StateWaiting
	StateIdle
)

const maxWaitingForRunes = 32

// Session is one live Claude Code session.
type Session struct {
	PID         int
	SessionID   string
	Name        string // raw registry "name", may be empty
	Cwd         string
	Kind        string
	State       State
	RawStatus   string    // "busy", "shell", "idle", "waiting", or anything else
	WaitingFor  string    // registry "waitingFor", "" when absent
	StartedAt   time.Time // zero if absent
	StatusSince time.Time // statusUpdatedAt, else updatedAt, else zero
	TmuxPane    string    // "%N" extracted from "tmux", "" if absent or malformed
	TurnEnded   bool      // never set by registry; filled by callers from transcript.Activity
}

// Reader reads the session registry of one config directory.
type Reader struct {
	Dir      string                               // absolute config dir, such as the expansion of ~/.claude
	Alive    func(pid int, procStart string) bool // default: procAlive
	ReadFile func(path string) ([]byte, error)    // default: os.ReadFile
}

var (
	sessionFileRe = regexp.MustCompile(`^([0-9]+)\.json$`)
	paneRe        = regexp.MustCompile(`^%[0-9]+$`)
)

// Sessions returns the live sessions found in the registry. A missing
// sessions directory yields an empty list; an error is returned only when the
// directory exists but cannot be read.
func (r Reader) Sessions() ([]Session, error) {
	dir := filepath.Join(r.Dir, "sessions")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	alive, read := r.Alive, r.ReadFile
	if alive == nil {
		alive = procAlive
	}
	if read == nil {
		read = os.ReadFile
	}
	var out []Session
	for _, e := range entries {
		m := sessionFileRe.FindStringSubmatch(e.Name())
		if m == nil || e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		s, procStart, ok := readSession(read, filepath.Join(dir, e.Name()), pid)
		if ok && alive(pid, procStart) {
			out = append(out, s)
		}
	}
	return out, nil
}

// readSession decodes one registry file, retrying once after a short delay in
// case the file is being rewritten. ok is false for files that must be skipped.
func readSession(read func(string) ([]byte, error), path string, pid int) (s Session, procStart string, ok bool) {
	rec, ok := readRecord(read, path)
	if !ok {
		return Session{}, "", false
	}
	if p := int(num(rec, "pid")); p != 0 && p != pid {
		return Session{}, "", false
	}
	kind := clean(str(rec, "kind"))
	if kind == "daemon" || kind == "daemon-worker" || boolean(rec, "spare") {
		return Session{}, "", false
	}
	// Non-interactive runs (claude -p, SDK) register with kind "interactive"
	// and an "sdk-*" entrypoint.
	if strings.HasPrefix(str(rec, "entrypoint"), "sdk") {
		return Session{}, "", false
	}
	raw := clean(str(rec, "status"))
	s = Session{
		PID:         pid,
		SessionID:   str(rec, "sessionId"),
		Name:        clean(str(rec, "name")),
		Cwd:         clean(str(rec, "cwd")),
		Kind:        kind,
		State:       stateOf(raw),
		RawStatus:   raw,
		WaitingFor:  truncateRunes(clean(str(rec, "waitingFor")), maxWaitingForRunes),
		StartedAt:   millis(rec, "startedAt"),
		StatusSince: statusSince(rec),
		TmuxPane:    tmuxPane(str(rec, "tmux")),
	}
	return s, procStartOf(rec), true
}

func readRecord(read func(string) ([]byte, error), path string) (map[string]json.RawMessage, bool) {
	for attempt := 0; attempt < 2; attempt++ {
		if attempt > 0 {
			time.Sleep(20 * time.Millisecond)
		}
		data, err := read(path)
		if err != nil {
			return nil, false
		}
		var rec map[string]json.RawMessage
		if json.Unmarshal(data, &rec) == nil && rec != nil {
			return rec, true
		}
	}
	return nil, false
}

func stateOf(status string) State {
	switch status {
	case "busy", "shell":
		return StateBusy
	case "waiting":
		return StateWaiting
	case "idle":
		return StateIdle
	}
	return StateUnknown
}

func statusSince(rec map[string]json.RawMessage) time.Time {
	if t := millis(rec, "statusUpdatedAt"); !t.IsZero() {
		return t
	}
	return millis(rec, "updatedAt")
}

func tmuxPane(tmux string) string {
	i := strings.LastIndex(tmux, ".")
	if i < 0 {
		return ""
	}
	if pane := tmux[i+1:]; paneRe.MatchString(pane) {
		return pane
	}
	return ""
}

// procStartOf returns procStart as a digit string whether the registry stores
// it as a string or as a number.
func procStartOf(rec map[string]json.RawMessage) string {
	raw := rec["procStart"]
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return strings.TrimSpace(s)
	}
	var n json.Number
	if json.Unmarshal(raw, &n) != nil {
		return ""
	}
	if _, err := strconv.ParseUint(n.String(), 10, 64); err == nil {
		return n.String()
	}
	f, err := n.Float64()
	if err != nil || f < 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', 0, 64)
}

func str(rec map[string]json.RawMessage, key string) string {
	var s string
	if json.Unmarshal(rec[key], &s) != nil {
		return ""
	}
	return s
}

func num(rec map[string]json.RawMessage, key string) float64 {
	var f float64
	if json.Unmarshal(rec[key], &f) != nil {
		return 0
	}
	return f
}

func boolean(rec map[string]json.RawMessage, key string) bool {
	var b bool
	return json.Unmarshal(rec[key], &b) == nil && b
}

func millis(rec map[string]json.RawMessage, key string) time.Time {
	ms := num(rec, key)
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(ms))
}

func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// procAlive reports whether pid exists and, when procStart is not empty,
// whether its start time matches, which rules out a reused PID.
func procAlive(pid int, procStart string) bool {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	if procStart == "" {
		return true
	}
	start, ok := parseStartTime(string(data))
	return ok && start == procStart
}

// parseStartTime extracts the starttime field (22) from a /proc/<pid>/stat
// line. The comm field may contain spaces and parentheses, so the line is
// split after its last closing parenthesis.
func parseStartTime(statLine string) (string, bool) {
	i := strings.LastIndex(statLine, ")")
	if i < 0 {
		return "", false
	}
	fields := strings.Fields(statLine[i+1:])
	if len(fields) <= 19 {
		return "", false
	}
	return fields[19], true
}

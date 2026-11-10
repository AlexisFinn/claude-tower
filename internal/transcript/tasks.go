package transcript

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

// TaskKind tells which kind of background task a TaskEvent concerns.
type TaskKind int

// Kinds of background tasks. TaskAny is an event that does not say the kind.
const (
	TaskAny TaskKind = iota
	TaskAgent
	TaskShell
)

// TaskEvent is the launch or the end of a background task, as recorded by one
// transcript line.
type TaskEvent struct {
	Kind     TaskKind
	ID       string
	Launched bool      // false when the task ended
	At       time.Time // time of the line; zero when it has none or it is unreadable
}

var taskIDRe = regexp.MustCompile(`<task-id>\s*([^<\s]+)\s*</task-id>`)

var taskMarkers = [][]byte{
	[]byte(`"async_launched"`),
	[]byte(`"backgroundTaskId"`),
	[]byte(`"task_type"`),
	[]byte(`"queue-operation"`),
}

type toolResult struct {
	Status           string `json:"status"`
	AgentID          string `json:"agentId"`
	TaskID           string `json:"task_id"`
	TaskType         string `json:"task_type"`
	BackgroundTaskID string `json:"backgroundTaskId"`
}

type taskRecord struct {
	Type          string          `json:"type"`
	Operation     string          `json:"operation"`
	Content       string          `json:"content"`
	Timestamp     string          `json:"timestamp"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
}

// mayCarryTask reports whether a line contains one of the strings that every
// launch or end record holds.
func mayCarryTask(line []byte) bool {
	for _, m := range taskMarkers {
		if bytes.Contains(line, m) {
			return true
		}
	}
	return false
}

// TaskEvents returns the background task launches and ends that one
// transcript line records, in the order they apply. Lines that cannot carry
// one are skipped without being decoded.
func TaskEvents(line []byte) []TaskEvent {
	if !mayCarryTask(line) {
		return nil
	}
	var rec taskRecord
	var typeErr *json.UnmarshalTypeError
	if err := json.Unmarshal(line, &rec); err != nil && !errors.As(err, &typeErr) {
		return nil
	}
	switch {
	case rec.Type == "queue-operation" && rec.Operation == "enqueue":
		return notificationEvents(rec.Content)
	case len(rec.ToolUseResult) > 0 && rec.ToolUseResult[0] == '{':
		var res toolResult
		if err := json.Unmarshal(rec.ToolUseResult, &res); err != nil && !errors.As(err, &typeErr) {
			return nil
		}
		return resultEvents(res, parseTime(rec.Timestamp))
	}
	return nil
}

// notificationEvents returns the ends reported by the task notifications of a
// queued message.
func notificationEvents(content string) []TaskEvent {
	var events []TaskEvent
	for _, block := range strings.Split(content, "<task-notification>")[1:] {
		if !strings.Contains(block, "<status>") {
			continue
		}
		if m := taskIDRe.FindStringSubmatch(block); m != nil {
			events = append(events, TaskEvent{Kind: TaskAny, ID: m[1]})
		}
	}
	return events
}

func resultEvents(res toolResult, ts time.Time) []TaskEvent {
	var events []TaskEvent
	if res.TaskID != "" {
		switch res.TaskType {
		case "local_agent":
			events = append(events, TaskEvent{Kind: TaskAgent, ID: res.TaskID})
		case "local_bash":
			events = append(events, TaskEvent{Kind: TaskShell, ID: res.TaskID})
		}
	}
	if res.Status == "async_launched" && res.AgentID != "" {
		events = append(events, TaskEvent{Kind: TaskAgent, ID: res.AgentID, Launched: true, At: ts})
	}
	if res.BackgroundTaskID != "" {
		events = append(events, TaskEvent{Kind: TaskShell, ID: res.BackgroundTaskID, Launched: true, At: ts})
	}
	return events
}

func parseTime(s string) time.Time {
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return ts
}

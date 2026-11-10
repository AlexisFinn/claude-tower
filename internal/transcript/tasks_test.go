package transcript

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)

func launchAgentLine(id string) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"toolUseResult":{"status":"async_launched","agentId":%q}}`, t0.Format(time.RFC3339Nano), id)
}

func launchShellLine(id string) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"toolUseResult":{"backgroundTaskId":%q}}`, t0.Format(time.RFC3339Nano), id)
}

func notificationLine(id, status string) string {
	content := fmt.Sprintf("<task-notification>\\n<task-id>%s</task-id>\\n<tool-use-id>toolu_1</tool-use-id>\\n<status>%s</status>\\n</task-notification>", id, status)
	return fmt.Sprintf(`{"type":"queue-operation","operation":"enqueue","content":"%s"}`, content)
}

func stopLine(id, taskType string) string {
	return fmt.Sprintf(`{"type":"user","toolUseResult":{"task_id":%q,"task_type":%q}}`, id, taskType)
}

func TestTaskEvents(t *testing.T) {
	cases := []struct {
		name string
		line string
		want []TaskEvent
	}{
		{"agent launch", launchAgentLine("aaa"), []TaskEvent{{Kind: TaskAgent, ID: "aaa", Launched: true, At: t0}}},
		{"shell launch", launchShellLine("bone"), []TaskEvent{{Kind: TaskShell, ID: "bone", Launched: true, At: t0}}},
		{"launch without timestamp", `{"type":"user","toolUseResult":{"status":"async_launched","agentId":"aaa"}}`, []TaskEvent{{Kind: TaskAgent, ID: "aaa", Launched: true}}},
		{"launch with unreadable timestamp", `{"type":"user","timestamp":"yesterday","toolUseResult":{"backgroundTaskId":"bone"}}`, []TaskEvent{{Kind: TaskShell, ID: "bone", Launched: true}}},
		{"foreground agent", `{"type":"user","toolUseResult":{"status":"completed","agentId":"aaa"}}`, nil},
		{"agent stop", stopLine("aaa", "local_agent"), []TaskEvent{{Kind: TaskAgent, ID: "aaa"}}},
		{"shell stop", stopLine("bone", "local_bash"), []TaskEvent{{Kind: TaskShell, ID: "bone"}}},
		{"stop of another task type", stopLine("aaa", "monitor"), nil},
		{"notification", notificationLine("aaa", "killed"), []TaskEvent{{Kind: TaskAny, ID: "aaa"}}},
		{
			"two notifications",
			`{"type":"queue-operation","operation":"enqueue","content":"<task-notification><task-id>a</task-id><status>x</status></task-notification><task-notification><task-id> b </task-id><status>y</status></task-notification>"}`,
			[]TaskEvent{{Kind: TaskAny, ID: "a"}, {Kind: TaskAny, ID: "b"}},
		},
		{"notification without status", `{"type":"queue-operation","operation":"enqueue","content":"<task-notification><task-id>aaa</task-id></task-notification>"}`, nil},
		{"dequeue", `{"type":"queue-operation","operation":"dequeue","content":"<task-notification><task-id>aaa</task-id><status>completed</status></task-notification>"}`, nil},
		{"enqueue of another record type", `{"type":"attachment","operation":"enqueue","source":"queue-operation","content":"<task-notification><task-id>aaa</task-id><status>completed</status></task-notification>"}`, nil},
		{"tool result that is a string", `{"type":"user","toolUseResult":"Error: async_launched"}`, nil},
		{"corrupt line", `{"toolUseResult": {oops "async_launched"`, nil},
		{
			"stop and launch of the same line",
			`{"type":"user","timestamp":"` + t0.Format(time.RFC3339Nano) + `","toolUseResult":{"task_id":"a","task_type":"local_agent","backgroundTaskId":"b"}}`,
			[]TaskEvent{{Kind: TaskAgent, ID: "a"}, {Kind: TaskShell, ID: "b", Launched: true, At: t0}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := TaskEvents([]byte(c.line)); !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %+v want %+v", got, c.want)
			}
		})
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
	decoded := []string{launchAgentLine("aaa"), launchShellLine("bone"), stopLine("aaa", "local_agent"), notificationLine("aaa", "completed")}
	for _, l := range decoded {
		if !mayCarryTask([]byte(l)) {
			t.Errorf("skipped: %s", l)
		}
	}
}

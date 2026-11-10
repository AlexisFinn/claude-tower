package locate

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAttachTarget(t *testing.T) {
	cases := []struct {
		argv []string
		id   string
		ok   bool
	}{
		{[]string{"claude", "attach", "504d303c"}, "504d303c", true},
		{[]string{"/home/u/.local/bin/claude", "--verbose", "attach", "ab12"}, "ab12", true},
		{[]string{"claude", "attach"}, "", false},
		{[]string{"claude", "attach", "--help"}, "", false},
		{[]string{"claude", "--resume", "x", "attach", "id"}, "", false},
		{[]string{"vim", "attach", "id"}, "", false},
		{[]string{"claude", "logs", "attach"}, "", false},
		{[]string{"/usr/bin/node", "/usr/lib/node_modules/@anthropic-ai/claude-code/cli.js", "attach", "ab12"}, "ab12", true},
		{[]string{"node", "/home/claude/app/server.js", "attach", "ab12"}, "", false},
		{nil, "", false},
		{[]string{"node", "server.js", "attach", "ab12"}, "", false},
	}
	for _, c := range cases {
		id, ok := attachTarget(c.argv)
		if id != c.id || ok != c.ok {
			t.Errorf("%v: %q %v", c.argv, id, ok)
		}
	}
}

func TestReadProcAttachments(t *testing.T) {
	dir := t.TempDir()
	write := func(pid, cmdline string) {
		if err := os.MkdirAll(filepath.Join(dir, pid), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, pid, "cmdline"), []byte(cmdline), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("10", "claude\x00attach\x00504d303c\x00")
	write("11", "claude\x00--resume\x00")
	write("12", "bash\x00")
	got, err := readProcAttachments(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, Attachments{10: "504d303c"}) {
		t.Errorf("got %v", got)
	}
}

func TestParsePSAttachments(t *testing.T) {
	out := "  10 claude attach 504d303c\n  11 /bin/zsh\nbroken\n  12 claude attach\n"
	if got := parsePSAttachments(out); !reflect.DeepEqual(got, Attachments{10: "504d303c"}) {
		t.Errorf("got %v", got)
	}
}

func TestReadAttachmentsLinuxReadsProcAndNotPS(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "10"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "10", "cmdline"), []byte("claude\x00attach\x00504d303c\x00"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(context.Context, io.Reader, string, ...string) ([]byte, error) {
		t.Error("ps run on linux")
		return nil, nil
	}
	got, err := readAttachments(context.Background(), run, "linux", dir)
	if err != nil || !reflect.DeepEqual(got, Attachments{10: "504d303c"}) {
		t.Errorf("got %v, %v", got, err)
	}
}

func TestReadAttachmentsOtherSystemsRunPS(t *testing.T) {
	var gotArgs []string
	run := func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)
		return []byte("  10 claude attach 504d303c\n"), nil
	}
	got, err := readAttachments(context.Background(), run, "darwin", t.TempDir())
	if err != nil || !reflect.DeepEqual(got, Attachments{10: "504d303c"}) {
		t.Errorf("got %v, %v", got, err)
	}
	if want := []string{"ps", "-A", "-o", "pid=,args="}; !reflect.DeepEqual(gotArgs, want) {
		t.Errorf("command: %v", gotArgs)
	}
	run = func(context.Context, io.Reader, string, ...string) ([]byte, error) {
		return nil, errors.New("ps unavailable")
	}
	if _, err := readAttachments(context.Background(), run, "darwin", t.TempDir()); err == nil {
		t.Error("ps error ignored")
	}
}

func TestReadAttachmentsOnThisSystem(t *testing.T) {
	run := func(context.Context, io.Reader, string, ...string) ([]byte, error) {
		return []byte("  10 claude attach 504d303c\n"), nil
	}
	if _, err := ReadAttachments(context.Background(), run); err != nil {
		t.Fatal(err)
	}
}

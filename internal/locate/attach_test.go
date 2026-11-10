package locate

import (
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
		{[]string{"/usr/bin/node", "/opt/claude/cli.js", "attach", "ab12"}, "ab12", true},
		{[]string{"node", "server.js", "attach", "ab12"}, "", false},
	}
	for _, c := range cases {
		id, ok := AttachTarget(c.argv)
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

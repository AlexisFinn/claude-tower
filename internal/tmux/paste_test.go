package tmux

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSanitize(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", "hello", "hello"},
		{"CRLF", "a\r\nb", "a\nb"},
		{"lone CR", "a\rb", "a\nb"},
		{"mixed endings", "a\r\n\rb\n", "a\n\nb\n"},
		{"tab kept", "a\tb", "a\tb"},
		{"ESC removed", "a\x1b[31mb\x1b[0m", "a[31mb[0m"},
		{"C0 removed", "a\x00\x01\x07\x08\x0b\x0c\x0e\x1fb", "ab"},
		{"DEL removed", "a\x7fb", "ab"},
		{"C1 removed", "a\u0085\u009bb", "ab"},
		{"unicode kept", "héllo — 日本語 🎉", "héllo — 日本語 🎉"},
		{"paste end marker neutralised", "x\x1b[201~y", "x[201~y"},
	}
	for _, c := range cases {
		if got := Sanitize(c.in); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestPasteCommandOrder(t *testing.T) {
	r := &recorder{}
	if err := r.server().Paste(context.Background(), "%7", "secret text\r\nline"); err != nil {
		t.Fatal(err)
	}
	cmds := r.commands()
	if len(cmds) != 4 {
		t.Fatalf("commands: %v", cmds)
	}
	buf := cmds[1][2]
	if !strings.HasPrefix(buf, "claude-tower-") {
		t.Errorf("buffer name %q", buf)
	}
	want := [][]string{
		{"if-shell", "-F", "-t", "%7", "#{pane_in_mode}", "send-keys -t %7 -X cancel"},
		{"load-buffer", "-b", buf, "-"},
		{"paste-buffer", "-p", "-d", "-b", buf, "-t", "%7"},
		{"send-keys", "-t", "%7", "Enter"},
	}
	if !reflect.DeepEqual(cmds, want) {
		t.Errorf("commands:\n got %v\nwant %v", cmds, want)
	}
	if r.calls[1].stdin != "secret text\nline" {
		t.Errorf("stdin: %q", r.calls[1].stdin)
	}
	for i, c := range r.calls {
		if i != 1 && c.stdin != "" {
			t.Errorf("call %d got stdin %q", i, c.stdin)
		}
		for _, a := range c.args {
			if strings.Contains(a, "secret") {
				t.Errorf("text leaked in arguments of call %d: %v", i, c.args)
			}
		}
	}
}

func TestPasteBuffersAreUnique(t *testing.T) {
	r := &recorder{}
	srv := r.server()
	srv.Paste(context.Background(), "%1", "a")
	srv.Paste(context.Background(), "%1", "b")
	if r.commands()[1][2] == r.commands()[5][2] {
		t.Error("buffer name reused")
	}
}

func TestPasteDeletesBufferWhenPasteFails(t *testing.T) {
	r := &recorder{reply: func(args []string) ([]byte, error) {
		if args[2] == "paste-buffer" {
			return nil, errors.New("no pane")
		}
		return nil, nil
	}}
	err := r.server().Paste(context.Background(), "%1", "x")
	if err == nil {
		t.Fatal("error swallowed")
	}
	cmds := r.commands()
	last := cmds[len(cmds)-1]
	if last[0] != "delete-buffer" || last[2] != cmds[1][2] {
		t.Errorf("last command: %v", last)
	}
	for _, c := range cmds {
		if c[0] == "send-keys" {
			t.Error("Enter sent after a failed paste")
		}
	}
}

func TestPasteRejectsEmptyText(t *testing.T) {
	for _, text := range []string{"", " \n\t", "\x1b\x07", "\r"} {
		r := &recorder{reply: func([]string) ([]byte, error) { return nil, nil }}
		err := r.server().Paste(context.Background(), "%1", text)
		if err == nil || !strings.Contains(err.Error(), "texte vide") {
			t.Errorf("%q: err=%v", text, err)
		}
		if n := len(r.commands()); n != 0 {
			t.Errorf("%q: tmux called: %v", text, r.commands())
		}
	}
}

func TestPaneLockKeyIncludesSocket(t *testing.T) {
	if paneLock("/s/a", "%1") == paneLock("/s/b", "%1") {
		t.Error("same lock for two sockets")
	}
	if paneLock("/s/a", "%1") != paneLock("/s/a", "%1") {
		t.Error("different locks for the same socket and pane")
	}
}

func TestPasteStopsOnLoadFailure(t *testing.T) {
	r := &recorder{reply: func(args []string) ([]byte, error) {
		if args[2] == "load-buffer" {
			return nil, errors.New("too long")
		}
		return nil, nil
	}}
	if err := r.server().Paste(context.Background(), "%1", "x"); err == nil {
		t.Fatal("error swallowed")
	}
	if n := len(r.commands()); n != 2 {
		t.Errorf("commands run after failure: %v", r.commands())
	}
}

func TestPasteCancelledDuringDelay(t *testing.T) {
	old := pasteDelay
	pasteDelay = time.Minute
	defer func() { pasteDelay = old }()
	ctx, cancel := context.WithCancel(context.Background())
	r := &recorder{reply: func(args []string) ([]byte, error) {
		if args[2] == "paste-buffer" {
			cancel()
		}
		return nil, nil
	}}
	if err := r.server().Paste(ctx, "%1", "x"); !errors.Is(err, context.Canceled) {
		t.Errorf("err=%v", err)
	}
}

func TestPasteSerialisesPerPane(t *testing.T) {
	run := &recorder{reply: func(args []string) ([]byte, error) {
		time.Sleep(2 * time.Millisecond)
		return nil, nil
	}}
	srv := run.server()
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			srv.Paste(context.Background(), "%1", "x")
		}()
	}
	wg.Wait()
	cmds := run.commands()
	if len(cmds) != 24 {
		t.Fatalf("commands: %d", len(cmds))
	}
	for i := 0; i < len(cmds); i += 4 {
		got := []string{cmds[i][0], cmds[i+1][0], cmds[i+2][0], cmds[i+3][0]}
		if got[0] != "if-shell" || got[1] != "load-buffer" || got[2] != "paste-buffer" || got[3] != "send-keys" {
			t.Fatalf("interleaved pastes at %d: %v", i, got)
		}
		if i > 0 && cmds[i+1][2] == cmds[i-3][2] {
			t.Errorf("two pastes share a buffer")
		}
		if cmds[i+1][2] != cmds[i+2][4] {
			t.Errorf("paste %d: buffer mismatch between load and paste", i/4)
		}
	}
}

func TestPasteDifferentPanesDoNotBlockEachOther(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	var first atomic.Bool
	r := &recorder{reply: func(args []string) ([]byte, error) {
		if args[2] == "load-buffer" && first.CompareAndSwap(false, true) {
			close(started)
			<-release
		}
		return nil, nil
	}}
	srv := r.server()
	done := make(chan struct{})
	go func() { srv.Paste(context.Background(), "%1", "a"); close(done) }()
	<-started
	finished := make(chan struct{})
	go func() { srv.Paste(context.Background(), "%2", "b"); close(finished) }()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Error("paste into another pane was blocked")
	}
	close(release)
	<-done
}

// pasteResult runs one paste against a real tmux pane that records its input
// and returns the recorded bytes.
func pasteResult(t *testing.T, before func(Server), text string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "recv")
	srv := testServer(t, "rawpaste", auxOutEnv+"="+out)
	waitFor(t, "auxiliary program", func() bool { return fileExists(out + ".ready") })
	panes, err := srv.Panes(context.Background())
	if err != nil || len(panes) != 1 {
		t.Fatalf("panes=%v err=%v", panes, err)
	}
	if before != nil {
		before(srv)
	}
	if err := srv.Paste(context.Background(), panes[0].ID, text); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "pasted bytes", func() bool { return strings.HasSuffix(readFile(out), "\x1b[201~\r") })
	return readFile(out)
}

func TestPasteIntegration(t *testing.T) {
	big := strings.Repeat("0123456789abcdefghij\n", 50*1024/21)
	cases := []struct{ name, text, want string }{
		{"single line", "hello world", "hello world"},
		{"multi line", "one\ntwo\r\nthree", "one\rtwo\rthree"},
		{"ESC neutralised", "a\x1b[31mred\x1b[0m\x1b[201~b", "a[31mred[0m[201~b"},
		{"50 KiB", big, strings.ReplaceAll(big, "\n", "\r")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pasteResult(t, nil, c.text)
			if want := "\x1b[200~" + c.want + "\x1b[201~\r"; got != want {
				t.Errorf("got %d bytes, want %d\n got %q\nwant %q", len(got), len(want), truncate(got), truncate(want))
			}
		})
	}
}

func TestPasteIntegrationCopyMode(t *testing.T) {
	got := pasteResult(t, func(s Server) {
		mustExec(t, s, "copy-mode", "-t", "main:")
		if mustExec(t, s, "display-message", "-p", "-t", "main:", "#{pane_in_mode}") != "1\n" {
			t.Fatal("pane is not in copy mode")
		}
	}, "line1\nline2")
	if want := "\x1b[200~line1\rline2\x1b[201~\r"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func truncate(s string) string {
	if len(s) > 120 {
		return s[:60] + "..." + s[len(s)-60:]
	}
	return s
}

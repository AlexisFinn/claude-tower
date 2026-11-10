package tmux

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFindBinary(t *testing.T) {
	look := func(found ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			for _, f := range found {
				if f == name {
					return name, nil
				}
			}
			return "", errors.New("not found")
		}
	}
	if got, err := FindBinary(look("tmux", "/usr/bin/tmux")); err != nil || got != "tmux" {
		t.Errorf("PATH first: got %q %v", got, err)
	}
	if got, err := FindBinary(look("/usr/local/bin/tmux", "/usr/bin/tmux")); err != nil || got != "/usr/local/bin/tmux" {
		t.Errorf("fallback order: got %q %v", got, err)
	}
	if got, err := FindBinary(look("/opt/homebrew/bin/tmux")); err != nil || got != "/opt/homebrew/bin/tmux" {
		t.Errorf("homebrew: got %q %v", got, err)
	}
	if _, err := FindBinary(look()); err == nil || !strings.Contains(err.Error(), "3.2") {
		t.Errorf("missing binary: %v", err)
	}
}

func TestCheckVersion(t *testing.T) {
	cases := []struct {
		out string
		ok  bool
	}{
		{"tmux 3.0a\n", false},
		{"tmux 3.1\n", false},
		{"tmux 2.9\n", false},
		{"tmux 3.2\n", true},
		{"tmux 3.2a\n", true},
		{"tmux 3.3a\n", true},
		{"tmux 3.8\n", true},
		{"tmux 10.0\n", true},
		{"tmux next-3.9\n", true},
		{"tmux next-3.1\n", false},
		{"tmux master\n", true},
		{"tmux unknown\n", false},
		{"", false},
	}
	for _, c := range cases {
		run := func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
			if name != "/bin/tmux" || !reflect.DeepEqual(args, []string{"-V"}) {
				t.Errorf("unexpected command %s %v", name, args)
			}
			return []byte(c.out), nil
		}
		if err := CheckVersion(context.Background(), "/bin/tmux", run); (err == nil) != c.ok {
			t.Errorf("%q: err=%v want ok=%v", c.out, err, c.ok)
		}
	}
	failing := func(context.Context, io.Reader, string, ...string) ([]byte, error) { return nil, errors.New("boom") }
	if err := CheckVersion(context.Background(), "tmux", failing); err == nil {
		t.Error("runner error ignored")
	}
}

func TestExecPrependsSocket(t *testing.T) {
	r := &recorder{}
	r.server().Exec(context.Background(), nil, "list-panes", "-a")
	if got := r.calls[0].args; !reflect.DeepEqual(got, []string{"-S", "/s/sock", "list-panes", "-a"}) {
		t.Errorf("args: %v", got)
	}
}

func TestExecRunnerIncludesStderr(t *testing.T) {
	_, err := ExecRunner(defaultTimeout)(context.Background(), nil, "sh", "-c", "echo oops >&2; exit 3")
	if err == nil || !strings.Contains(err.Error(), "oops") {
		t.Errorf("stderr missing from %v", err)
	}
}

func TestExecRunnerTimeout(t *testing.T) {
	_, err := ExecRunner(50*time.Millisecond)(context.Background(), nil, "sleep", "5")
	if err == nil {
		t.Error("timeout not enforced")
	}
}

func TestExecRunnerTimeoutWithLingeringChild(t *testing.T) {
	start := time.Now()
	_, err := ExecRunner(200*time.Millisecond)(context.Background(), nil, "sh", "-c", "sleep 3 & wait")
	if err == nil {
		t.Fatal("timeout not enforced")
	}
	if d := time.Since(start); d > 1500*time.Millisecond {
		t.Errorf("returned after %v", d)
	}
	if !strings.Contains(err.Error(), "délai") {
		t.Errorf("error does not mention the timeout: %v", err)
	}
}

func TestExecRunnerKeepsOutputWhenChildHoldsPipe(t *testing.T) {
	out, err := ExecRunner(defaultTimeout)(context.Background(), nil, "sh", "-c", "echo hello; sleep 1 &")
	if !errors.Is(err, exec.ErrWaitDelay) || string(out) != "hello\n" {
		t.Errorf("out=%q err=%v", out, err)
	}
}

func TestExecRunnerStdin(t *testing.T) {
	out, err := ExecRunner(defaultTimeout)(context.Background(), strings.NewReader("hello"), "cat")
	if err != nil || string(out) != "hello" {
		t.Errorf("out=%q err=%v", out, err)
	}
}

const paneLines = "%1\t100\t$2\tclaude-tower-view-1-1\t@1\t1\t\t1700000000\t/w\n" +
	"%1\t100\t$1\tmain\t@1\t1\t\t1700000000\t/w\n" +
	"%2\t101\t$1\tmain\t@1\t0\t\t1700000000\t/w\n" +
	"%3\t102\t$3\tg1\t@3\t1\tgrp\t1700000000\t/w\tab\n" +
	"%3\t102\t$4\tg2\t@3\t1\tgrp\t1700000000\t/w\tab\n" +
	"broken line\n" +
	"%9\tnotapid\t$1\tmain\t@1\t1\t\t1700000000\t/w\n"

func TestPanesParsesExcludesViewsAndDeduplicates(t *testing.T) {
	r := &recorder{reply: func([]string) ([]byte, error) { return []byte(paneLines), nil }}
	srv := r.server()
	panes, err := srv.Panes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := r.commands()[0]; !reflect.DeepEqual(got[:3], []string{"list-panes", "-a", "-F"}) {
		t.Errorf("command: %v", got)
	}
	ids := []string{}
	for _, p := range panes {
		ids = append(ids, p.ID)
	}
	if !reflect.DeepEqual(ids, []string{"%1", "%2", "%3"}) {
		t.Fatalf("ids: %v", ids)
	}
	p1, p3 := panes[0], panes[2]
	if p1.PID != 100 || p1.SessionID != "$1" || p1.SessionName != "main" || p1.WindowID != "@1" || !p1.Active ||
		p1.SessionGroup != "" || p1.Server.Socket != "/s/sock" {
		t.Errorf("pane %%1: %+v", p1)
	}
	if panes[1].Active {
		t.Error("%2 must be inactive")
	}
	if !p1.Created.Equal(time.Unix(1700000000, 0)) || p1.Cwd != "/w" {
		t.Errorf("pane %%1 created %v cwd %q", p1.Created, p1.Cwd)
	}
	if p3.SessionID != "$3" || p3.SessionGroup != "grp" || p3.Cwd != "/w\tab" {
		t.Errorf("pane %%3: %+v", p3)
	}
}

func TestPanesError(t *testing.T) {
	r := &recorder{reply: func([]string) ([]byte, error) { return nil, errors.New("no server") }}
	if _, err := r.server().Panes(context.Background()); err == nil {
		t.Error("error swallowed")
	}
}

func TestClientsParsesAndExcludesViews(t *testing.T) {
	out := "/dev/pts/1\t500\t$1\tmain\t\t1700000000\n" +
		"/dev/pts/2\t501\t$2\tclaude-tower-view-1-1\tgrp\t1700000005\n" +
		"/dev/pts/3\t502\t$3\tg1\tgrp\t1700000009\n" +
		"junk\n"
	r := &recorder{reply: func([]string) ([]byte, error) { return []byte(out), nil }}
	clients, err := r.server().Clients(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []Client{
		{Name: "/dev/pts/1", PID: 500, SessionID: "$1", Activity: 1700000000},
		{Name: "/dev/pts/3", PID: 502, SessionID: "$3", SessionGroup: "grp", Activity: 1700000009},
	}
	if !reflect.DeepEqual(clients, want) {
		t.Errorf("got %+v", clients)
	}
	if got := r.commands()[0]; got[0] != "list-clients" {
		t.Errorf("command: %v", got)
	}
}

func TestPaneAttached(t *testing.T) {
	clients := []Client{
		{Name: "a", SessionID: "$1"},
		{Name: "b", SessionID: "$9", SessionGroup: "grp"},
		{Name: "c", SessionID: "$8", SessionGroup: ""},
	}
	cases := []struct {
		name string
		pane Pane
		want []string
	}{
		{"same session", Pane{SessionID: "$1"}, []string{"a"}},
		{"same group", Pane{SessionID: "$3", SessionGroup: "grp"}, []string{"b"}},
		{"ungrouped pane does not match empty groups", Pane{SessionID: "$7"}, nil},
		{"no client", Pane{SessionID: "$7", SessionGroup: "other"}, nil},
	}
	for _, c := range cases {
		var got []string
		for _, cl := range c.pane.Clients(clients) {
			got = append(got, cl.Name)
		}
		if !reflect.DeepEqual(got, c.want) || c.pane.Attached(clients) != (len(c.want) > 0) {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestCaptureAndSwitchClient(t *testing.T) {
	r := &recorder{reply: func([]string) ([]byte, error) { return []byte("line1\nline2\n100\t30\n"), nil }}
	srv := r.server()
	got, w, h, err := srv.Capture(context.Background(), "%4")
	if err != nil || got != "line1\nline2\n" || w != 100 || h != 30 {
		t.Errorf("capture: %q %dx%d %v", got, w, h, err)
	}
	if err := srv.SwitchClient(context.Background(), "/dev/pts/1", "%4"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"capture-pane", "-p", "-e", "-t", "%4", ";", "display-message", "-p", "-t", "%4", "#{pane_width}\t#{pane_height}"},
		{"switch-client", "-c", "/dev/pts/1", "-t", "%4"},
	}
	if !reflect.DeepEqual(r.commands(), want) {
		t.Errorf("commands: %v", r.commands())
	}
}

func TestParseCapture(t *testing.T) {
	cases := []struct {
		name, out, text string
		w, h            int
		wantErr         bool
	}{
		{"text then size", "a\nb\n80\t24\n", "a\nb\n", 80, 24, false},
		{"text that looks like a size", "1\t2\n80\t24\n", "1\t2\n", 80, 24, false},
		{"size only", "80\t24\n", "", 80, 24, false},
		{"empty", "", "", 0, 0, true},
		{"no tab", "a\n80 24\n", "", 0, 0, true},
		{"not numbers", "a\nx\ty\n", "", 0, 0, true},
	}
	for _, c := range cases {
		text, w, h, err := parseCapture(c.out)
		if (err != nil) != c.wantErr || text != c.text || w != c.w || h != c.h {
			t.Errorf("%s: got %q %dx%d err=%v", c.name, text, w, h, err)
		}
	}
}

func TestCaptureReturnsSizeOfTheLivePane(t *testing.T) {
	srv := testServer(t, "sleep")
	out := strings.TrimSpace(mustExec(t, srv, "display-message", "-p", "-t", "main:", "#{pane_id}"))
	text, w, h, err := srv.Capture(context.Background(), out)
	if err != nil || w != 80 || h != 24 || strings.Count(text, "\n") != 24 {
		t.Fatalf("w=%d h=%d lines=%d err=%v", w, h, strings.Count(text, "\n"), err)
	}
	mustExec(t, srv, "resize-window", "-t", "main:", "-x", "100", "-y", "30")
	text, w, h, err = srv.Capture(context.Background(), out)
	if err != nil || w != 100 || h != 30 || strings.Count(text, "\n") != 30 {
		t.Errorf("after resize: w=%d h=%d lines=%d err=%v", w, h, strings.Count(text, "\n"), err)
	}
}

func TestCandidateServersIncludePrivateSocket(t *testing.T) {
	userDir, err := os.MkdirTemp("/tmp", "ct")
	if err != nil {
		t.Fatal(err)
	}
	privDir, err := os.MkdirTemp("/tmp", "ct")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(userDir); os.RemoveAll(privDir) })
	for _, p := range []string{filepath.Join(userDir, "default"), filepath.Join(privDir, "tmux.sock")} {
		l, err := net.Listen("unix", p)
		if err != nil {
			t.Skipf("unix sockets unavailable: %v", err)
		}
		t.Cleanup(func() { l.Close() })
	}
	os.WriteFile(filepath.Join(privDir, "env-123"), nil, 0o600)
	var got []string
	for _, s := range CandidateServers(userDir, filepath.Join(privDir, "tmux.sock"), "tmux", nil) {
		got = append(got, s.Socket)
	}
	want := []string{filepath.Join(userDir, "default"), filepath.Join(privDir, "tmux.sock")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	if got := CandidateServers(userDir, "", "tmux", nil); len(got) != 1 {
		t.Errorf("without private socket: %v", got)
	}
}

func TestSocketDir(t *testing.T) {
	if got := SocketDir("", 1000); got != "/tmp/tmux-1000" {
		t.Errorf("default: %q", got)
	}
	if got := SocketDir("/run/x", 7); got != "/run/x/tmux-7" {
		t.Errorf("custom: %q", got)
	}
}

func TestServersListsOnlySockets(t *testing.T) {
	dir, err := os.MkdirTemp("/tmp", "ct")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "default")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	ln.Close()
	if err := os.WriteFile(filepath.Join(dir, "regular"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	servers := Servers(dir, "tmux", nil)
	if len(servers) != 1 || servers[0].Socket != sock || servers[0].Bin != "tmux" {
		t.Fatalf("servers: %+v", servers)
	}
	if Servers(filepath.Join(dir, "missing"), "tmux", nil) != nil {
		t.Error("missing dir must yield no server")
	}
}

func TestDeadSocketIsIgnoredByCallers(t *testing.T) {
	bin := requireTmux(t)
	dir, err := os.MkdirTemp("/tmp", "ct")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	ln, err := net.Listen("unix", filepath.Join(dir, "dead"))
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	ln.Close()
	servers := Servers(dir, bin, nil)
	if len(servers) != 1 {
		t.Fatalf("servers: %+v", servers)
	}
	if panes, err := servers[0].Panes(context.Background()); err == nil || panes != nil {
		t.Errorf("dead socket: panes=%v err=%v", panes, err)
	}
}

func TestServerNotRunning(t *testing.T) {
	for msg, want := range map[string]bool{
		"tmux: exit status 1: no server running on /s":                     true,
		"tmux: exit status 1: error connecting to /s (Connection refused)": true,
		"tmux: délai de 2s dépassé: context deadline exceeded":             false,
	} {
		if got := ServerNotRunning(errors.New(msg)); got != want {
			t.Errorf("%q: %v", msg, got)
		}
	}
	if ServerNotRunning(nil) {
		t.Error("nil error")
	}
}

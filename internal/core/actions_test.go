package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"claude-tower/internal/locate"
	"claude-tower/internal/tmux"
)

// promptEnv has one idle session in the user's tmux, hosted by pane %1.
func promptEnv(t *testing.T) (*env, *Core) {
	t.Helper()
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"idle"`)
	e.tree = locate.Tree{100: 10, 10: 1}
	e.fake.panes[e.userSock] = paneLine("%1", 10, "$1", "a", "@1", "")
	c := e.core()
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	return e, c
}

func TestPromptPastes(t *testing.T) {
	e, c := promptEnv(t)
	if err := c.Prompt(context.Background(), 100, "hello\nworld"); err != nil {
		t.Fatal(err)
	}
	var words []string
	for _, cmd := range e.fake.commands(e.userSock) {
		words = append(words, cmd[0])
	}
	want := []string{"list-panes", "list-clients", "list-panes", "if-shell", "load-buffer", "paste-buffer", "send-keys"}
	if !reflect.DeepEqual(words, want) {
		t.Errorf("commands: %v want %v", words, want)
	}
	if got := e.fake.stdinOf("load-buffer"); got != "hello\nworld" {
		t.Errorf("stdin of load-buffer: %q", got)
	}
}

func TestPromptRefusedWhenStatusChangedAfterSnapshot(t *testing.T) {
	e, c := promptEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"waiting","waitingFor":"permission prompt"`)
	err := c.Prompt(context.Background(), 100, "go")
	if err == nil || !strings.Contains(err.Error(), "attend une autorisation") {
		t.Fatalf("err=%v", err)
	}
	for _, cmd := range e.fake.commands(e.userSock) {
		if cmd[0] == "paste-buffer" || cmd[0] == "load-buffer" {
			t.Errorf("text pasted: %v", cmd)
		}
	}
}

func TestPromptRefusals(t *testing.T) {
	e, c := promptEnv(t)
	ctx := context.Background()
	if err := c.Prompt(ctx, 100, " \n\t "); err == nil {
		t.Error("blank text accepted")
	}
	if err := c.Prompt(ctx, 999, "go"); err == nil {
		t.Error("unknown session accepted")
	}
	e.alive[100] = false
	if err := c.Prompt(ctx, 100, "go"); err == nil || !strings.Contains(err.Error(), "n'existe plus") {
		t.Errorf("dead session: %v", err)
	}
	e.alive[100] = true
	e.fake.panes[e.userSock] = paneLine("%9", 77, "$1", "a", "@1", "")
	if err := c.Prompt(ctx, 100, "go"); err == nil || !strings.Contains(err.Error(), "volet") {
		t.Errorf("vanished pane: %v", err)
	}
	for _, cmd := range e.fake.commands(e.userSock) {
		if cmd[0] == "paste-buffer" {
			t.Error("text pasted")
		}
	}
}

func TestPlanFocus(t *testing.T) {
	older := tmux.Client{Name: "a", PID: 1, Activity: 10}
	newer := tmux.Client{Name: "b", PID: 2, Activity: 20}
	cases := []struct {
		name      string
		inTmux    bool
		clients   []tmux.Client
		supported bool
		want      focusPlan
	}{
		{"outside tmux", false, nil, true, focusPlan{kind: focusRaiseOwner}},
		{"outside tmux unsupported", false, nil, false, focusPlan{kind: focusRaiseOwner}},
		{"no client", true, nil, true, focusPlan{kind: focusEmbedded}},
		{"unsupported", true, []tmux.Client{older}, false, focusPlan{kind: focusEmbedded}},
		{"most recent client", true, []tmux.Client{older, newer, older}, true, focusPlan{kind: focusSwitch, client: newer}},
		{"single client", true, []tmux.Client{older}, true, focusPlan{kind: focusSwitch, client: older}},
	}
	for _, c := range cases {
		if got := planFocus(c.inTmux, c.clients, c.supported); got != c.want {
			t.Errorf("%s: got %+v want %+v", c.name, got, c.want)
		}
	}
}

// focusEnv has a session in the user's tmux shown by two clients, and a
// session outside tmux.
func focusEnv(t *testing.T) (*env, *Core) {
	t.Helper()
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"idle"`)
	e.session(200, `"sessionId":"`+idB+`","startedAt":2000,"status":"idle"`)
	e.tree = locate.Tree{100: 10, 10: 1, 200: 20, 20: 1, 500: 50, 50: 5, 5: 1, 600: 60, 60: 6, 6: 1}
	e.fake.panes[e.userSock] = paneLine("%1", 10, "$1", "a", "@1", "")
	e.fake.clients[e.userSock] = clientLine("/dev/pts/1", 500, "$1", "a", "", 10) + clientLine("/dev/pts/2", 600, "$1", "a", "", 99)
	c := e.core()
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	return e, c
}

func switchCalls(e *env) [][]string {
	var out [][]string
	for _, cmd := range e.fake.commands(e.userSock) {
		if cmd[0] == "switch-client" {
			out = append(out, cmd)
		}
	}
	return out
}

func TestFocusRaisesMostRecentClientThenSwitches(t *testing.T) {
	e, c := focusEnv(t)
	r, err := c.Focus(context.Background(), 100)
	if err != nil || r != (FocusResult{Mode: "external"}) {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	if !reflect.DeepEqual(e.raised, [][]int{{600, 60, 6}}) {
		t.Errorf("raised %v", e.raised)
	}
	if got := switchCalls(e); !reflect.DeepEqual(got, [][]string{{"switch-client", "-c", "/dev/pts/2", "-t", "%1"}}) {
		t.Errorf("switch calls: %v", got)
	}
}

func TestFocusFallsBackToEmbeddedWhenRaiseFails(t *testing.T) {
	e, c := focusEnv(t)
	e.raiseErr = errors.New("aucune fenêtre trouvée")
	r, err := c.Focus(context.Background(), 100)
	if err != nil || r.Mode != "embedded" || r.Warning != "aucune fenêtre trouvée" {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	if len(switchCalls(e)) != 0 {
		t.Error("a client was switched although the window was not raised")
	}
}

func TestFocusEmbeddedWithoutClientOrSupport(t *testing.T) {
	e, c := focusEnv(t)
	e.supports = false
	if r, _ := c.Focus(context.Background(), 100); r != (FocusResult{Mode: "embedded"}) {
		t.Errorf("unsupported: %+v", r)
	}
	e.supports = true
	e.fake.clients[e.userSock] = ""
	c.Cards(context.Background())
	if r, _ := c.Focus(context.Background(), 100); r != (FocusResult{Mode: "embedded"}) {
		t.Errorf("no client: %+v", r)
	}
	if len(e.raised) != 0 || len(switchCalls(e)) != 0 {
		t.Errorf("raised=%v switches=%v", e.raised, switchCalls(e))
	}
}

func TestFocusOutsideTmux(t *testing.T) {
	e, c := focusEnv(t)
	r, err := c.Focus(context.Background(), 200)
	if err != nil || r != (FocusResult{Mode: "external"}) {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	if !reflect.DeepEqual(e.raised, [][]int{{200, 20}}) {
		t.Errorf("raised %v", e.raised)
	}
	e.raiseErr = errors.New("non pris en charge")
	if _, err := c.Focus(context.Background(), 200); err == nil || !strings.Contains(err.Error(), "impossible d'afficher le terminal de cette session") {
		t.Errorf("err=%v", err)
	}
	if _, err := c.Focus(context.Background(), 12345); err == nil {
		t.Error("unknown session accepted")
	}
}

func TestFocusUnknownSessionIsRefusedBeforeAnyRaise(t *testing.T) {
	e, c := focusEnv(t)
	_, err := c.Focus(context.Background(), 12345)
	if err == nil || !strings.Contains(err.Error(), "session inconnue") {
		t.Errorf("err=%v", err)
	}
	if len(e.raised) != 0 || len(switchCalls(e)) != 0 {
		t.Errorf("raised=%v switches=%v", e.raised, switchCalls(e))
	}
}

func TestFocusIgnoresClientsOfOtherSessions(t *testing.T) {
	e, c := focusEnv(t)
	e.tree[700], e.tree[70], e.tree[7] = 70, 7, 1
	e.fake.clients[e.userSock] += clientLine("/dev/pts/7", 700, "$9", "other", "", 5000)
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r, err := c.Focus(context.Background(), 100); err != nil || r.Mode != "external" {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	if !reflect.DeepEqual(e.raised, [][]int{{600, 60, 6}}) {
		t.Errorf("raised %v", e.raised)
	}
	if got := switchCalls(e); !reflect.DeepEqual(got, [][]string{{"switch-client", "-c", "/dev/pts/2", "-t", "%1"}}) {
		t.Errorf("switch calls: %v", got)
	}
}

func TestKillOnlyPrivateSessions(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"idle"`)
	e.session(200, `"sessionId":"`+idB+`","startedAt":2000,"status":"idle"`)
	e.tree = locate.Tree{100: 10, 10: 1, 200: 20, 20: 1}
	e.fake.panes[e.userSock] = paneLine("%1", 10, "$1", "a", "@1", "")
	e.fake.panes[e.privSock] = paneLine("%2", 20, "$7", "b", "@7", "")
	c := e.core()
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Kill(context.Background(), 100); err == nil {
		t.Error("session of the user's tmux killed")
	}
	if e.fake.has(e.userSock, "kill-session") {
		t.Error("kill-session sent to the user's server")
	}
	if err := c.Kill(context.Background(), 200); err != nil {
		t.Fatal(err)
	}
	if !e.fake.has(e.privSock, "kill-session") {
		t.Error("private session not killed")
	}
	if err := c.Kill(context.Background(), 999); err == nil {
		t.Error("unknown session accepted")
	}
}

// prepareClaude installs a fake claude command in the configuration of the
// private server and returns its path.
func prepareClaude(t *testing.T, e *env) string {
	t.Helper()
	claude := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(claude, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(e.hosting.ConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.hosting.ConfigDir, "config.json"), []byte(`{"claudeCommand":["`+claude+`"]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return claude
}

// startSession starts a session through the Core and returns the content of
// the environment file given to exec-env.
func startSession(t *testing.T, e *env) string {
	t.Helper()
	claude := prepareClaude(t, e)
	e.fake.panes[e.privSock] = paneLine("%5", 77, "$3", "proj", "@3", "")
	if err := e.core().NewSession(context.Background(), "/work/proj", ""); err != nil {
		t.Fatal(err)
	}
	var command []string
	for _, cmd := range e.fake.commands(e.privSock) {
		if cmd[0] == "-f" {
			command = cmd
		}
	}
	i := indexOf(command, "exec-env")
	if i < 0 {
		t.Fatalf("no session started: %v", e.fake.calls)
	}
	if command[i+3] != claude {
		t.Errorf("command: %v", command)
	}
	data, err := os.ReadFile(command[i+1])
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestNewSessionUsesApplicationEnvironment(t *testing.T) {
	e := newEnv(t)
	e.hosting.Env = []string{"TERM=xterm", "TMUX=/s,1,0", "KEEP=1", "LANG=fr_FR.UTF-8", "CLAUDE_CONFIG_DIR=/caller"}
	got := startSession(t, e)
	if want := "KEEP=1\x00LANG=fr_FR.UTF-8\x00CLAUDE_CONFIG_DIR=/app/claude\x00"; got != want {
		t.Errorf("environment file %q want %q", got, want)
	}
	if e.fake.shells != 0 {
		t.Error("login shell used although the environment has a TERM")
	}
}

func TestNewSessionKeepsClaudeDefaultConfigDir(t *testing.T) {
	e := newEnv(t)
	e.hosting.SessionConfigDir = ""
	e.hosting.Env = []string{"TERM=xterm", "KEEP=1", "LANG=fr_FR.UTF-8"}
	if got := startSession(t, e); strings.Contains(got, "CLAUDE_CONFIG_DIR") {
		t.Errorf("CLAUDE_CONFIG_DIR set without configuration: %q", got)
	}
}

func TestNewSessionCompletesEnvironmentWithLoginShellWhenNoTerminal(t *testing.T) {
	e := newEnv(t)
	e.hosting.Env = []string{"KEEP=1", "PATH=/app/bin", "LANG=C", "TMUX=/s,1,0"}
	e.fake.shell = "PATH=/login/bin\x00LOGIN=1\x00LANG=en_US.UTF-8\x00"
	got := strings.Split(strings.TrimSuffix(startSession(t, e), "\x00"), "\x00")
	if e.fake.shells != 1 {
		t.Errorf("login shell runs: %d", e.fake.shells)
	}
	for _, want := range []string{"KEEP=1", "LOGIN=1", "PATH=/login/bin:/app/bin", "CLAUDE_CONFIG_DIR=/app/claude"} {
		if indexOf(got, want) < 0 {
			t.Errorf("%q missing from %q", want, got)
		}
	}
	for _, e := range got {
		if strings.HasPrefix(e, "TMUX=") {
			t.Errorf("terminal variable kept: %q", got)
		}
	}
}

func TestNewSessionForcesUTF8Locale(t *testing.T) {
	e := newEnv(t)
	e.hosting.Env = []string{"TERM=xterm", "LC_ALL=C", "LANG=C"}
	got := strings.Split(strings.TrimSuffix(startSession(t, e), "\x00"), "\x00")
	locale := func(name string) string {
		for _, e := range got {
			if v, ok := strings.CutPrefix(e, name+"="); ok {
				return v
			}
		}
		return ""
	}
	if !strings.Contains(locale("LC_ALL"), "UTF-8") || !strings.Contains(locale("LANG"), "UTF-8") {
		t.Errorf("locale not UTF-8: %q", got)
	}
}

func indexOf(list []string, want string) int {
	for i, s := range list {
		if s == want {
			return i
		}
	}
	return -1
}

func TestNewSessionNeedsHosting(t *testing.T) {
	e := newEnv(t)
	c := e.core(func(o *Options) { o.Hosting = nil })
	if err := c.NewSession(context.Background(), "/x", ""); err == nil || !strings.Contains(err.Error(), "hébergement") {
		t.Errorf("err=%v", err)
	}
}

func TestKillNeedsHosting(t *testing.T) {
	e := newEnv(t)
	c := e.core(func(o *Options) { o.Hosting = nil })
	if err := c.Kill(context.Background(), 1); err == nil || !strings.Contains(err.Error(), "hébergement") {
		t.Errorf("err=%v", err)
	}
	if got := c.privateSocket(); got != "" {
		t.Errorf("private socket %q", got)
	}
}

func TestKillRefusesSessionOutsidePrivateServer(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"idle"`)
	e.tree = locate.Tree{100: 10, 10: 1}
	e.fake.panes[e.userSock] = paneLine("%1", 10, "$1", "a", "@1", "")
	e.fake.listing[e.userSock] = "$1\ta\t\n"
	c := e.core()
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	err := c.Kill(context.Background(), 100)
	if err == nil || !strings.Contains(err.Error(), "lancées par claude-tower") {
		t.Errorf("err=%v", err)
	}
	for _, sock := range []string{e.userSock, e.privSock} {
		if e.fake.has(sock, "kill-session") || e.fake.has(sock, "list-sessions") {
			t.Errorf("session commands sent to %s: %v", sock, e.fake.commands(sock))
		}
	}
}

func TestKillStopsViewSessionsCreatedAfterTheSnapshot(t *testing.T) {
	e := newEnv(t)
	e.session(200, `"sessionId":"`+idB+`","startedAt":2000,"status":"idle"`)
	e.tree = locate.Tree{200: 20, 20: 1}
	e.fake.panes[e.privSock] = paneLine("%2", 20, "$7", "b", "@7", "")
	c := e.core()
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	if p, _ := c.PaneOf(200); p.SessionGroup != "" {
		t.Fatalf("session grouped in the snapshot: %+v", p)
	}
	e.fake.listing[e.privSock] = "$7\tb\tgrp\n$20\tclaude-tower-view-1-1\tgrp\n$21\tclaude-tower-view-1-2\tother\n"
	if err := c.Kill(context.Background(), 200); err != nil {
		t.Fatal(err)
	}
	var kills [][]string
	for _, cmd := range e.fake.commands(e.privSock) {
		if cmd[0] == "kill-session" {
			kills = append(kills, cmd)
		}
	}
	want := [][]string{{"kill-session", "-t", "$20"}, {"kill-session", "-t", "$7"}}
	if !reflect.DeepEqual(kills, want) {
		t.Errorf("kills %v want %v", kills, want)
	}
}

func TestNewSessionNameBecomesNameArgument(t *testing.T) {
	tests := []struct {
		name string
		want []string
	}{
		{"", nil},
		{" \t ", nil},
		{`mon "nom" d'essai`, []string{"--name=" + `mon "nom" d'essai`}},
		{"  avec espaces\n", []string{"--name=" + "avec espaces"}},
		{strings.Repeat("x", 80), []string{"--name=" + strings.Repeat("x", 60)}},
	}
	for _, tt := range tests {
		e := newEnv(t)
		claude := prepareClaude(t, e)
		e.fake.panes[e.privSock] = paneLine("%5", 77, "$3", "proj", "@3", "")
		if err := e.core().NewSession(context.Background(), "/work/proj", tt.name); err != nil {
			t.Fatal(err)
		}
		var command []string
		for _, cmd := range e.fake.commands(e.privSock) {
			if cmd[0] == "-f" {
				command = cmd
			}
		}
		i := indexOf(command, "exec-env")
		if i < 0 {
			t.Fatalf("%q: no session started: %v", tt.name, e.fake.calls)
		}
		if got := command[i+3:]; !reflect.DeepEqual(got, append([]string{claude}, tt.want...)) {
			t.Errorf("%q: command %q", tt.name, got)
		}
	}
}

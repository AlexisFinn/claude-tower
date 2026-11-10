package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"claude-tower/internal/locate"
	"claude-tower/internal/registry"
	"claude-tower/internal/tmux"
)

func TestNewChecksTmuxVersion(t *testing.T) {
	e := newEnv(t)
	e.fake.version = "3.1"
	_, err := New(Options{Bin: "tmux", Run: e.fake.run})
	if err == nil || !strings.Contains(err.Error(), "trop ancien") {
		t.Errorf("err=%v", err)
	}
	e.fake.version = "3.8"
	if _, err := New(Options{Bin: "tmux", Run: e.fake.run}); err != nil {
		t.Errorf("recent tmux refused: %v", err)
	}
}

func TestCardsOrderAndFields(t *testing.T) {
	e := newEnv(t)
	e.session(300, `"sessionId":"`+idC+`","name":"gamma","cwd":"/home/u/gamma","startedAt":2000,"status":"busy","statusUpdatedAt":1794222000000`)
	e.session(100, `"sessionId":"`+idA+`","name":"alpha","cwd":"/srv/alpha","startedAt":3000,"status":"idle"`)
	e.session(200, `"sessionId":"`+idB+`","name":"beta","startedAt":2000,"status":"waiting","waitingFor":"dialog open"`)
	e.transcript(idC, `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"make"}}]}}`)
	// 200 and 300 share a start time (ordered by PID); 100 started last.
	e.tree = locate.Tree{300: 30, 30: 1, 200: 20, 20: 1, 100: 10, 10: 1}
	e.fake.panes[e.userSock] = paneLine("%1", 30, "$1", "main", "@1", "")
	e.fake.clients[e.userSock] = clientLine("/dev/pts/3", 999, "$1", "main", "", 5)
	e.fake.panes[e.privSock] = paneLine("%2", 10, "$7", "alpha", "@7", "")

	cards, err := e.core().Cards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 3 {
		t.Fatalf("cards: %+v", cards)
	}
	var pids, orders []int
	for _, c := range cards {
		pids = append(pids, c.PID)
		orders = append(orders, c.Order)
	}
	if !reflect.DeepEqual(pids, []int{200, 300, 100}) || !reflect.DeepEqual(orders, []int{0, 1, 2}) {
		t.Errorf("order: pids=%v orders=%v", pids, orders)
	}
	beta, gamma, alpha := cards[0], cards[1], cards[2]
	if beta.State != "waiting" || beta.InTmux || beta.CanPrompt || beta.PromptReason != "session hors tmux" {
		t.Errorf("beta: %+v", beta)
	}
	if gamma.State != "busy" || !gamma.InTmux || gamma.Private || !gamma.Attached || !gamma.CanPrompt ||
		gamma.Activity != "Bash: make" || gamma.Cwd != "~/gamma" || gamma.Label != "travaille" {
		t.Errorf("gamma: %+v", gamma)
	}
	if alpha.State != "idle" || !alpha.InTmux || !alpha.Private || alpha.Attached || !alpha.CanPrompt || alpha.Label != "fini" {
		t.Errorf("alpha: %+v", alpha)
	}
}

func TestCardNameIsTheFullName(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","name":"a-session-name-longer-than-sixteen","startedAt":1000,"status":"idle"`)
	e.session(200, `"sessionId":"`+idB+`","cwd":"/home/u/proj","startedAt":2000,"status":"idle"`)
	cards, err := e.core().Cards(context.Background())
	if err != nil || len(cards) != 2 {
		t.Fatalf("cards=%+v err=%v", cards, err)
	}
	if cards[0].Name != "a-session-name-longer-than-sixteen" || cards[1].Name != "proj" {
		t.Errorf("names: %q %q", cards[0].Name, cards[1].Name)
	}
}

func TestCardsIgnoreFailingServerAndKeepStableOrder(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","name":"alpha","startedAt":1000,"status":"idle"`)
	e.tree = locate.Tree{100: 10, 10: 1}
	e.fake.failing[e.userSock] = true
	e.fake.panes[e.privSock] = paneLine("%2", 10, "$7", "alpha", "@7", "")
	c := e.core()
	for i := 0; i < 2; i++ {
		cards, err := c.Cards(context.Background())
		if err != nil || len(cards) != 1 || !cards[0].InTmux || !cards[0].Private {
			t.Fatalf("cards=%+v err=%v", cards, err)
		}
	}
}

func TestCardsAttachedThroughGroup(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"idle"`)
	e.tree = locate.Tree{100: 10, 10: 1}
	e.fake.panes[e.userSock] = paneLine("%1", 10, "$1", "a", "@1", "grp")
	e.fake.clients[e.userSock] = clientLine("/dev/pts/9", 50, "$2", "b", "grp", 1)
	cards, _ := e.core().Cards(context.Background())
	if !cards[0].Attached {
		t.Errorf("session shown by a grouped session is not attached: %+v", cards[0])
	}
}

func TestCardsReturnsTreeError(t *testing.T) {
	e := newEnv(t)
	c := e.core(func(o *Options) {
		o.ReadTree = func(context.Context, tmux.Runner) (locate.Tree, error) { return nil, context.Canceled }
	})
	if _, err := c.Cards(context.Background()); err == nil {
		t.Error("tree error hidden")
	}
}

func TestPromptReasonOrder(t *testing.T) {
	waiting := func(for_ string) registry.Session {
		return registry.Session{State: registry.StateWaiting, RawStatus: "waiting", WaitingFor: for_}
	}
	cases := []struct {
		name   string
		inTmux bool
		s      registry.Session
		want   string
	}{
		{"outside tmux wins", false, waiting("permission prompt"), "session hors tmux"},
		{"permission", true, waiting("permission prompt"), "attend une autorisation"},
		{"other waiting", true, waiting("dialog open"), "attend une réponse (dialog open)"},
		{"waiting without detail", true, waiting(""), "attend une réponse"},
		{"unknown status", true, registry.Session{RawStatus: "starting"}, "état inconnu"},
		{"empty status", true, registry.Session{}, "état inconnu"},
		{"idle", true, registry.Session{State: registry.StateIdle, RawStatus: "idle"}, ""},
		{"busy", true, registry.Session{State: registry.StateBusy, RawStatus: "busy"}, ""},
		{"shell", true, registry.Session{State: registry.StateBusy, RawStatus: "shell"}, ""},
	}
	for _, c := range cases {
		if got := PromptReason(c.inTmux, c.s); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestScreenTailLinesAndHash(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"idle"`)
	e.tree = locate.Tree{100: 10, 10: 1}
	e.fake.panes[e.userSock] = paneLine("%1", 10, "$1", "a", "@1", "")
	e.fake.capture["%1"] = "one\ntwo\n\x1b[0m\nthree\n\n  \n\x1b[1m\x1b[0m\n"
	c := e.core()
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	s, err := c.Screen(context.Background(), 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"\x1b[0m", "three"}; !reflect.DeepEqual(s.Lines, want) || s.Width != 80 || s.Height != 24 {
		t.Errorf("screen: %+v", s)
	}
	all, _ := c.Screen(context.Background(), 100, 0)
	if len(all.Lines) != 4 || all.Hash == s.Hash {
		t.Errorf("all: %+v", all)
	}
	again, _ := c.Screen(context.Background(), 100, 2)
	if again.Hash != s.Hash {
		t.Error("hash not stable")
	}
	e.fake.capture["%1"] = "one\ntwo\n\x1b[0m\nthrex\n"
	if changed, _ := c.Screen(context.Background(), 100, 2); changed.Hash == s.Hash {
		t.Error("hash ignores content")
	}
	if screenHash(s.Lines, 80, 24) == screenHash(s.Lines, 81, 24) || screenHash(s.Lines, 80, 24) == screenHash(s.Lines, 80, 25) {
		t.Error("hash ignores size")
	}
}

func TestScreenSizeComesFromTheCapture(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"idle"`)
	e.tree = locate.Tree{100: 10, 10: 1}
	e.fake.panes[e.userSock] = paneLine("%1", 10, "$1", "a", "@1", "")
	e.fake.capture["%1"] = "one\n"
	c := e.core()
	if _, err := c.Cards(context.Background()); err != nil {
		t.Fatal(err)
	}
	before, err := c.Screen(context.Background(), 100, 5)
	if err != nil || before.Width != 80 || before.Height != 24 {
		t.Fatalf("screen=%+v err=%v", before, err)
	}
	e.fake.sizes["%1"] = "120\t40"
	after, err := c.Screen(context.Background(), 100, 5)
	if err != nil || after.Width != 120 || after.Height != 40 {
		t.Fatalf("screen=%+v err=%v", after, err)
	}
	if after.Hash == before.Hash || !reflect.DeepEqual(after.Lines, before.Lines) {
		t.Errorf("hash does not follow the live size: %q %q", before.Hash, after.Hash)
	}
	if after.Hash != screenHash(after.Lines, 120, 40) {
		t.Error("hash does not cover the live size")
	}
	e.fake.sizes["%1"] = "garbage"
	if _, err := c.Screen(context.Background(), 100, 5); err == nil {
		t.Error("unreadable size accepted")
	}
}

func TestCardsDropTranscriptCacheOfVanishedSessions(t *testing.T) {
	e := newEnv(t)
	line := func(cmd string) string {
		return `{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash","input":{"command":"` + cmd + `"}}]}}`
	}
	body := `"sessionId":"` + idA + `","startedAt":1000,"status":"busy"`
	e.session(100, body)
	e.transcript(idA, line("make"))
	path := filepath.Join(e.cfgDir, "projects", "slug", idA+".jsonl")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	c := e.core()
	activity := func() string {
		cards, err := c.Cards(context.Background())
		if err != nil || len(cards) != 1 {
			t.Fatalf("cards=%+v err=%v", cards, err)
		}
		return cards[0].Activity
	}
	if got := activity(); got != "Bash: make" {
		t.Fatalf("activity %q", got)
	}

	e.alive[100] = false
	if cards, _ := c.Cards(context.Background()); len(cards) != 0 {
		t.Fatalf("session still listed: %+v", cards)
	}
	e.transcript(idA, line("test"))
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	e.alive[100] = true
	if got := activity(); got != "Bash: test" {
		t.Errorf("stale activity %q", got)
	}
}

func TestScreenEmptyPaneAndUnknownSession(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"idle"`)
	e.tree = locate.Tree{100: 10, 10: 1}
	e.fake.panes[e.userSock] = paneLine("%1", 10, "$1", "a", "@1", "")
	e.fake.capture["%1"] = "\n\n"
	c := e.core()
	c.Cards(context.Background())
	if s, err := c.Screen(context.Background(), 100, 5); err != nil || s.Lines == nil || len(s.Lines) != 0 {
		t.Errorf("empty screen: %+v err=%v", s, err)
	}
	if _, err := c.Screen(context.Background(), 999, 5); err == nil {
		t.Error("unknown session accepted")
	}
}

func TestPaneOf(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","startedAt":1000,"status":"idle"`)
	e.tree = locate.Tree{100: 10, 10: 1}
	e.fake.panes[e.userSock] = paneLine("%1", 10, "$1", "a", "@1", "")
	c := e.core()
	if _, ok := c.PaneOf(100); ok {
		t.Error("pane known before the first snapshot")
	}
	c.Cards(context.Background())
	if p, ok := c.PaneOf(100); !ok || p.ID != "%1" {
		t.Errorf("pane=%+v ok=%v", p, ok)
	}
	if _, ok := c.PaneOf(5); ok {
		t.Error("unknown pid has a pane")
	}
}

func TestNewDefaultsRegistryDirectory(t *testing.T) {
	e := newEnv(t)
	build := func(h Host, home string) *Core {
		t.Helper()
		c, err := New(Options{Bin: "tmux", Run: e.fake.run, Hosting: h, Home: home})
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	if got := build(nil, "/home/u").opts.Registry.Dir; got != "/home/u/.claude" {
		t.Errorf("home default: %q", got)
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "/from/env")
	if got := build(nil, "/home/u").opts.Registry.Dir; got != "/from/env" {
		t.Errorf("environment: %q", got)
	}
	if got := build(e.hosting, "/home/u").opts.Registry.Dir; got != "/app/claude" {
		t.Errorf("hosting: %q", got)
	}
	c, err := New(Options{Bin: "tmux", Run: e.fake.run, Registry: registry.Reader{Dir: "/explicit"}, Home: "/home/u"})
	if err != nil || c.opts.Registry.Dir != "/explicit" {
		t.Errorf("explicit directory replaced: %v", c.opts.Registry.Dir)
	}
}

func TestNewDefaultsUserSocketDirectoryFromHostingConfig(t *testing.T) {
	e := newEnv(t)
	tmp, err := os.MkdirTemp("/tmp", "ct")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmp) })
	other, err := os.MkdirTemp("/tmp", "ct")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(other) })
	sockets := map[string]string{}
	for name, root := range map[string]string{"config": tmp, "env": other} {
		dir := filepath.Join(root, fmt.Sprintf("tmux-%d", os.Getuid()))
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		sockets[name] = filepath.Join(dir, "default")
		listenUnix(t, sockets[name])
	}
	if err := os.MkdirAll(e.hosting.ConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`{"tmuxTmpDir":%q}`, tmp)
	if err := os.WriteFile(filepath.Join(e.hosting.ConfigDir, "config.json"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	e.hosting.Env = append(e.hosting.Env, "TMUX_TMPDIR="+other)
	c, err := New(Options{Bin: "tmux", Run: e.fake.run, Hosting: e.hosting})
	if err != nil {
		t.Fatal(err)
	}
	c.Sweep(context.Background())
	if !e.fake.has(sockets["config"], "list-sessions") || e.fake.has(sockets["env"], "list-sessions") {
		t.Errorf("servers visited: %v", e.fake.calls)
	}
}

func TestSweepRemovesOldEnvironmentFiles(t *testing.T) {
	e := newEnv(t)
	old := filepath.Join(e.hosting.SocketDir, "env-old")
	recent := filepath.Join(e.hosting.SocketDir, "env-recent")
	for path, age := range map[string]time.Duration{old: 2 * time.Minute, recent: 10 * time.Second} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, testNow.Add(-age), testNow.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}
	e.core().Sweep(context.Background())
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old environment file kept (err=%v)", err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("recent environment file removed: %v", err)
	}
}

func TestSweepVisitsEveryServer(t *testing.T) {
	e := newEnv(t)
	e.core().Sweep(context.Background())
	if !e.fake.has(e.userSock, "list-sessions") || !e.fake.has(e.privSock, "list-sessions") {
		t.Errorf("calls: %v", e.fake.calls)
	}
}

func TestSortSessions(t *testing.T) {
	t0 := time.Unix(100, 0)
	ss := []registry.Session{{PID: 3, StartedAt: t0.Add(time.Second)}, {PID: 2, StartedAt: t0}, {PID: 1, StartedAt: t0}, {PID: 0}}
	sortSessions(ss)
	var pids []int
	for _, s := range ss {
		pids = append(pids, s.PID)
	}
	if !reflect.DeepEqual(pids, []int{0, 1, 2, 3}) {
		t.Errorf("pids=%v", pids)
	}
}

func TestReadServersReportsPanesAndClientsFailures(t *testing.T) {
	panesLine := paneLine("%1", 10, "$1", "a", "@1", "")
	fail := func(failing ...string) tmux.Runner {
		return func(_ context.Context, _ io.Reader, _ string, args ...string) ([]byte, error) {
			switch args[2] {
			case "list-panes":
				if slices.Contains(failing, "list-panes") {
					return nil, errors.New("panes failed")
				}
				return []byte(panesLine), nil
			default:
				if slices.Contains(failing, "list-clients") {
					return nil, errors.New("clients failed")
				}
				return []byte(clientLine("/dev/pts/1", 7, "$1", "a", "", 5)), nil
			}
		}
	}
	read := func(failing ...string) serverState {
		return (&Core{}).readServers(context.Background(), []tmux.Server{{Socket: "/s", Run: fail(failing...)}})[0]
	}
	if st := read(); !st.ok || len(st.panes) != 1 || len(st.clients) != 1 {
		t.Errorf("healthy server: %+v", st)
	}
	if st := read("list-clients"); st.ok || st.err == nil || st.err.Error() != "clients failed" {
		t.Errorf("clients failure: %+v", st)
	}
	if st := read("list-panes", "list-clients"); st.ok || st.err == nil || st.err.Error() != "panes failed" {
		t.Errorf("both failing: %+v", st)
	}
}

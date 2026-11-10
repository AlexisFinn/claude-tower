package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"claude-tower/internal/core"
	"claude-tower/internal/hosting"
	"claude-tower/internal/tmux"
)

type fakeEngine struct {
	newSessions []string
	newNames    []string
	cards       []core.Card
	onSweep     func()
	// err is returned by the methods that return an error.
	err error
	// calls lists the methods called, with their arguments, and deadlines
	// the methods that got a context with a deadline.
	mu        sync.Mutex
	calls     []string
	deadlines []string
}

func (f *fakeEngine) record(ctx context.Context, call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	if _, ok := ctx.Deadline(); ok {
		f.deadlines = append(f.deadlines, call)
	}
}

func (f *fakeEngine) Cards(ctx context.Context) ([]core.Card, error) {
	f.record(ctx, "Cards")
	return f.cards, f.err
}
func (f *fakeEngine) Screen(ctx context.Context, pid, maxLines int) (core.Screen, error) {
	f.record(ctx, fmt.Sprintf("Screen %d %d", pid, maxLines))
	return core.Screen{Width: pid, Height: maxLines}, f.err
}
func (f *fakeEngine) Prompt(ctx context.Context, pid int, text string) error {
	f.record(ctx, fmt.Sprintf("Prompt %d %s", pid, text))
	return f.err
}
func (f *fakeEngine) Focus(ctx context.Context, pid int) (core.FocusResult, error) {
	f.record(ctx, fmt.Sprintf("Focus %d", pid))
	return core.FocusResult{Mode: "embedded"}, f.err
}
func (f *fakeEngine) NewSession(_ context.Context, dir, name string) error {
	f.newSessions = append(f.newSessions, dir)
	f.newNames = append(f.newNames, name)
	return nil
}
func (f *fakeEngine) Kill(ctx context.Context, pid int) error {
	f.record(ctx, fmt.Sprintf("Kill %d", pid))
	return f.err
}
func (f *fakeEngine) Fit(ctx context.Context, pid, cols, rows int) (bool, error) {
	f.record(ctx, fmt.Sprintf("Fit %d %d %d", pid, cols, rows))
	return true, f.err
}
func (f *fakeEngine) PaneOf(int) (tmux.Pane, bool) { return tmux.Pane{}, false }
func (f *fakeEngine) Sweep(context.Context) {
	if f.onSweep != nil {
		f.onSweep()
	}
}
func (f *fakeEngine) StartFailures() []core.StartFailure {
	f.mu.Lock()
	f.calls = append(f.calls, "StartFailures")
	f.mu.Unlock()
	return []core.StartFailure{{Name: "proj"}}
}

type fakeWindow struct {
	dir   string
	files []string
	err   error
}

func (w fakeWindow) Emit(string, any)                 {}
func (w fakeWindow) ChooseDirectory() (string, error) { return w.dir, w.err }
func (w fakeWindow) ChooseFiles() ([]string, error)   { return w.files, w.err }
func (w fakeWindow) GuardQuit(bool)                   {}
func (w fakeWindow) AckQuit()                         {}
func (w fakeWindow) Quit()                            {}

func started(e Engine, w Window) *App {
	a := New(w, nil)
	a.setEngine(e)
	close(a.started)
	return a
}

func TestNewSessionStartsInChosenDirectory(t *testing.T) {
	e := &fakeEngine{}
	a := started(e, fakeWindow{dir: "/work/x"})
	if err := a.NewSession("mon nom"); err != nil {
		t.Fatal(err)
	}
	if len(e.newSessions) != 1 || e.newSessions[0] != "/work/x" {
		t.Errorf("sessions started: %v", e.newSessions)
	}
	if len(e.newNames) != 1 || e.newNames[0] != "mon nom" {
		t.Errorf("names given: %v", e.newNames)
	}
}

func TestNewSessionCancelledStartsNothing(t *testing.T) {
	e := &fakeEngine{}
	a := started(e, fakeWindow{})
	if err := a.NewSession(""); err != nil {
		t.Fatalf("cancelling is not an error: %v", err)
	}
	if len(e.newSessions) != 0 {
		t.Errorf("sessions started: %v", e.newSessions)
	}
}

func TestChooseFilesReturnsThePickedPaths(t *testing.T) {
	a := New(fakeWindow{files: []string{"/work/a.txt", "/work/b c.png"}}, nil)
	got, err := a.ChooseFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "/work/a.txt" || got[1] != "/work/b c.png" {
		t.Errorf("files: %v", got)
	}
}

func TestChooseFilesCancelledReturnsNothing(t *testing.T) {
	a := New(fakeWindow{}, nil)
	got, err := a.ChooseFiles()
	if err != nil || len(got) != 0 {
		t.Errorf("files %v, error %v", got, err)
	}
}

func TestChooseFilesPassesTheDialogError(t *testing.T) {
	boom := errors.New("boom")
	a := New(fakeWindow{files: []string{"/work/a.txt"}, err: boom}, nil)
	if _, err := a.ChooseFiles(); !errors.Is(err, boom) {
		t.Errorf("error: %v", err)
	}
}

func TestNewSessionDialogErrorStartsNothing(t *testing.T) {
	e := &fakeEngine{}
	boom := errors.New("boom")
	a := started(e, fakeWindow{dir: "/work/x", err: boom})
	if err := a.NewSession(""); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if len(e.newSessions) != 0 {
		t.Errorf("sessions started: %v", e.newSessions)
	}
}

func TestMethodsFailWithStartupError(t *testing.T) {
	a := New(fakeWindow{}, nil)
	a.startErr = errors.New("tmux introuvable")
	close(a.started)
	if a.StartupError() != "tmux introuvable" {
		t.Errorf("StartupError: %q", a.StartupError())
	}
	if _, err := a.Cards(); err == nil {
		t.Error("Cards succeeded")
	}
	if err := a.NewSession(""); err == nil {
		t.Error("NewSession succeeded")
	}
	if _, err := a.OpenTerminal(1, 80, 24); err == nil {
		t.Error("OpenTerminal succeeded")
	}
	if err := a.CloseTerminal(1); err == nil {
		t.Error("CloseTerminal succeeded")
	}
	if _, err := a.StartFailures(); err == nil {
		t.Error("StartFailures succeeded")
	}
	Shutdown(a)
}

func TestCardsAreForwarded(t *testing.T) {
	e := &fakeEngine{cards: []core.Card{{PID: 7}}}
	a := started(e, fakeWindow{})
	cards, err := a.Cards()
	if err != nil || len(cards) != 1 || cards[0].PID != 7 {
		t.Errorf("got %v %v", cards, err)
	}
	if a.StartupError() != "" {
		t.Errorf("StartupError: %q", a.StartupError())
	}
}

func TestStartFailuresAreForwarded(t *testing.T) {
	a := started(&fakeEngine{}, fakeWindow{})
	got, err := a.StartFailures()
	if err != nil || len(got) != 1 || got[0].Name != "proj" {
		t.Errorf("got %v %v", got, err)
	}
}

func TestOpenTerminalOfUnknownSessionFails(t *testing.T) {
	a := started(&fakeEngine{}, fakeWindow{})
	if _, err := a.OpenTerminal(1, 80, 24); err == nil {
		t.Error("OpenTerminal succeeded")
	}
	if err := a.TermReady(3); err == nil {
		t.Error("TermReady accepted an unknown terminal")
	}
	Shutdown(a)
}

func TestMethodsFailBeforeStartup(t *testing.T) {
	a := New(fakeWindow{}, nil)
	close(a.started)
	if _, err := a.Cards(); !errors.Is(err, errNotStarted) {
		t.Errorf("got %v", err)
	}
}

func TestMethodsWaitForSlowStartup(t *testing.T) {
	a := New(fakeWindow{}, nil)
	release := make(chan struct{})
	a.boot = func(a *App) {
		<-release
		a.setEngine(&fakeEngine{cards: []core.Card{{PID: 9}}})
		a.startWarn = "config"
	}
	go Startup(a)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cards, err := a.Cards()
			if err != nil || len(cards) != 1 || cards[0].PID != 9 {
				t.Errorf("Cards: %v %v", cards, err)
			}
			if a.StartupError() != "" || a.StartupWarning() != "config" {
				t.Errorf("StartupError %q, StartupWarning %q", a.StartupError(), a.StartupWarning())
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
}

func TestMethodsFailWhenStartupIsTooLong(t *testing.T) {
	a := New(fakeWindow{}, nil)
	a.startWait = 20 * time.Millisecond
	if _, err := a.Cards(); !errors.Is(err, errStartTooLong) {
		t.Errorf("got %v", err)
	}
	if a.StartupError() != errStartTooLong.Error() {
		t.Errorf("StartupError: %q", a.StartupError())
	}
	Shutdown(a)
}

func TestStartupWarningIsEmptyWithoutWarning(t *testing.T) {
	a := started(&fakeEngine{}, fakeWindow{})
	if a.StartupWarning() != "" {
		t.Errorf("StartupWarning: %q", a.StartupWarning())
	}
}

// startProbe records what start does, in order, with a Hosting whose tmux and
// login shell are fakes.
type startProbe struct {
	events        []string
	configDir     string
	coreErr       error
	hostingErr    error
	sessionConfig string
}

func (p *startProbe) add(event string) { p.events = append(p.events, event) }

func (p *startProbe) app(t *testing.T) *App {
	t.Helper()
	root := t.TempDir()
	if p.configDir == "" {
		p.configDir = filepath.Join(root, "cfg")
	}
	a := New(fakeWindow{}, nil)
	a.newHosting = func([]string) (*hosting.Hosting, error) {
		p.add("hosting")
		if p.hostingErr != nil {
			return nil, p.hostingErr
		}
		return &hosting.Hosting{
			SocketDir: filepath.Join(root, "run"),
			ConfigDir: p.configDir,
			Env:       []string{"HOME=/h"},
			Shell:     "/bin/fake-shell",
			ShellRun: func(context.Context, io.Reader, string, ...string) ([]byte, error) {
				p.add("login-shell")
				return []byte("CLAUDE_CONFIG_DIR=/resolved\x00"), nil
			},
			Tmux: func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
				if name == "infocmp" {
					p.add("infocmp")
					return nil, nil
				}
				p.add(args[len(args)-1])
				return nil, errors.New("no server running")
			},
		}, nil
	}
	a.newCore = func(h *hosting.Hosting) (Engine, error) {
		p.add("core")
		p.sessionConfig = h.SessionConfigDir
		if p.coreErr != nil {
			return nil, p.coreErr
		}
		return &fakeEngine{onSweep: func() { p.add("sweep") }}, nil
	}
	return a
}

func TestStartResolvesConfigDirBeforeCoreAndWritesConfigBeforeSweep(t *testing.T) {
	p := &startProbe{}
	a := p.app(t)
	Startup(a)
	defer Shutdown(a)
	want := []string{"hosting", "login-shell", "core", "infocmp", "list-sessions", "sweep"}
	if !reflect.DeepEqual(p.events, want) {
		t.Errorf("events %v, want %v", p.events, want)
	}
	if p.sessionConfig != "/resolved" {
		t.Errorf("config directory seen by core: %q", p.sessionConfig)
	}
	if a.StartupError() != "" || a.StartupWarning() != "" {
		t.Errorf("StartupError %q, StartupWarning %q", a.StartupError(), a.StartupWarning())
	}
	if _, err := a.Cards(); err != nil {
		t.Errorf("Cards: %v", err)
	}
}

func TestStartHostingErrorIsTheStartupError(t *testing.T) {
	p := &startProbe{hostingErr: errors.New("pas de dossier personnel")}
	a := p.app(t)
	Startup(a)
	defer Shutdown(a)
	if a.StartupError() != "pas de dossier personnel" {
		t.Errorf("StartupError: %q", a.StartupError())
	}
	if want := []string{"hosting"}; !reflect.DeepEqual(p.events, want) {
		t.Errorf("events %v, want %v", p.events, want)
	}
	if _, err := a.Cards(); err == nil {
		t.Error("Cards succeeded")
	}
}

func TestStartCoreErrorIsTheStartupError(t *testing.T) {
	p := &startProbe{coreErr: errors.New("tmux introuvable")}
	a := p.app(t)
	Startup(a)
	defer Shutdown(a)
	if a.StartupError() != "tmux introuvable" {
		t.Errorf("StartupError: %q", a.StartupError())
	}
	if want := []string{"hosting", "login-shell", "core"}; !reflect.DeepEqual(p.events, want) {
		t.Errorf("events %v, want %v", p.events, want)
	}
	if _, err := a.Cards(); err == nil {
		t.Error("Cards succeeded")
	}
}

func TestStartConfigErrorIsAWarningAndDoesNotBlock(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	p := &startProbe{configDir: filepath.Join(file, "cfg")}
	a := p.app(t)
	Startup(a)
	defer Shutdown(a)
	if a.StartupWarning() == "" {
		t.Error("no startup warning")
	}
	if a.StartupError() != "" {
		t.Errorf("StartupError: %q", a.StartupError())
	}
	if want := []string{"hosting", "login-shell", "core", "sweep"}; !reflect.DeepEqual(p.events, want) {
		t.Errorf("events %v, want %v", p.events, want)
	}
	if _, err := a.Cards(); err != nil {
		t.Errorf("Cards: %v", err)
	}
}

// wrappers lists the methods of App that forward to the engine or to the
// terminals, with the value they return when everything works.
var wrappers = []struct {
	name string
	call func(a *App) (any, error)
	want any
	// call is the engine call that the method makes, "" when it makes none.
	engineCall string
	// engineFails reports whether the error of the engine reaches the caller.
	engineFails bool
}{
	{"Cards", func(a *App) (any, error) { return a.Cards() }, []core.Card{{PID: 7}}, "Cards", true},
	{"StartFailures", func(a *App) (any, error) { return a.StartFailures() }, []core.StartFailure{{Name: "proj"}}, "StartFailures", false},
	{"Screen", func(a *App) (any, error) { return a.Screen(4, 20) }, core.Screen{Width: 4, Height: 20}, "Screen 4 20", true},
	{"Prompt", func(a *App) (any, error) { return nil, a.Prompt(4, "hi") }, nil, "Prompt 4 hi", true},
	{"Focus", func(a *App) (any, error) { return a.Focus(4) }, core.FocusResult{Mode: "embedded"}, "Focus 4", true},
	{"Kill", func(a *App) (any, error) { return nil, a.Kill(4) }, nil, "Kill 4", true},
	{"FitSession", func(a *App) (any, error) { return a.FitSession(4, 80, 24) }, true, "Fit 4 80 24", true},
}

// terminalWrappers lists the methods of App that forward to the terminals;
// all but the closing fail on an unknown session or terminal once the engine
// is ready.
var terminalWrappers = []struct {
	name         string
	call         func(a *App) error
	unknownFails bool
}{
	{"OpenTerminal", func(a *App) error { _, err := a.OpenTerminal(4, 80, 24); return err }, true},
	{"TermReady", func(a *App) error { return a.TermReady(3) }, true},
	{"TermInput", func(a *App) error { return a.TermInput(3, "x") }, true},
	{"TermResize", func(a *App) error { return a.TermResize(3, 80, 24) }, true},
	{"CloseTerminal", func(a *App) error { return a.CloseTerminal(3) }, false},
}

func TestWrappersFailBeforeStartup(t *testing.T) {
	for _, c := range wrappers {
		t.Run(c.name, func(t *testing.T) {
			a := New(fakeWindow{}, nil)
			close(a.started)
			if _, err := c.call(a); !errors.Is(err, errNotStarted) {
				t.Errorf("got %v", err)
			}
		})
	}
	for _, c := range terminalWrappers {
		t.Run(c.name, func(t *testing.T) {
			a := New(fakeWindow{}, nil)
			close(a.started)
			if err := c.call(a); !errors.Is(err, errNotStarted) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestWrappersFailWithTheStartError(t *testing.T) {
	boom := errors.New("tmux introuvable")
	for _, c := range wrappers {
		t.Run(c.name, func(t *testing.T) {
			a := New(fakeWindow{}, nil)
			a.startErr = boom
			close(a.started)
			if _, err := c.call(a); !errors.Is(err, boom) {
				t.Errorf("got %v", err)
			}
		})
	}
	for _, c := range terminalWrappers {
		t.Run(c.name, func(t *testing.T) {
			a := New(fakeWindow{}, nil)
			a.startErr = boom
			close(a.started)
			if err := c.call(a); !errors.Is(err, boom) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestWrappersForwardToTheEngine(t *testing.T) {
	for _, c := range wrappers {
		t.Run(c.name, func(t *testing.T) {
			e := &fakeEngine{cards: []core.Card{{PID: 7}}}
			a := started(e, fakeWindow{})
			got, err := c.call(a)
			if err != nil || !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, %v want %v", got, err, c.want)
			}
			if !reflect.DeepEqual(e.calls, []string{c.engineCall}) {
				t.Errorf("engine calls %v want %v", e.calls, []string{c.engineCall})
			}
			if c.engineCall != "StartFailures" && !reflect.DeepEqual(e.deadlines, []string{c.engineCall}) {
				t.Errorf("calls with a deadline: %v", e.deadlines)
			}
		})
	}
}

func TestWrappersForwardTheEngineError(t *testing.T) {
	boom := errors.New("boom")
	for _, c := range wrappers {
		if !c.engineFails {
			continue
		}
		t.Run(c.name, func(t *testing.T) {
			a := started(&fakeEngine{err: boom}, fakeWindow{})
			if _, err := c.call(a); !errors.Is(err, boom) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestTerminalWrappersReachTheTerminals(t *testing.T) {
	for _, c := range terminalWrappers {
		t.Run(c.name, func(t *testing.T) {
			a := started(&fakeEngine{}, fakeWindow{})
			defer Shutdown(a)
			err := c.call(a)
			if errors.Is(err, errNotStarted) || (err != nil) != c.unknownFails {
				t.Errorf("got %v, unknownFails=%v", err, c.unknownFails)
			}
		})
	}
}

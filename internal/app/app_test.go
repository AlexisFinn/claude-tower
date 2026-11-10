package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"claude-tower/internal/core"
	"claude-tower/internal/tmux"
)

type fakeEngine struct {
	newSessions []string
	newNames    []string
	cards       []core.Card
}

func (f *fakeEngine) Cards(context.Context) ([]core.Card, error) { return f.cards, nil }
func (f *fakeEngine) Screen(context.Context, int, int) (core.Screen, error) {
	return core.Screen{}, nil
}
func (f *fakeEngine) Prompt(context.Context, int, string) error { return nil }
func (f *fakeEngine) Focus(context.Context, int) (core.FocusResult, error) {
	return core.FocusResult{}, nil
}
func (f *fakeEngine) NewSession(_ context.Context, dir, name string) error {
	f.newSessions = append(f.newSessions, dir)
	f.newNames = append(f.newNames, name)
	return nil
}
func (f *fakeEngine) Kill(context.Context, int) error                  { return nil }
func (f *fakeEngine) Fit(context.Context, int, int, int) (bool, error) { return true, nil }
func (f *fakeEngine) PaneOf(int) (tmux.Pane, bool)                     { return tmux.Pane{}, false }
func (f *fakeEngine) Sweep(context.Context)                            {}
func (f *fakeEngine) StartFailures() []core.StartFailure {
	return []core.StartFailure{{Name: "proj"}}
}

type fakeDialogs struct {
	dir   string
	files []string
	err   error
}

func (d fakeDialogs) ChooseDirectory() (string, error) { return d.dir, d.err }
func (d fakeDialogs) ChooseFiles() ([]string, error)   { return d.files, d.err }
func (d fakeDialogs) GuardQuit()                       {}
func (d fakeDialogs) Quit()                            {}

type noEmit struct{}

func (noEmit) Emit(string, any) {}

func started(e Engine, d Dialogs) *App {
	a := New(noEmit{}, d, nil)
	a.setEngine(e)
	close(a.started)
	return a
}

func TestNewSessionStartsInChosenDirectory(t *testing.T) {
	e := &fakeEngine{}
	a := started(e, fakeDialogs{dir: "/work/x"})
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
	a := started(e, fakeDialogs{})
	if err := a.NewSession(""); err != nil {
		t.Fatalf("cancelling is not an error: %v", err)
	}
	if len(e.newSessions) != 0 {
		t.Errorf("sessions started: %v", e.newSessions)
	}
}

func TestChooseFilesReturnsThePickedPaths(t *testing.T) {
	a := New(noEmit{}, fakeDialogs{files: []string{"/work/a.txt", "/work/b c.png"}}, nil)
	got, err := a.ChooseFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "/work/a.txt" || got[1] != "/work/b c.png" {
		t.Errorf("files: %v", got)
	}
}

func TestChooseFilesCancelledReturnsNothing(t *testing.T) {
	a := New(noEmit{}, fakeDialogs{}, nil)
	got, err := a.ChooseFiles()
	if err != nil || len(got) != 0 {
		t.Errorf("files %v, error %v", got, err)
	}
}

func TestChooseFilesPassesTheDialogError(t *testing.T) {
	boom := errors.New("boom")
	a := New(noEmit{}, fakeDialogs{files: []string{"/work/a.txt"}, err: boom}, nil)
	if _, err := a.ChooseFiles(); !errors.Is(err, boom) {
		t.Errorf("error: %v", err)
	}
}

func TestNewSessionDialogErrorStartsNothing(t *testing.T) {
	e := &fakeEngine{}
	boom := errors.New("boom")
	a := started(e, fakeDialogs{dir: "/work/x", err: boom})
	if err := a.NewSession(""); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if len(e.newSessions) != 0 {
		t.Errorf("sessions started: %v", e.newSessions)
	}
}

func TestMethodsFailWithStartupError(t *testing.T) {
	a := New(noEmit{}, fakeDialogs{}, nil)
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
	a := started(e, fakeDialogs{})
	cards, err := a.Cards()
	if err != nil || len(cards) != 1 || cards[0].PID != 7 {
		t.Errorf("got %v %v", cards, err)
	}
	if a.StartupError() != "" {
		t.Errorf("StartupError: %q", a.StartupError())
	}
}

func TestStartFailuresAreForwarded(t *testing.T) {
	a := started(&fakeEngine{}, fakeDialogs{})
	got, err := a.StartFailures()
	if err != nil || len(got) != 1 || got[0].Name != "proj" {
		t.Errorf("got %v %v", got, err)
	}
}

func TestOpenTerminalOfUnknownSessionFails(t *testing.T) {
	a := started(&fakeEngine{}, fakeDialogs{})
	if _, err := a.OpenTerminal(1, 80, 24); err == nil {
		t.Error("OpenTerminal succeeded")
	}
	if err := a.TermReady(3); err == nil {
		t.Error("TermReady accepted an unknown terminal")
	}
	Shutdown(a)
}

func TestMethodsFailBeforeStartup(t *testing.T) {
	a := New(noEmit{}, fakeDialogs{}, nil)
	close(a.started)
	if _, err := a.Cards(); !errors.Is(err, errNotStarted) {
		t.Errorf("got %v", err)
	}
}

func TestMethodsWaitForSlowStartup(t *testing.T) {
	a := New(noEmit{}, fakeDialogs{}, nil)
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
	a := New(noEmit{}, fakeDialogs{}, nil)
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
	a := started(&fakeEngine{}, fakeDialogs{})
	if a.StartupWarning() != "" {
		t.Errorf("StartupWarning: %q", a.StartupWarning())
	}
}

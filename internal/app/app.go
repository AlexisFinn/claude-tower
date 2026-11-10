// Package app holds the methods that the frontend calls. It wires the engine
// of internal/core and the embedded terminals together; what belongs to the
// window system comes in through the Emitter and Dialogs interfaces.
package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"claude-tower/internal/core"
	"claude-tower/internal/hosting"
	"claude-tower/internal/ptyterm"
	"claude-tower/internal/titles"
	"claude-tower/internal/tmux"
)

const (
	callTimeout = 30 * time.Second
	// resolveTimeout bounds the login shell run at startup, within the
	// callTimeout of the whole Startup.
	resolveTimeout = 10 * time.Second
	// startWait is how long a method waits for Startup to finish.
	startWait = 30 * time.Second
)

var (
	errNotStarted   = errors.New("application non démarrée")
	errStartTooLong = errors.New("démarrage trop long")
)

// Emitter sends an event to the frontend.
type Emitter = ptyterm.Emitter

// Dialogs asks the user for something through native dialogs.
type Dialogs interface {
	// ChooseDirectory returns the directory picked by the user, "" when the
	// dialog is cancelled.
	ChooseDirectory() (string, error)
	// ChooseFiles returns the files picked by the user, empty when the dialog
	// is cancelled.
	ChooseFiles() ([]string, error)
}

// Engine is what App needs from core.Core.
type Engine interface {
	Cards(ctx context.Context) ([]core.Card, error)
	Screen(ctx context.Context, pid, maxLines int) (core.Screen, error)
	Prompt(ctx context.Context, pid int, text string) error
	Focus(ctx context.Context, pid int) (core.FocusResult, error)
	NewSession(ctx context.Context, dir, name string) error
	Kill(ctx context.Context, pid int) error
	Fit(ctx context.Context, pid, cols, rows int) (bool, error)
	PaneOf(pid int) (tmux.Pane, bool)
	Sweep(ctx context.Context)
	StartFailures() []core.StartFailure
}

// App holds the methods bound to the frontend. Startup and Shutdown, which
// the frontend must not call, are package functions.
type App struct {
	emit    Emitter
	dialogs Dialogs
	environ []string

	// started is closed when Startup ends; engine, terms, startErr and
	// startWarn are written before that and only read after it.
	started   chan struct{}
	startOnce sync.Once
	startWait time.Duration
	boot      func(a *App)
	engine    Engine
	terms     *ptyterm.Manager
	startErr  error
	startWarn string
}

// New returns an App that has not started. environ is the environment of the
// process.
func New(emit Emitter, dialogs Dialogs, environ []string) *App {
	a := &App{emit: emit, dialogs: dialogs, environ: environ, started: make(chan struct{}), startWait: startWait}
	a.boot = (*App).start
	return a
}

// Startup builds the engine, writes the configuration of the private tmux
// server and removes the view sessions left by a previous run. When the engine
// cannot start, for instance because tmux is missing or too old, every method
// returns that error and StartupError describes it. A failing configuration
// write does not stop the engine: StartupWarning describes it. Startup
// releases the methods waiting for it when it returns.
func Startup(a *App) {
	defer a.startOnce.Do(func() { close(a.started) })
	a.boot(a)
}

func (a *App) start() {
	h, err := hosting.NewSystem(a.environ)
	if err != nil {
		a.startErr = err
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	rctx, rcancel := context.WithTimeout(ctx, resolveTimeout)
	h.ResolveClaudeConfigDir(rctx)
	rcancel()
	c, err := core.New(core.Options{Hosting: h, Titles: titles.New(titles.Options{Claude: h.ClaudePath, Env: h.NewSessionEnv})})
	if err != nil {
		a.startErr = err
		return
	}
	if err := h.EnsureConfig(ctx); err != nil {
		a.startWarn = err.Error()
	}
	c.Sweep(ctx)
	a.setEngine(c)
}

func (a *App) setEngine(e Engine) {
	a.engine = e
	a.terms = ptyterm.NewManager(e, a.emit, a.environ)
}

// Shutdown closes every embedded terminal.
func Shutdown(a *App) {
	select {
	case <-a.started:
	default:
		return
	}
	if a.terms != nil {
		a.terms.CloseAll()
	}
}

// wait blocks until Startup has ended, or fails after startWait.
func (a *App) wait() error {
	select {
	case <-a.started:
		return nil
	default:
	}
	timer := time.NewTimer(a.startWait)
	defer timer.Stop()
	select {
	case <-a.started:
		return nil
	case <-timer.C:
		return errStartTooLong
	}
}

// ready returns the engine, or the error that prevented it from starting.
func (a *App) ready() (Engine, error) {
	if err := a.wait(); err != nil {
		return nil, err
	}
	if a.engine == nil {
		if a.startErr == nil {
			return nil, errNotStarted
		}
		return nil, a.startErr
	}
	return a.engine, nil
}

func callContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), callTimeout)
}

// StartupError returns the reason the application cannot work, "" when it
// started well.
func (a *App) StartupError() string {
	if err := a.wait(); err != nil {
		return err.Error()
	}
	if a.startErr == nil {
		return ""
	}
	return a.startErr.Error()
}

// StartupWarning returns a problem met at startup that does not prevent the
// application from working, "" when there is none.
func (a *App) StartupWarning() string {
	if a.wait() != nil {
		return ""
	}
	return a.startWarn
}

// Cards returns one card per session, in display order.
func (a *App) Cards() ([]core.Card, error) {
	e, err := a.ready()
	if err != nil {
		return nil, err
	}
	ctx, cancel := callContext()
	defer cancel()
	return e.Cards(ctx)
}

// StartFailures returns the sessions started by the application that ended
// before Claude Code registered them, since the last call.
func (a *App) StartFailures() ([]core.StartFailure, error) {
	e, err := a.ready()
	if err != nil {
		return nil, err
	}
	return e.StartFailures(), nil
}

// Screen returns the bottom of the screen of a session.
func (a *App) Screen(pid, maxLines int) (core.Screen, error) {
	e, err := a.ready()
	if err != nil {
		return core.Screen{}, err
	}
	ctx, cancel := callContext()
	defer cancel()
	return e.Screen(ctx, pid, maxLines)
}

// Prompt sends a message to a session.
func (a *App) Prompt(pid int, text string) error {
	e, err := a.ready()
	if err != nil {
		return err
	}
	ctx, cancel := callContext()
	defer cancel()
	return e.Prompt(ctx, pid, text)
}

// Focus brings a session to the user.
func (a *App) Focus(pid int) (core.FocusResult, error) {
	e, err := a.ready()
	if err != nil {
		return core.FocusResult{}, err
	}
	ctx, cancel := callContext()
	defer cancel()
	return e.Focus(ctx, pid)
}

// Kill stops a session of the private server.
func (a *App) Kill(pid int) error {
	e, err := a.ready()
	if err != nil {
		return err
	}
	ctx, cancel := callContext()
	defer cancel()
	return e.Kill(ctx, pid)
}

// FitSession resizes a session launched by the application to the size its
// card shows without shrinking the text, unless a client shows it, and
// reports whether the session has that size now.
func (a *App) FitSession(pid, cols, rows int) (bool, error) {
	e, err := a.ready()
	if err != nil {
		return false, err
	}
	ctx, cancel := callContext()
	defer cancel()
	return e.Fit(ctx, pid, cols, rows)
}

// NewSession asks for a directory and starts a session there, with the given
// name unless it is empty. Cancelling the dialog starts nothing and is not an
// error.
func (a *App) NewSession(name string) error {
	e, err := a.ready()
	if err != nil {
		return err
	}
	dir, err := a.dialogs.ChooseDirectory()
	if err != nil || dir == "" {
		return err
	}
	ctx, cancel := callContext()
	defer cancel()
	return e.NewSession(ctx, dir, name)
}

// ChooseFiles asks for files to attach to a message and returns their absolute
// paths. Cancelling the dialog returns no path and is not an error.
func (a *App) ChooseFiles() ([]string, error) {
	return a.dialogs.ChooseFiles()
}

// OpenTerminal shows a session in a new terminal of cols by rows cells and
// returns its ID. The output starts to flow after TermReady.
func (a *App) OpenTerminal(pid, cols, rows int) (int, error) {
	if _, err := a.ready(); err != nil {
		return 0, err
	}
	ctx, cancel := callContext()
	defer cancel()
	return a.terms.Open(ctx, pid, cols, rows)
}

// TermReady tells that the frontend listens to the events of a terminal.
func (a *App) TermReady(id int) error {
	if _, err := a.ready(); err != nil {
		return err
	}
	return a.terms.Ready(id)
}

// TermInput writes what the user typed to a terminal.
func (a *App) TermInput(id int, data string) error {
	if _, err := a.ready(); err != nil {
		return err
	}
	return a.terms.Input(id, data)
}

// TermResize changes the size of a terminal.
func (a *App) TermResize(id, cols, rows int) error {
	if _, err := a.ready(); err != nil {
		return err
	}
	return a.terms.Resize(id, cols, rows)
}

// CloseTerminal closes a terminal.
func (a *App) CloseTerminal(id int) error {
	if _, err := a.ready(); err != nil {
		return err
	}
	return a.terms.Close(id)
}

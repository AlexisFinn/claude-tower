// Package app holds the methods that the frontend calls. It wires the engine
// of internal/core and the embedded terminals together; what belongs to the
// window system comes in through the Window interface.
package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"claude-tower/internal/core"
	"claude-tower/internal/hosting"
	"claude-tower/internal/ptyterm"
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

// Window is what App needs from the window system: events to the frontend,
// native dialogs, and the protocol that confirms the closing of the window.
type Window interface {
	Emitter
	// ChooseDirectory returns the directory picked by the user, "" when the
	// dialog is cancelled.
	ChooseDirectory() (string, error)
	// ChooseFiles returns the files picked by the user, empty when the dialog
	// is cancelled.
	ChooseFiles() ([]string, error)
	// GuardQuit makes closing the window ask the frontend for a confirmation
	// instead, while on is true.
	GuardQuit(on bool)
	// AckQuit tells that the frontend shows the confirmation it was asked for.
	AckQuit()
	// Quit closes the application without asking again.
	Quit()
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
	Close()
}

// App holds the methods bound to the frontend. Startup and Shutdown, which
// the frontend must not call, are package functions.
type App struct {
	window  Window
	environ []string

	// newHosting and newCore build what start wires together.
	newHosting func(environ []string) (*hosting.Hosting, error)
	newCore    func(h *hosting.Hosting) (Engine, error)

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
func New(window Window, environ []string) *App {
	a := &App{window: window, environ: environ, started: make(chan struct{}), startWait: startWait}
	a.boot = (*App).start
	a.newHosting = hosting.NewSystem
	a.newCore = newCore
	return a
}

func newCore(h *hosting.Hosting) (Engine, error) {
	c, err := core.New(core.Options{Hosting: h})
	if err != nil {
		return nil, err
	}
	return c, nil
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
	h, err := a.newHosting(a.environ)
	if err != nil {
		a.startErr = err
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	rctx, rcancel := context.WithTimeout(ctx, resolveTimeout)
	h.ResolveClaudeConfigDir(rctx)
	rcancel()
	c, err := a.newCore(h)
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
	a.terms = ptyterm.NewManager(e, a.window, a.environ)
}

// Shutdown closes every embedded terminal and stops the engine.
func Shutdown(a *App) {
	select {
	case <-a.started:
	default:
		return
	}
	if a.terms != nil {
		a.terms.CloseAll()
	}
	if a.engine != nil {
		a.engine.Close()
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

// call runs f on the engine with the context of one call, or returns the
// error that prevented the engine from starting.
func call[T any](a *App, f func(e Engine, ctx context.Context) (T, error)) (T, error) {
	e, err := a.ready()
	if err != nil {
		var zero T
		return zero, err
	}
	ctx, cancel := callContext()
	defer cancel()
	return f(e, ctx)
}

// callErr is call for a function that returns only an error.
func callErr(a *App, f func(e Engine, ctx context.Context) error) error {
	_, err := call(a, func(e Engine, ctx context.Context) (struct{}, error) {
		return struct{}{}, f(e, ctx)
	})
	return err
}

// terminals returns the manager of the embedded terminals, or the error that
// prevented the engine from starting.
func (a *App) terminals() (*ptyterm.Manager, error) {
	if _, err := a.ready(); err != nil {
		return nil, err
	}
	return a.terms, nil
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
	return call(a, func(e Engine, ctx context.Context) ([]core.Card, error) {
		return e.Cards(ctx)
	})
}

// StartFailures returns the sessions started by the application that ended
// before Claude Code registered them, since the last call.
func (a *App) StartFailures() ([]core.StartFailure, error) {
	return call(a, func(e Engine, _ context.Context) ([]core.StartFailure, error) {
		return e.StartFailures(), nil
	})
}

// Screen returns the bottom of the screen of a session.
func (a *App) Screen(pid, maxLines int) (core.Screen, error) {
	return call(a, func(e Engine, ctx context.Context) (core.Screen, error) {
		return e.Screen(ctx, pid, maxLines)
	})
}

// Prompt sends a message to a session.
func (a *App) Prompt(pid int, text string) error {
	return callErr(a, func(e Engine, ctx context.Context) error {
		return e.Prompt(ctx, pid, text)
	})
}

// Focus brings a session to the user.
func (a *App) Focus(pid int) (core.FocusResult, error) {
	return call(a, func(e Engine, ctx context.Context) (core.FocusResult, error) {
		return e.Focus(ctx, pid)
	})
}

// Kill stops a session of the private server.
func (a *App) Kill(pid int) error {
	return callErr(a, func(e Engine, ctx context.Context) error {
		return e.Kill(ctx, pid)
	})
}

// FitSession resizes a session launched by the application to the size its
// card shows without shrinking the text, unless a client shows it, and
// reports whether the session has that size now.
func (a *App) FitSession(pid, cols, rows int) (bool, error) {
	return call(a, func(e Engine, ctx context.Context) (bool, error) {
		return e.Fit(ctx, pid, cols, rows)
	})
}

// NewSession asks for a directory and starts a session there, with the given
// name unless it is empty. Cancelling the dialog starts nothing and is not an
// error.
func (a *App) NewSession(name string) error {
	e, err := a.ready()
	if err != nil {
		return err
	}
	dir, err := a.window.ChooseDirectory()
	if err != nil || dir == "" {
		return err
	}
	ctx, cancel := callContext()
	defer cancel()
	return e.NewSession(ctx, dir, name)
}

// GuardQuit tells whether the frontend confirms the closing of the window:
// while it does, closing it emits "quit-requested" instead, which the
// frontend answers with AckQuit.
func (a *App) GuardQuit(on bool) {
	a.window.GuardQuit(on)
}

// AckQuit tells that the frontend shows the confirmation of a close request.
func (a *App) AckQuit() {
	a.window.AckQuit()
}

// Quit closes the application after the user confirmed it. The sessions of
// the private server keep running.
func (a *App) Quit() {
	a.window.Quit()
}

// ChooseFiles asks for files to attach to a message and returns their absolute
// paths. Cancelling the dialog returns no path and is not an error.
func (a *App) ChooseFiles() ([]string, error) {
	return a.window.ChooseFiles()
}

// OpenTerminal shows a session in a new terminal of cols by rows cells and
// returns its ID. The output starts to flow after TermReady.
func (a *App) OpenTerminal(pid, cols, rows int) (int, error) {
	terms, err := a.terminals()
	if err != nil {
		return 0, err
	}
	ctx, cancel := callContext()
	defer cancel()
	return terms.Open(ctx, pid, cols, rows)
}

// TermReady tells that the frontend listens to the events of a terminal.
func (a *App) TermReady(id int) error {
	terms, err := a.terminals()
	if err != nil {
		return err
	}
	return terms.Ready(id)
}

// TermInput writes what the user typed to a terminal.
func (a *App) TermInput(id int, data string) error {
	terms, err := a.terminals()
	if err != nil {
		return err
	}
	return terms.Input(id, data)
}

// TermResize changes the size of a terminal.
func (a *App) TermResize(id, cols, rows int) error {
	terms, err := a.terminals()
	if err != nil {
		return err
	}
	return terms.Resize(id, cols, rows)
}

// CloseTerminal closes a terminal.
func (a *App) CloseTerminal(id int) error {
	terms, err := a.terminals()
	if err != nil {
		return err
	}
	return terms.Close(id)
}

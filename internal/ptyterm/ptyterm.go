// Package ptyterm shows a tmux pane in a pseudo-terminal that the frontend
// renders: it attaches a tmux client to a temporary view session and relays
// the bytes in both directions.
package ptyterm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"claude-tower/internal/hosting"
	"claude-tower/internal/tmux"

	"github.com/creack/pty"
)

const (
	closeTimeout = 10 * time.Second
	// maxCells is the largest number of columns or rows of a terminal.
	maxCells = 1000
)

var errClosed = errors.New("gestionnaire de terminaux arrêté")

func checkSize(cols, rows int) error {
	if cols <= 0 || rows <= 0 || cols > maxCells || rows > maxCells {
		return fmt.Errorf("taille de terminal invalide : %dx%d", cols, rows)
	}
	return nil
}

// Emitter sends an event to the frontend.
type Emitter interface {
	Emit(name string, data any)
}

// PaneSource finds the tmux pane hosting a session.
type PaneSource interface {
	PaneOf(pid int) (tmux.Pane, bool)
}

// Manager owns the open terminals. It is safe for concurrent use.
type Manager struct {
	panes PaneSource
	emit  Emitter
	env   []string
	start func(cmd *exec.Cmd, size *pty.Winsize) (*os.File, error)

	mu     sync.Mutex
	next   int
	closed bool
	terms  map[int]*terminal
}

// NewManager returns a Manager whose terminals inherit env, adjusted so that
// tmux can attach: TERM and COLORTERM are set and the locale is UTF-8.
func NewManager(panes PaneSource, emit Emitter, env []string) *Manager {
	return &Manager{
		panes: panes,
		emit:  emit,
		env:   terminalEnv(env),
		start: pty.StartWithSize,
		terms: map[int]*terminal{},
	}
}

type terminal struct {
	id       int
	srv      tmux.Server
	view     tmux.ViewSession
	cmd      *exec.Cmd
	file     *os.File
	out      *stream
	stop     chan struct{}
	readDone chan struct{}
	flushed  chan struct{}
}

// terminalEnv returns env without the variables that describe an enclosing
// tmux, with TERM and COLORTERM forced and a UTF-8 locale.
func terminalEnv(env []string) []string {
	out := make([]string, 0, len(env)+2)
	for _, e := range env {
		name, _, _ := strings.Cut(e, "=")
		switch name {
		case "TERM", "COLORTERM", "TMUX", "TMUX_PANE":
		default:
			out = append(out, e)
		}
	}
	out = append(out, "TERM=xterm-256color", "COLORTERM=truecolor")
	return hosting.WithUTF8Locale(out)
}

// Open shows the pane of the session with the given PID in a new terminal of
// cols by rows cells and returns its ID. Nothing reaches the frontend until
// Ready is called for that ID.
func (m *Manager) Open(ctx context.Context, pid, cols, rows int) (int, error) {
	if err := checkSize(cols, rows); err != nil {
		return 0, err
	}
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return 0, errClosed
	}
	pane, ok := m.panes.PaneOf(pid)
	if !ok {
		return 0, errors.New("session hors tmux ou inconnue")
	}
	view, err := pane.Server.NewViewSession(ctx, pane)
	if err != nil {
		return 0, err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		cleanupView(ctx, pane.Server, view)
		return 0, errClosed
	}
	m.next++
	id := m.next
	m.mu.Unlock()
	t, err := m.attach(id, pane.Server, view, cols, rows)
	if err != nil {
		cleanupView(ctx, pane.Server, view)
		return 0, err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		t.close()
		return 0, errClosed
	}
	m.terms[id] = t
	m.mu.Unlock()
	return id, nil
}

// attach starts a tmux client on the view session in a pseudo-terminal. The
// view is marked destroy-unattached by the same client command, after the
// attach, because the option destroys a session that was never attached.
func (m *Manager) attach(id int, srv tmux.Server, view tmux.ViewSession, cols, rows int) (*terminal, error) {
	bin := srv.Bin
	if bin == "" {
		bin = "tmux"
	}
	cmd := exec.Command(bin, "-S", srv.Socket, "attach", "-t", view.ID,
		";", "set-option", "-t", view.ID, "destroy-unattached", "on")
	cmd.Env = m.env
	file, err := m.start(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		return nil, fmt.Errorf("attache à tmux : %w", err)
	}
	t := &terminal{
		id: id, srv: srv, view: view, cmd: cmd, file: file,
		out:      newStream(m.emit, fmt.Sprintf("term:%d:data", id), fmt.Sprintf("term:%d:exit", id)),
		stop:     make(chan struct{}),
		readDone: make(chan struct{}),
		flushed:  make(chan struct{}),
	}
	go t.read()
	go func() {
		t.out.run(t.stop)
		close(t.flushed)
	}()
	return t, nil
}

// read copies the output of the pseudo-terminal into the stream until the
// client exits, then reaps it.
func (t *terminal) read() {
	defer close(t.readDone)
	buf := make([]byte, maxPacket)
	for {
		n, err := t.file.Read(buf)
		if n > 0 {
			t.out.write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	t.out.finish()
	t.cmd.Wait()
}

func (m *Manager) get(id int) (*terminal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.terms[id]
	if !ok {
		return nil, fmt.Errorf("terminal %d inconnu", id)
	}
	return t, nil
}

// Ready tells that the frontend listens to the events of the terminal: the
// output held so far is sent, and so is the exit event if the client already
// exited.
func (m *Manager) Ready(id int) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}
	t.out.setReady()
	return nil
}

// Input writes what the user typed to the terminal.
func (m *Manager) Input(id int, data string) error {
	t, err := m.get(id)
	if err != nil {
		return err
	}
	_, err = t.file.WriteString(data)
	return err
}

// Resize changes the size of the terminal.
func (m *Manager) Resize(id, cols, rows int) error {
	if err := checkSize(cols, rows); err != nil {
		return err
	}
	t, err := m.get(id)
	if err != nil {
		return err
	}
	return pty.Setsize(t.file, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

// Close ends the terminal: it stops the client, destroys the view session
// and gives the pane back its previous active state. Closing an unknown or
// already closed terminal does nothing.
func (m *Manager) Close(id int) error {
	m.mu.Lock()
	t, ok := m.terms[id]
	delete(m.terms, id)
	m.mu.Unlock()
	if !ok {
		return nil
	}
	return t.close()
}

// CloseAll closes every open terminal and refuses the following Open calls.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	m.closed = true
	ids := make([]int, 0, len(m.terms))
	for id := range m.terms {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		m.Close(id)
	}
}

func (t *terminal) close() error {
	if t.cmd.Process != nil {
		t.cmd.Process.Kill()
	}
	t.file.Close()
	<-t.readDone
	close(t.stop)
	<-t.flushed
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	return cleanupView(ctx, t.srv, t.view)
}

// cleanupView destroys the view session, unless it is already gone, and
// restores the active pane of its window.
func cleanupView(ctx context.Context, srv tmux.Server, view tmux.ViewSession) error {
	var errs []error
	if err := srv.KillSession(ctx, view.ID); err != nil && viewExists(ctx, srv, view) {
		errs = append(errs, err)
	}
	if err := srv.RestoreActivePane(ctx, view); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func viewExists(ctx context.Context, srv tmux.Server, view tmux.ViewSession) bool {
	_, err := srv.Exec(ctx, nil, "has-session", "-t", view.ID)
	return err == nil
}

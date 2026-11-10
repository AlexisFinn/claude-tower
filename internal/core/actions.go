package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"claude-tower/internal/hosting"
	"claude-tower/internal/tmux"

	"github.com/charmbracelet/x/ansi"
)

// Screen is the visible content of a pane.
type Screen struct {
	Lines         []string
	Width, Height int
	Hash          string // covers Lines, Width and Height
}

// FocusResult tells how the session was brought to the user.
type FocusResult struct {
	Mode    string // "external" or "embedded"
	Warning string
}

// Screen captures the pane of the session in the last snapshot and returns
// its last maxLines lines, without the empty ones at the bottom, with the size
// of the pane at the time of the capture. A maxLines of zero or less returns
// every line.
func (c *Core) Screen(ctx context.Context, pid, maxLines int) (Screen, error) {
	pane, err := c.paneOfSession(pid)
	if err != nil {
		return Screen{}, err
	}
	text, width, height, err := pane.Server.Capture(ctx, pane.ID)
	if err != nil {
		return Screen{}, err
	}
	lines := tailLines(text, maxLines)
	c.noteScreen(pane.ID, lines)
	return Screen{Lines: lines, Width: width, Height: height, Hash: screenHash(lines, width, height)}, nil
}

// tailLines splits a capture into lines, drops the empty lines at the bottom
// and keeps the last max ones.
func tailLines(text string, max int) []string {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	if max > 0 && len(lines) > max {
		lines = lines[len(lines)-max:]
	}
	if lines == nil {
		lines = []string{}
	}
	return lines
}

func screenHash(lines []string, w, h int) string {
	sum := sha256.New()
	fmt.Fprintf(sum, "%d\x00%d\x00", w, h)
	for _, l := range lines {
		sum.Write([]byte(l))
		sum.Write([]byte{'\n'})
	}
	return hex.EncodeToString(sum.Sum(nil)[:12])
}

// Prompt sends text to the session. The registry file of the session is read
// again just before the paste, because its status may have changed since the
// last snapshot.
func (c *Core) Prompt(ctx context.Context, pid int, text string) error {
	if strings.TrimSpace(text) == "" {
		return errors.New("message vide")
	}
	pane, err := c.paneOfSession(pid)
	if err != nil {
		return err
	}
	if c.isPending(pid) {
		return errors.New("envoi refusé : la session démarre")
	}
	s, ok := c.opts.Registry.Session(pid)
	if !ok {
		return errors.New("la session n'existe plus")
	}
	if reason := PromptReason(true, s); reason != "" {
		return fmt.Errorf("envoi refusé : %s", reason)
	}
	panes, err := pane.Server.Panes(ctx)
	if err != nil {
		return err
	}
	if !hasPane(panes, pane.ID) {
		return errors.New("le volet de la session n'existe plus")
	}
	return pane.Server.Paste(ctx, pane.ID, text)
}

func hasPane(panes []tmux.Pane, id string) bool {
	for _, p := range panes {
		if p.ID == id {
			return true
		}
	}
	return false
}

// NewSession starts Claude Code in dir, in the private server. A name that is
// not empty once cleaned becomes the name of the session.
func (c *Core) NewSession(ctx context.Context, dir, name string) error {
	h := c.opts.Hosting
	if h == nil {
		return errors.New("hébergement indisponible")
	}
	env := h.NewSessionEnv(ctx)
	pane, err := h.NewSession(ctx, dir, hosting.NameArgs(name), env)
	if err != nil {
		return err
	}
	c.recordLaunch(pane, dir)
	return nil
}

// Bounds of the size Fit accepts: below them Claude Code cannot lay out its screen.
const (
	minFitCols  = 60
	minFitRows  = 12
	maxFitCells = 1000
)

// Fit resizes the window of a session of the private server to cols x rows,
// so that its card shows it without shrinking the text. A session that a
// client shows, such as the fullscreen view or a terminal of the user, keeps
// the size of that client, and so does a session with a view session not
// attached yet. Both are read again, because they may have appeared since the
// last snapshot. It reports whether the window has the size now, which is
// false when a client keeps it.
func (c *Core) Fit(ctx context.Context, pid, cols, rows int) (bool, error) {
	if cols < minFitCols || rows < minFitRows || cols > maxFitCells || rows > maxFitCells {
		return false, fmt.Errorf("taille refusée : %dx%d", cols, rows)
	}
	pane, err := c.paneOfSession(pid)
	if err != nil {
		return false, err
	}
	if private := c.privateSocket(); private == "" || pane.Server.Socket != private {
		return false, errors.New("seules les sessions lancées par claude-tower s'adaptent à leur carte")
	}
	// An attach session shows a background session that other clients may
	// show too: its size is theirs to choose.
	if strings.HasPrefix(pane.SessionName, tmux.AttachPrefix) {
		return false, errors.New("une session d'arrière-plan garde sa taille")
	}
	// A view session shows the pane in fullscreen, or is about to: its client
	// is not among Clients, which leaves view sessions out.
	views, err := pane.Server.ViewSessionsOf(ctx, pane)
	if err != nil || len(views) > 0 {
		return false, err
	}
	clients, err := pane.Server.Clients(ctx)
	if err != nil {
		return false, err
	}
	if pane.Attached(clients) {
		return false, nil
	}
	if err := pane.Server.FitWindow(ctx, pane.WindowID, cols, rows); err != nil {
		return false, err
	}
	return true, nil
}

// Kill stops a session of the private server, or a background session of
// Claude Code with `claude stop`.
func (c *Core) Kill(ctx context.Context, pid int) error {
	if c.opts.Hosting == nil {
		return errors.New("hébergement indisponible")
	}
	c.mu.Lock()
	id, background := c.snap.background[pid]
	c.mu.Unlock()
	if background {
		h := c.opts.Hosting
		return h.StopBackground(ctx, id, h.NewSessionEnv(ctx))
	}
	pane, err := c.paneOfSession(pid)
	if err != nil {
		return err
	}
	// The launch is dropped before the kill, so that a refresh meanwhile does
	// not report the killed session as a failed start.
	l := c.takeLaunch(pane.ID)
	if err := c.opts.Hosting.Kill(ctx, pane); err != nil {
		c.restoreLaunch(pane.ID, l)
		return err
	}
	return nil
}

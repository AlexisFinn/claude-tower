package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

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

// NewSession starts Claude Code in dir, in the private server.
func (c *Core) NewSession(ctx context.Context, dir string) error {
	h := c.opts.Hosting
	if h == nil {
		return errors.New("hébergement indisponible")
	}
	env := h.NewSessionEnv(ctx)
	pane, err := h.NewSession(ctx, dir, nil, env)
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
// the size of that client. The clients are read again, because one may have
// attached since the last snapshot.
func (c *Core) Fit(ctx context.Context, pid, cols, rows int) error {
	if cols < minFitCols || rows < minFitRows || cols > maxFitCells || rows > maxFitCells {
		return fmt.Errorf("taille refusée : %dx%d", cols, rows)
	}
	pane, err := c.paneOfSession(pid)
	if err != nil {
		return err
	}
	if private := c.privateSocket(); private == "" || pane.Server.Socket != private {
		return errors.New("seules les sessions lancées par claude-tower s'adaptent à leur carte")
	}
	if pane.Width == cols && pane.Height == rows {
		return nil
	}
	clients, err := pane.Server.Clients(ctx)
	if err != nil {
		return err
	}
	if pane.Attached(clients) {
		return nil
	}
	return pane.Server.FitWindow(ctx, pane.WindowID, cols, rows)
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

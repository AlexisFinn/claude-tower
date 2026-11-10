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
	_, err := h.NewSession(ctx, dir, nil, env)
	return err
}

// Kill stops a session of the private server.
func (c *Core) Kill(ctx context.Context, pid int) error {
	if c.opts.Hosting == nil {
		return errors.New("hébergement indisponible")
	}
	pane, err := c.paneOfSession(pid)
	if err != nil {
		return err
	}
	return c.opts.Hosting.Kill(ctx, pane)
}

package tmux

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
)

var viewSeq atomic.Uint64

// ViewSession is a session grouped with another one, created to show one pane
// in an embedded terminal without moving the user's own clients.
type ViewSession struct {
	ID             string // "$N"
	Name           string
	PreviousActive string // active pane of the window before the view selected another one
}

// NewViewSession creates a session grouped with the pane's session, showing
// the pane's window with the pane selected. The caller attaches to it and is
// responsible for making it disappear: it is not destroyed on its own.
func (s Server) NewViewSession(ctx context.Context, pane Pane) (ViewSession, error) {
	prev, err := s.Exec(ctx, nil, "display-message", "-p", "-t", pane.WindowID, "#{pane_id}")
	if err != nil {
		return ViewSession{}, fmt.Errorf("volet actif : %w", err)
	}
	name := fmt.Sprintf("%s%d-%d", ViewPrefix, os.Getpid(), viewSeq.Add(1))
	out, err := s.Exec(ctx, nil, "new-session", "-d", "-t", pane.SessionID, "-s", name, "-P", "-F", "#{session_id}")
	if err != nil {
		return ViewSession{}, fmt.Errorf("new-session : %w", err)
	}
	v := ViewSession{ID: strings.TrimSpace(string(out)), Name: name, PreviousActive: strings.TrimSpace(string(prev))}
	steps := [][]string{
		{"set-option", "-t", v.ID, "status", "off"},
		{"select-window", "-t", v.ID + ":" + pane.WindowID},
		{"select-pane", "-t", pane.ID},
	}
	for _, args := range steps {
		if _, err := s.Exec(ctx, nil, args...); err != nil {
			s.KillSession(ctx, v.ID)
			return ViewSession{}, fmt.Errorf("%s : %w", args[0], err)
		}
	}
	return v, nil
}

// RestoreActivePane selects again the pane that was active before the view
// session, if it still exists.
func (s Server) RestoreActivePane(ctx context.Context, v ViewSession) error {
	if v.PreviousActive == "" {
		return nil
	}
	if _, err := s.Exec(ctx, nil, "display-message", "-p", "-t", v.PreviousActive, "#{pane_id}"); err != nil {
		return nil
	}
	_, err := s.Exec(ctx, nil, "select-pane", "-t", v.PreviousActive)
	return err
}

// KillSession destroys the session with the given ID.
func (s Server) KillSession(ctx context.Context, sessionID string) error {
	_, err := s.Exec(ctx, nil, "kill-session", "-t", sessionID)
	return err
}

// SweepViewSessions destroys the view sessions that have no client.
func (s Server) SweepViewSessions(ctx context.Context) error {
	out, err := s.Exec(ctx, nil, "list-sessions", "-F", "#{session_id}\t#{session_name}\t#{session_attached}")
	if err != nil {
		return err
	}
	var firstErr error
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 3 || !strings.HasPrefix(f[1], ViewPrefix) {
			continue
		}
		if n, err := strconv.Atoi(f[2]); err != nil || n != 0 {
			continue
		}
		if err := s.KillSession(ctx, f[0]); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// ViewSessionsOf lists the IDs of the view sessions that the server groups,
// at this moment, with the pane's session.
func (s Server) ViewSessionsOf(ctx context.Context, pane Pane) ([]string, error) {
	out, err := s.Exec(ctx, nil, "list-sessions", "-F", "#{session_id}\t#{session_name}\t#{session_group}")
	if err != nil {
		return nil, fmt.Errorf("list-sessions : %w", err)
	}
	type entry struct{ id, name, group string }
	var entries []entry
	group := ""
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			continue
		}
		entries = append(entries, entry{f[0], f[1], f[2]})
		if f[0] == pane.SessionID {
			group = f[2]
		}
	}
	if group == "" {
		return nil, nil
	}
	var ids []string
	for _, e := range entries {
		if strings.HasPrefix(e.name, ViewPrefix) && e.group == group {
			ids = append(ids, e.id)
		}
	}
	return ids, nil
}

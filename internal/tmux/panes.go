package tmux

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// ViewPrefix starts the name of every session created to show a pane in an
// embedded terminal. Such sessions are hidden from the pane and client lists.
const ViewPrefix = "claude-tower-view-"

// Pane is one tmux pane.
type Pane struct {
	Server        Server
	ID            string // "%N"
	PID           int    // pane_pid
	SessionID     string // "$N"
	SessionName   string
	WindowID      string // "@N"
	Active        bool   // pane_active
	SessionGroup  string // session_group, "" when ungrouped
	Width, Height int
}

// Client is one tmux client.
type Client struct {
	Name         string
	PID          int
	SessionID    string
	SessionGroup string
	Activity     int64
}

const (
	paneFormat   = "#{pane_id}\t#{pane_pid}\t#{session_id}\t#{session_name}\t#{window_id}\t#{pane_active}\t#{session_group}\t#{pane_width}\t#{pane_height}"
	clientFormat = "#{client_name}\t#{client_pid}\t#{session_id}\t#{session_name}\t#{session_group}\t#{client_activity}"
)

// Panes lists the panes of every session except the view sessions. A pane
// shown by several sessions of a group appears once.
func (s Server) Panes(ctx context.Context) ([]Pane, error) {
	out, err := s.Exec(ctx, nil, "list-panes", "-a", "-F", paneFormat)
	if err != nil {
		return nil, err
	}
	return parsePanes(s, string(out)), nil
}

func parsePanes(s Server, out string) []Pane {
	var panes []Pane
	seen := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 9 || strings.HasPrefix(f[3], ViewPrefix) || seen[f[0]] {
			continue
		}
		pid, err1 := strconv.Atoi(f[1])
		w, err2 := strconv.Atoi(f[7])
		h, err3 := strconv.Atoi(f[8])
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		seen[f[0]] = true
		panes = append(panes, Pane{
			Server: s, ID: f[0], PID: pid, SessionID: f[2], SessionName: f[3],
			WindowID: f[4], Active: f[5] == "1", SessionGroup: f[6], Width: w, Height: h,
		})
	}
	return panes
}

// Clients lists the attached clients except those of the view sessions.
func (s Server) Clients(ctx context.Context) ([]Client, error) {
	out, err := s.Exec(ctx, nil, "list-clients", "-F", clientFormat)
	if err != nil {
		return nil, err
	}
	return parseClients(string(out)), nil
}

func parseClients(out string) []Client {
	var clients []Client
	for _, line := range strings.Split(out, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 6 || strings.HasPrefix(f[3], ViewPrefix) {
			continue
		}
		pid, err1 := strconv.Atoi(f[1])
		activity, err2 := strconv.ParseInt(f[5], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		clients = append(clients, Client{Name: f[0], PID: pid, SessionID: f[2], SessionGroup: f[4], Activity: activity})
	}
	return clients
}

// Clients returns the clients attached to the pane's session or to a session
// of the same group, whatever window each client currently shows.
func (p Pane) Clients(all []Client) []Client {
	var out []Client
	for _, c := range all {
		if c.SessionID == p.SessionID || (p.SessionGroup != "" && c.SessionGroup == p.SessionGroup) {
			out = append(out, c)
		}
	}
	return out
}

// Attached reports whether at least one client shows the pane's session.
func (p Pane) Attached(all []Client) bool {
	return len(p.Clients(all)) > 0
}

// Capture returns the visible screen of the pane with its escape sequences.
// Lines are separated by "\n" and the text ends with one.
func (s Server) Capture(ctx context.Context, pane string) (string, error) {
	out, err := s.Exec(ctx, nil, "capture-pane", "-p", "-e", "-t", pane)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// SwitchClient makes the client show the pane.
func (s Server) SwitchClient(ctx context.Context, client, pane string) error {
	if _, err := s.Exec(ctx, nil, "switch-client", "-c", client, "-t", pane); err != nil {
		return fmt.Errorf("switch-client : %w", err)
	}
	return nil
}

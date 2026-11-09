package core

import (
	"context"

	"claude-tower/internal/registry"
	"claude-tower/internal/tmux"
	"claude-tower/internal/view"
)

// Card is what the interface shows for one session.
type Card struct {
	PID, Order   int // Order is the index in start time, then PID order
	Name         string
	State        string // "waiting", "busy", "background", "idle" or "unknown"
	Label        string
	Since        string
	Cwd          string
	Activity     string
	InTmux       bool
	Private      bool // the session runs in the private server
	Attached     bool // a client of the user shows the session
	CanPrompt    bool
	PromptReason string // why CanPrompt is false
}

// Cards runs a full refresh cycle and returns one card per session, in a
// stable order. The cycle becomes the last snapshot.
func (c *Core) Cards(ctx context.Context) ([]Card, error) {
	snap, err := c.refresh(ctx)
	if err != nil {
		return nil, err
	}
	now := c.opts.Now()
	cards := make([]Card, 0, len(snap.sessions))
	for i, s := range snap.sessions {
		act := c.finder.Last(s.SessionID)
		s.TurnEnded = act.TurnEnded
		card := Card{
			PID:      s.PID,
			Order:    i,
			Name:     view.DisplayName(s),
			State:    stateName(view.DisplayOf(s)),
			Label:    view.Label(s),
			Since:    view.Duration(s.StatusSince, now),
			Cwd:      view.ShortPath(s.Cwd, c.opts.Home),
			Activity: act.Text,
		}
		pane, inTmux := snap.panes[s.PID]
		card.InTmux = inTmux
		if inTmux {
			card.Private = snap.private != "" && pane.Server.Socket == snap.private
			card.Attached = pane.Attached(snap.clients[pane.Server.Socket])
		}
		card.PromptReason = PromptReason(inTmux, s)
		card.CanPrompt = card.PromptReason == ""
		cards = append(cards, card)
	}
	return cards, nil
}

func stateName(d view.Display) string {
	switch d {
	case view.DisplayWaiting:
		return "waiting"
	case view.DisplayBusy:
		return "busy"
	case view.DisplayBackground:
		return "background"
	case view.DisplayIdle:
		return "idle"
	}
	return "unknown"
}

// promptable lists the registry statuses in which a prompt may be sent.
func promptable(rawStatus string) bool {
	switch rawStatus {
	case "idle", "busy", "shell":
		return true
	}
	return false
}

// PromptReason explains why a prompt cannot be sent to the session, and is
// empty when it can. Only the statuses known to accept input are allowed.
func PromptReason(inTmux bool, s registry.Session) string {
	switch {
	case !inTmux:
		return "session hors tmux"
	case s.State == registry.StateWaiting && s.WaitingFor == "permission prompt":
		return "attend une autorisation"
	case s.State == registry.StateWaiting && s.WaitingFor != "":
		return "attend une réponse (" + s.WaitingFor + ")"
	case s.State == registry.StateWaiting:
		return "attend une réponse"
	case !promptable(s.RawStatus):
		return "état inconnu"
	}
	return ""
}

// paneOfSession returns the pane of the session in the last snapshot.
func (c *Core) paneOfSession(pid int) (tmux.Pane, error) {
	pane, ok := c.PaneOf(pid)
	if !ok {
		return tmux.Pane{}, errNotInTmux
	}
	return pane, nil
}

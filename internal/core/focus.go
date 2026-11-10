package core

import (
	"context"
	"errors"
	"fmt"

	"claude-tower/internal/tmux"
)

type focusKind int

const (
	focusEmbedded   focusKind = iota // show the session in the embedded terminal
	focusSwitch                      // raise a client's terminal, then switch the client to the pane
	focusRaiseOwner                  // raise the terminal of a session outside tmux
)

// focusPlan is how a session should be shown.
type focusPlan struct {
	kind   focusKind
	client tmux.Client // the client to raise and switch, for focusSwitch
}

// planFocus decides how to show a session: through the most recently used
// client when one shows it and windows can be raised, in the embedded
// terminal otherwise, and through its own terminal when it is outside tmux.
func planFocus(inTmux bool, clients []tmux.Client, supported bool) focusPlan {
	if !inTmux {
		return focusPlan{kind: focusRaiseOwner}
	}
	if len(clients) == 0 || !supported {
		return focusPlan{kind: focusEmbedded}
	}
	best := clients[0]
	for _, c := range clients[1:] {
		if c.Activity > best.Activity {
			best = c
		}
	}
	return focusPlan{kind: focusSwitch, client: best}
}

// Focus brings the session to the user. When it cannot raise a window it asks
// for the embedded terminal, with the reason in Warning, and switches no
// client.
func (c *Core) Focus(ctx context.Context, pid int) (FocusResult, error) {
	c.snapMu.Lock()
	snap := c.snap
	c.snapMu.Unlock()
	pane, inTmux := snap.panes[pid]
	if !inTmux && !knownSession(snap, pid) {
		return FocusResult{}, errors.New("session inconnue")
	}
	var clients []tmux.Client
	if inTmux {
		clients = pane.Clients(snap.clients[pane.Server.Socket])
	}
	plan := planFocus(inTmux, clients, c.opts.Supported())
	switch plan.kind {
	case focusSwitch:
		return c.raiseAndSwitch(ctx, snap, pane, plan.client), nil
	case focusRaiseOwner:
		if err := c.opts.Raise(snap.tree.Ancestors(pid)); err != nil {
			return FocusResult{}, fmt.Errorf("impossible d'afficher le terminal de cette session : %w", err)
		}
		return FocusResult{Mode: "external"}, nil
	}
	return FocusResult{Mode: "embedded"}, nil
}

func (c *Core) raiseAndSwitch(ctx context.Context, snap snapshot, pane tmux.Pane, client tmux.Client) FocusResult {
	if err := c.opts.Raise(snap.tree.Ancestors(client.PID)); err != nil {
		return FocusResult{Mode: "embedded", Warning: err.Error()}
	}
	if err := pane.Server.SwitchClient(ctx, client.Name, pane.ID); err != nil {
		return FocusResult{Mode: "embedded", Warning: err.Error()}
	}
	return FocusResult{Mode: "external"}
}

func knownSession(snap snapshot, pid int) bool {
	for _, s := range snap.sessions {
		if s.PID == pid {
			return true
		}
	}
	return false
}

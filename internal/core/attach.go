package core

import (
	"context"
	"strings"
	"time"

	"claude-tower/internal/locate"
	"claude-tower/internal/registry"
	"claude-tower/internal/tmux"
)

// attachRetry is how long the application waits before it attaches again to
// a background session whose private attach session failed or ended.
const attachRetry = 30 * time.Second

// attachTimeout bounds the creation or removal of one attach session.
const attachTimeout = 20 * time.Second

// isBackground reports whether the session runs in the background daemon of
// Claude Code, without a terminal of its own.
func isBackground(s registry.Session) bool {
	return s.Kind == "bg" || s.Kind == "background"
}

// backgroundID returns the short id that `claude attach` and `claude stop`
// take: the first eight characters of the session ID.
func backgroundID(s registry.Session) string {
	if len(s.SessionID) < 8 {
		return s.SessionID
	}
	return s.SessionID[:8]
}

// attachTargets reports whether target, the argument of a `claude attach`
// client, designates the session.
func attachTargets(target string, s registry.Session) bool {
	return target != "" && (target == backgroundID(s) || target == s.SessionID || target == s.Name)
}

// matchBackground adds to matched the pane that shows each background session
// not matched yet: a pane of the user's servers running a `claude attach`
// client of the session first, then the attach session of the private server.
// It returns the short id of every background session, keyed by PID.
func matchBackground(sessions []registry.Session, panes []tmux.Pane, tree locate.Tree, clients locate.Attachments, private string, matched map[int]tmux.Pane) map[int]string {
	byPID := make(map[int]tmux.Pane, len(panes))
	byName := map[string]tmux.Pane{}
	for _, p := range panes {
		byPID[p.PID] = p
		if p.Server.Socket == private && strings.HasPrefix(p.SessionName, tmux.AttachPrefix) {
			byName[p.SessionName] = p
		}
	}
	ids := map[int]string{}
	for _, s := range sessions {
		if !isBackground(s) {
			continue
		}
		id := backgroundID(s)
		ids[s.PID] = id
		if _, ok := matched[s.PID]; ok {
			continue
		}
		if p, ok := userAttachPane(s, byPID, tree, clients, private); ok {
			matched[s.PID] = p
		} else if p, ok := byName[tmux.AttachPrefix+id]; ok {
			matched[s.PID] = p
		}
	}
	return ids
}

// userAttachPane returns a pane of the user's servers whose process tree runs
// a `claude attach` client of s.
func userAttachPane(s registry.Session, byPID map[int]tmux.Pane, tree locate.Tree, clients locate.Attachments, private string) (tmux.Pane, bool) {
	for pid, target := range clients {
		if !attachTargets(target, s) {
			continue
		}
		for _, a := range tree.Ancestors(pid) {
			if p, ok := byPID[a]; ok && p.Server.Socket != private {
				return p, true
			}
		}
	}
	return tmux.Pane{}, false
}

// isAttachPane reports whether p is an attach session of the private server.
func isAttachPane(p tmux.Pane, private string) bool {
	return p.Server.Socket == private && strings.HasPrefix(p.SessionName, tmux.AttachPrefix)
}

// attachWork lists what syncAttachments must do after a snapshot.
type attachWork struct {
	create []registry.Session // background sessions without any pane
	remove []tmux.Pane        // private attach sessions no longer needed
}

// planAttachments compares the background sessions with the private attach
// sessions: a session shown by no pane gets one, unless an attempt for it is
// recent; an attach session is removed when its session is gone or when a
// pane of the user's servers shows the session.
func planAttachments(snap snapshot, tried map[string]time.Time, now time.Time) attachWork {
	var w attachWork
	needed := map[string]bool{}
	for _, s := range snap.sessions {
		id, ok := snap.background[s.PID]
		if !ok {
			continue
		}
		p, shown := snap.panes[s.PID]
		if shown && !isAttachPane(p, snap.private) {
			continue
		}
		needed[id] = true
		if !shown {
			if t, ok := tried[id]; !ok || now.Sub(t) >= attachRetry {
				w.create = append(w.create, s)
			}
		}
	}
	for _, p := range snap.listedPrivate {
		if isAttachPane(p, snap.private) && !needed[strings.TrimPrefix(p.SessionName, tmux.AttachPrefix)] {
			w.remove = append(w.remove, p)
		}
	}
	return w
}

// syncAttachments starts and removes private attach sessions as
// planAttachments decides, in the background so that Cards does not wait for
// them. A session already being handled is skipped.
func (c *Core) syncAttachments(snap snapshot) {
	h := c.opts.Hosting
	if h == nil || snap.private == "" || (!snap.privateOK && !snap.privateGone) {
		return
	}
	now := c.opts.Now()
	c.mu.Lock()
	work := planAttachments(snap, c.attachTried, now)
	var create []registry.Session
	for _, s := range work.create {
		id := snap.background[s.PID]
		if c.attaching[id] {
			continue
		}
		c.attaching[id] = true
		c.attachTried[id] = now
		create = append(create, s)
	}
	c.mu.Unlock()
	for _, s := range create {
		go c.attach(s, snap.background[s.PID])
	}
	for _, p := range work.remove {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), attachTimeout)
			defer cancel()
			p.Server.KillSession(ctx, p.SessionID)
		}()
	}
}

func (c *Core) attach(s registry.Session, id string) {
	defer func() {
		c.mu.Lock()
		delete(c.attaching, id)
		c.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), attachTimeout)
	defer cancel()
	h := c.opts.Hosting
	// A failure leaves the card without a screen until attachRetry has passed.
	h.NewAttachSession(ctx, s.Cwd, id, h.NewSessionEnv(ctx))
}

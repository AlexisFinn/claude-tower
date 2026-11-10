package core

import (
	"context"
	"sort"
	"strings"
	"time"

	"claude-tower/internal/locate"
	"claude-tower/internal/registry"
	"claude-tower/internal/tmux"
)

// attachRetry is how long the application waits before it attaches again to
// a background session whose private attach session failed or ended; the
// wait doubles at each new attempt, up to maxAttachRetry.
const (
	attachRetry    = 30 * time.Second
	maxAttachRetry = 10 * time.Minute
)

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
// client, designates s. A name designates a session only when no other
// session has it, nor has it as short id.
func attachTargets(target string, s registry.Session, ambiguous map[string]bool) bool {
	if target == "" {
		return false
	}
	return target == backgroundID(s) || target == s.SessionID || (target == s.Name && !ambiguous[target])
}

// ambiguousNames returns the names that designate several sessions: names
// shared by sessions, and names equal to the short id of another session.
func ambiguousNames(sessions []registry.Session) map[string]bool {
	count := map[string]int{}
	for _, s := range sessions {
		if s.Name != "" {
			count[s.Name]++
		}
	}
	out := map[string]bool{}
	for _, s := range sessions {
		if count[s.Name] > 1 {
			out[s.Name] = true
		}
		if id := backgroundID(s); count[id] > 0 && s.Name != id {
			out[id] = true
		}
	}
	return out
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
		if isAttachPane(p, private) {
			byName[p.SessionName] = p
		}
	}
	// Clients are tried in PID order, so that a session shown by several
	// panes keeps the same one from a refresh to the next.
	pids := make([]int, 0, len(clients))
	for pid := range clients {
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	ambiguous := ambiguousNames(sessions)
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
		if p, ok := userAttachPane(s, pids, byPID, tree, clients, private, ambiguous); ok {
			matched[s.PID] = p
		} else if p, ok := byName[tmux.AttachPrefix+id]; ok {
			matched[s.PID] = p
		}
	}
	return ids
}

// userAttachPane returns a pane of the user's servers whose process tree runs
// a `claude attach` client of s.
func userAttachPane(s registry.Session, pids []int, byPID map[int]tmux.Pane, tree locate.Tree, clients locate.Attachments, private string, ambiguous map[string]bool) (tmux.Pane, bool) {
	for _, pid := range pids {
		if !attachTargets(clients[pid], s, ambiguous) {
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

// attachAttempt records the attempts to attach to one background session.
type attachAttempt struct {
	at    time.Time
	count int // consecutive attempts without the attach session being seen
}

// attachDelay is the wait before the next attempt after count failed ones:
// attachRetry, doubled at each attempt, up to maxAttachRetry.
func attachDelay(count int) time.Duration {
	d := attachRetry
	for i := 1; i < count && d < maxAttachRetry; i++ {
		d *= 2
	}
	return min(d, maxAttachRetry)
}

// attachWork lists what syncAttachments must do after a snapshot.
type attachWork struct {
	create []registry.Session // background sessions without any pane
	remove []tmux.Pane        // private attach sessions not needed in this snapshot
	shown  []string           // ids shown by their attach session
}

// planAttachments compares the background sessions with the private attach
// sessions: a session shown by no pane gets one, unless the last attempt is
// too recent or the snapshot may have missed a pane of the user; an attach
// session is not needed when its session is gone or when a pane of the user's
// servers shows the session.
func planAttachments(snap snapshot, tried map[string]attachAttempt, now time.Time) attachWork {
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
		if shown {
			w.shown = append(w.shown, id)
			continue
		}
		if a, ok := tried[id]; snap.userComplete && (!ok || now.Sub(a.at) >= attachDelay(a.count)) {
			w.create = append(w.create, s)
		}
	}
	for _, p := range snap.listedPrivate {
		if isAttachPane(p, snap.private) && !needed[strings.TrimPrefix(p.SessionName, tmux.AttachPrefix)] {
			w.remove = append(w.remove, p)
		}
	}
	return w
}

// removeAfter is the number of consecutive snapshots in which an attach
// session must be unneeded before it is removed, so that a registry file read
// while being written does not remove an attach session still in use.
const removeAfter = 2

// syncAttachments starts and removes private attach sessions as
// planAttachments decides, in the background so that Cards does not wait for
// them. A session already being created or removed is skipped.
func (c *Core) syncAttachments(snap snapshot) {
	h := c.opts.Hosting
	if h == nil || snap.private == "" || (!snap.privateOK && !snap.privateGone) {
		return
	}
	now := c.opts.Now()
	c.mu.Lock()
	work := planAttachments(snap, c.attachTried, now)
	for _, id := range work.shown {
		delete(c.attachTried, id)
	}
	live := map[string]bool{}
	for _, id := range snap.background {
		live[id] = true
	}
	for id := range c.attachTried {
		if !live[id] {
			delete(c.attachTried, id)
		}
	}
	var create []registry.Session
	for _, s := range work.create {
		id := snap.background[s.PID]
		if c.attaching[id] {
			continue
		}
		c.attaching[id] = true
		a := c.attachTried[id]
		c.attachTried[id] = attachAttempt{at: now, count: a.count + 1}
		create = append(create, s)
	}
	unneeded := map[string]int{}
	var remove []tmux.Pane
	for _, p := range work.remove {
		unneeded[p.ID] = c.unneeded[p.ID] + 1
		if unneeded[p.ID] >= removeAfter && !c.removing[p.ID] {
			c.removing[p.ID] = true
			remove = append(remove, p)
		}
	}
	c.unneeded = unneeded
	c.mu.Unlock()
	for _, s := range create {
		go c.attach(s, snap.background[s.PID])
	}
	for _, p := range remove {
		go c.removeAttach(p)
	}
}

// removeAttach destroys the attach session of p.
func (c *Core) removeAttach(p tmux.Pane) {
	defer func() {
		c.mu.Lock()
		delete(c.removing, p.ID)
		c.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), attachTimeout)
	defer cancel()
	p.Server.KillSession(ctx, p.SessionID)
}

// attach creates the attach session of the background session s.
func (c *Core) attach(s registry.Session, id string) {
	defer func() {
		c.mu.Lock()
		delete(c.attaching, id)
		c.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), attachTimeout)
	defer cancel()
	h := c.opts.Hosting
	// A failure leaves the card without a screen until the next attempt.
	h.NewAttachSession(ctx, s.Cwd, id, h.NewSessionEnv(ctx))
}

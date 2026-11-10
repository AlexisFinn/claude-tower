package core

import (
	"sort"
	"strings"
	"time"

	"claude-tower/internal/tmux"
	"claude-tower/internal/view"

	"github.com/charmbracelet/x/ansi"
)

// startupGrace is how long a pane of the private server may run without a
// registry entry before its card asks for the user. Claude Code registers
// within a few seconds unless a screen such as onboarding, folder trust or
// login stops it before.
const startupGrace = 5 * time.Second

// failureLines is the number of screen lines kept to describe a failed start.
const failureLines = 4

// StartFailure describes a session started by the application that ended
// before Claude Code registered it.
type StartFailure struct {
	Name  string
	Cwd   string
	Lines []string // last non-empty lines of its screen, oldest first; may be empty
}

// launch follows a session started by NewSession until Claude Code registers
// it or it ends.
type launch struct {
	name, cwd string
	seq       uint64   // value of Core.launchSeq when the launch was recorded
	lines     []string // last screen lines seen, without escape sequences
}

// pendingPanes returns the panes of the private server that host no
// registered session, attach sessions aside, ordered by creation time, then pane ID.
func pendingPanes(panes []tmux.Pane, matched map[int]tmux.Pane, private string) []tmux.Pane {
	if private == "" {
		return nil
	}
	used := make(map[string]bool, len(matched))
	for _, p := range matched {
		if p.Server.Socket == private {
			used[p.ID] = true
		}
	}
	var out []tmux.Pane
	for _, p := range panes {
		if p.Server.Socket == private && !used[p.ID] && !isAttachPane(p, private) {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].Created.Equal(out[j].Created) {
			return out[i].Created.Before(out[j].Created)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// recordLaunch follows the pane that NewSession has just created.
func (c *Core) recordLaunch(pane tmux.Pane, dir string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.launchSeq++
	c.launches[pane.ID] = &launch{name: pane.SessionName, cwd: view.ShortPath(dir, c.opts.Home), seq: c.launchSeq}
}

// takeLaunch stops following a pane and returns its launch, nil when the pane
// was not followed.
func (c *Core) takeLaunch(paneID string) *launch {
	c.mu.Lock()
	defer c.mu.Unlock()
	l := c.launches[paneID]
	delete(c.launches, paneID)
	return l
}

// restoreLaunch follows a pane again after takeLaunch.
func (c *Core) restoreLaunch(paneID string, l *launch) {
	if l == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.launches[paneID] = l
}

// updateLaunches settles the launches against a snapshot whose listing began
// when launchSeq was seq: a launch whose pane is listed without being pending
// hosts a registered session and is done, and one whose pane is gone has
// failed. A launch recorded after the
// listing began may not be in it yet, and waits for the next snapshot. Nothing
// is settled while the private server fails without being gone. The caller
// holds c.mu.
func (c *Core) updateLaunches(snap snapshot, seq uint64) {
	if !snap.privateOK && !snap.privateGone {
		return
	}
	pending := make(map[string]bool, len(snap.pending))
	for _, p := range snap.pending {
		pending[p.ID] = true
	}
	for id, l := range c.launches {
		switch {
		case pending[id]:
		case l.seq > seq:
		case snap.privateIDs[id]:
			delete(c.launches, id)
		default:
			c.failures = append(c.failures, StartFailure{Name: l.name, Cwd: l.cwd, Lines: l.lines})
			delete(c.launches, id)
		}
	}
}

// noteScreen keeps the last lines of a launched pane, to describe its end if
// it fails.
func (c *Core) noteScreen(paneID string, lines []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	l, ok := c.launches[paneID]
	if !ok {
		return
	}
	var kept []string
	for i := len(lines) - 1; i >= 0 && len(kept) < failureLines; i-- {
		if t := strings.TrimSpace(ansi.Strip(lines[i])); t != "" {
			kept = append([]string{t}, kept...)
		}
	}
	if len(kept) > 0 {
		l.lines = kept
	}
}

// StartFailures returns the failed starts found since the last call.
func (c *Core) StartFailures() []StartFailure {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.failures
	c.failures = nil
	if out == nil {
		out = []StartFailure{}
	}
	return out
}

// isPending reports whether the pane of pid hosts no registered session yet.
func (c *Core) isPending(pid int) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.snap.pending {
		if p.PID == pid {
			return true
		}
	}
	return false
}

// pendingCard describes a pane of the private server where Claude Code has
// not registered yet.
func (c *Core) pendingCard(p tmux.Pane, order int, now time.Time, clients []tmux.Client) Card {
	state, label := "busy", "démarrage"
	if p.Created.IsZero() || now.Sub(p.Created) >= startupGrace {
		state, label = "waiting", "attend (démarrage)"
	}
	since := ""
	if !p.Created.IsZero() {
		since = view.Duration(p.Created, now)
	}
	return Card{
		PID:          p.PID,
		Order:        order,
		Name:         p.SessionName,
		State:        state,
		Label:        label,
		Since:        since,
		Cwd:          view.ShortPath(p.Cwd, c.opts.Home),
		Activity:     "Claude n'est pas encore prêt : ouvre la session pour voir son écran",
		InTmux:       true,
		Private:      true,
		Attached:     p.Attached(clients),
		PromptReason: "la session démarre",
	}
}

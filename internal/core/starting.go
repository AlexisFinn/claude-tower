package core

import (
	"sort"
	"strings"
	"sync"
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
	seq       uint64   // value of launchTracker.seq when the launch was recorded
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

// launchTracker follows the sessions started by the application until Claude
// Code registers them or they end, and collects the starts that failed. It is
// safe for concurrent use; its lock is independent of every other lock of Core.
type launchTracker struct {
	mu       sync.Mutex
	launches map[string]*launch // keyed by pane ID
	seq      uint64             // number of launches recorded so far
	failures []StartFailure
}

func newLaunchTracker() *launchTracker {
	return &launchTracker{launches: map[string]*launch{}}
}

// record follows the pane that NewSession has just created; cwd is the
// directory as shown to the user.
func (t *launchTracker) record(pane tmux.Pane, cwd string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq++
	t.launches[pane.ID] = &launch{name: pane.SessionName, cwd: cwd, seq: t.seq}
}

// sequence returns the sequence number of the last launch recorded. A refresh
// reads it before it lists the panes, and hands it to update.
func (t *launchTracker) sequence() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.seq
}

// take stops following a pane and returns its launch, nil when the pane was
// not followed.
func (t *launchTracker) take(paneID string) *launch {
	t.mu.Lock()
	defer t.mu.Unlock()
	l := t.launches[paneID]
	delete(t.launches, paneID)
	return l
}

// restore follows a pane again after take.
func (t *launchTracker) restore(paneID string, l *launch) {
	if l == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.launches[paneID] = l
}

// update settles the launches against a snapshot whose listing began when the
// sequence number was seq: a launch whose pane is listed without being
// pending hosts a registered session and is done, and one whose pane is gone
// has failed. A launch recorded after the listing began may not be in it yet,
// and waits for the next snapshot. Nothing is settled while the private server
// fails without being gone.
func (t *launchTracker) update(snap snapshot, seq uint64) {
	if !snap.privateOK && !snap.privateGone {
		return
	}
	pending := make(map[string]bool, len(snap.pending))
	for _, p := range snap.pending {
		pending[p.ID] = true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, l := range t.launches {
		switch {
		case pending[id]:
		case l.seq > seq:
		case hasPane(snap.listedPrivate, id):
			delete(t.launches, id)
		default:
			t.failures = append(t.failures, StartFailure{Name: l.name, Cwd: l.cwd, Lines: l.lines})
			delete(t.launches, id)
		}
	}
}

// noteScreen keeps the last lines of a launched pane, to describe its end if
// it fails.
func (t *launchTracker) noteScreen(paneID string, lines []string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	l, ok := t.launches[paneID]
	if !ok {
		return
	}
	var kept []string
	for i := len(lines) - 1; i >= 0 && len(kept) < failureLines; i-- {
		if text := strings.TrimSpace(ansi.Strip(lines[i])); text != "" {
			kept = append([]string{text}, kept...)
		}
	}
	if len(kept) > 0 {
		l.lines = kept
	}
}

// takeFailures returns the failed starts found since the last call.
func (t *launchTracker) takeFailures() []StartFailure {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.failures
	t.failures = nil
	return out
}

// StartFailures returns the failed starts found since the last call.
func (c *Core) StartFailures() []StartFailure {
	out := c.launches.takeFailures()
	if out == nil {
		out = []StartFailure{}
	}
	return out
}

// isPending reports whether the pane of pid hosts no registered session yet.
func (c *Core) isPending(pid int) bool {
	c.snapMu.Lock()
	defer c.snapMu.Unlock()
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

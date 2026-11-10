package core

import (
	"context"
	"sort"
	"strings"
	"sync"

	"claude-tower/internal/locate"
	"claude-tower/internal/registry"
	"claude-tower/internal/tmux"
)

// snapshot is the result of one refresh cycle.
type snapshot struct {
	sessions []registry.Session
	panes    map[int]tmux.Pane        // keyed by session PID
	clients  map[string][]tmux.Client // keyed by server socket
	tree     locate.Tree
	private  string
	// pending lists the panes of the private server that host no registered
	// session; panes also holds them, keyed by pane PID.
	pending     []tmux.Pane
	privateOK   bool // the private server answered
	privateGone bool // the private server is not running
	// listedPrivate holds every pane the private server listed.
	listedPrivate []tmux.Pane
	// background holds the short id of each background session, keyed by PID.
	background map[int]string
	// userComplete reports that every server of the user answered and the
	// attach clients were read, so that a session without pane really has none.
	userComplete bool
}

// serverState is what one server reported.
type serverState struct {
	socket  string
	panes   []tmux.Pane
	clients []tmux.Client
	ok      bool
	err     error // why the server did not answer
}

// refresh reads the registry, the tmux servers and the process tree, matches
// sessions to panes and stores the result as the last snapshot.
func (c *Core) refresh(ctx context.Context) (snapshot, error) {
	sessions, err := c.opts.Registry.Sessions()
	if err != nil {
		return snapshot{}, err
	}
	sortSessions(sessions)
	seq := c.launches.sequence()
	private := c.privateSocket()
	states := c.readServers(ctx, c.servers(private))
	tree, err := c.opts.ReadTree(ctx, c.opts.Run)
	if err != nil {
		return snapshot{}, err
	}
	// The attach clients only serve background sessions. Without them,
	// background sessions fall back to the attach sessions of the private
	// server.
	var attached locate.Attachments
	userComplete := true
	if hasBackground(sessions) {
		var attachErr error
		attached, attachErr = c.opts.ReadAttachments(ctx, c.opts.Run)
		userComplete = attachErr == nil
	}
	var panes []tmux.Pane
	clients := map[string][]tmux.Client{}
	privateOK, privateGone := false, false
	var listedPrivate []tmux.Pane
	for _, st := range states {
		if !st.ok {
			if st.socket == private {
				privateGone = serverNotRunning(st.err)
			} else if !serverNotRunning(st.err) {
				userComplete = false
			}
			continue
		}
		if st.socket == private {
			privateOK = true
			listedPrivate = st.panes
		}
		panes = append(panes, st.panes...)
		clients[st.socket] = st.clients
	}
	snap := snapshot{
		sessions:      sessions,
		panes:         locate.Match(sessions, panes, tree),
		clients:       clients,
		tree:          tree,
		private:       private,
		privateOK:     privateOK,
		privateGone:   privateGone,
		listedPrivate: listedPrivate,
		userComplete:  userComplete,
	}
	snap.background = matchBackground(sessions, panes, tree, attached, private, snap.panes)
	// A pane whose PID is already a session's key would give two cards the
	// same PID: it gets none.
	for _, p := range pendingPanes(panes, snap.panes, private) {
		if _, taken := snap.panes[p.PID]; !taken && !knownSession(snap, p.PID) {
			snap.panes[p.PID] = p
			snap.pending = append(snap.pending, p)
		}
	}
	ids := make([]string, 0, len(sessions))
	for _, s := range sessions {
		ids = append(ids, s.SessionID)
	}
	c.finder.Retain(ids)
	c.retainTrackers(ids)
	c.snapMu.Lock()
	c.snap = snap
	c.snapMu.Unlock()
	c.launches.update(snap, seq)
	return snap, nil
}

// readServers queries every server in parallel, and the panes and the clients
// of one server in parallel too. A server that fails, such as an orphaned
// socket, is reported as not ok; its panes error takes precedence over its
// clients error.
func (c *Core) readServers(ctx context.Context, servers []tmux.Server) []serverState {
	states := make([]serverState, len(servers))
	var wg sync.WaitGroup
	for i, srv := range servers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var clients []tmux.Client
			var clientsErr error
			done := make(chan struct{})
			go func() {
				defer close(done)
				clients, clientsErr = srv.Clients(ctx)
			}()
			panes, err := srv.Panes(ctx)
			<-done
			if err == nil {
				err = clientsErr
			}
			if err != nil {
				states[i] = serverState{socket: srv.Socket, err: err}
				return
			}
			states[i] = serverState{socket: srv.Socket, panes: panes, clients: clients, ok: true}
		}()
	}
	wg.Wait()
	return states
}

// serverNotRunning reports whether err tells that no tmux server listens on
// the socket, as opposed to a server that answers too slowly.
func serverNotRunning(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no server running") || strings.Contains(msg, "error connecting")
}

// sortSessions orders sessions by start time, then PID, which keeps the cards
// where they are from one refresh to the next.
func sortSessions(ss []registry.Session) {
	sort.SliceStable(ss, func(i, j int) bool {
		if !ss[i].StartedAt.Equal(ss[j].StartedAt) {
			return ss[i].StartedAt.Before(ss[j].StartedAt)
		}
		return ss[i].PID < ss[j].PID
	})
}

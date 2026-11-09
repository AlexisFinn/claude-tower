package core

import (
	"context"
	"sort"
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
}

// serverState is what one server reported.
type serverState struct {
	socket  string
	panes   []tmux.Pane
	clients []tmux.Client
	ok      bool
}

// refresh reads the registry, the tmux servers and the process tree, matches
// sessions to panes and stores the result as the last snapshot.
func (c *Core) refresh(ctx context.Context) (snapshot, error) {
	sessions, err := c.opts.Registry.Sessions()
	if err != nil {
		return snapshot{}, err
	}
	sortSessions(sessions)
	private := c.privateSocket()
	states := c.readServers(ctx, c.servers(private))
	tree, err := c.opts.ReadTree(ctx, c.opts.Run)
	if err != nil {
		return snapshot{}, err
	}
	var panes []tmux.Pane
	clients := map[string][]tmux.Client{}
	for _, st := range states {
		if !st.ok {
			continue
		}
		panes = append(panes, st.panes...)
		clients[st.socket] = st.clients
	}
	snap := snapshot{
		sessions: sessions,
		panes:    locate.Match(sessions, panes, tree),
		clients:  clients,
		tree:     tree,
		private:  private,
	}
	ids := make([]string, 0, len(sessions))
	for _, s := range sessions {
		ids = append(ids, s.SessionID)
	}
	c.finder.Retain(ids)
	c.mu.Lock()
	c.snap = snap
	c.mu.Unlock()
	return snap, nil
}

// readServers queries every server in parallel. A server that fails, such as
// an orphaned socket, is reported as not ok.
func (c *Core) readServers(ctx context.Context, servers []tmux.Server) []serverState {
	states := make([]serverState, len(servers))
	var wg sync.WaitGroup
	for i, srv := range servers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			panes, err := srv.Panes(ctx)
			if err != nil {
				return
			}
			clients, err := srv.Clients(ctx)
			if err != nil {
				return
			}
			states[i] = serverState{socket: srv.Socket, panes: panes, clients: clients, ok: true}
		}()
	}
	wg.Wait()
	return states
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

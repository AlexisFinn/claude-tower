package core

import (
	"time"

	"claude-tower/internal/background"
	"claude-tower/internal/locate"
	"claude-tower/internal/registry"
)

// tracker returns the background tracker of a session, creating it on first
// use.
func (c *Core) tracker(sessionID string) *background.Tracker {
	c.bgMu.Lock()
	defer c.bgMu.Unlock()
	t := c.trackers[sessionID]
	if t == nil {
		t = background.NewTracker()
		c.trackers[sessionID] = t
	}
	return t
}

// retainTrackers drops the trackers of sessions that are not in sessionIDs.
func (c *Core) retainTrackers(sessionIDs []string) {
	keep := make(map[string]struct{}, len(sessionIDs))
	for _, id := range sessionIDs {
		keep[id] = struct{}{}
	}
	c.bgMu.Lock()
	defer c.bgMu.Unlock()
	for id := range c.trackers {
		if _, ok := keep[id]; !ok {
			delete(c.trackers, id)
		}
	}
}

// backgroundCounts reads what the transcript of the session gained and counts
// its running background agents and shells. A session without an ID has none;
// a transcript that is missing or unreadable leaves both counts Unknown.
func (c *Core) backgroundCounts(s registry.Session, tree locate.Tree, now time.Time) background.Counts {
	if s.SessionID == "" {
		return background.Counts{}
	}
	t := c.tracker(s.SessionID)
	path := c.finder.Path(s.SessionID)
	if path == "" || t.Update(path) != nil {
		return background.Counts{Agents: background.Unknown, Shells: background.Unknown}
	}
	return t.Counts(background.Query{
		StartedAt:   s.StartedAt,
		Now:         now,
		Descendants: func() []int { return tree.Descendants(s.PID) },
		ReadFDs:     c.opts.ReadFDs,
	})
}

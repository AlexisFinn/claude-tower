// Package sessionstate completes the sessions of the registry with what the
// state shown to the user depends on and the registry does not hold: whether
// the last turn has ended and whether background work is running. The cards of
// the application, `claude-tower list` and the tmux status bar all go through
// it, so that they show the same state.
package sessionstate

import (
	"claude-tower/internal/background"
	"claude-tower/internal/registry"
	"claude-tower/internal/transcript"
)

// Info is a session ready to be shown.
type Info struct {
	// Session has TurnEnded and HasBackground filled.
	Session registry.Session
	// Activity is the last visible activity of the transcript, "" when there
	// is none.
	Activity string
	// Background is the count of background work of the session, as given by
	// the caller.
	Background background.Counts
}

// Option changes how Enrich completes the sessions.
type Option func(*options)

type options struct {
	busyOnly bool
}

// BusyTranscriptsOnly makes Enrich read the transcript of the busy sessions
// only, the ones whose shown state depends on whether the last turn has
// ended. The other sessions keep a zero TurnEnded and an empty Activity.
func BusyTranscriptsOnly() Option {
	return func(o *options) { o.busyOnly = true }
}

// Enrich returns one Info per session, in the same order, without modifying
// sessions. counts holds the background work of the sessions by PID; a session
// missing from it has none. An unknown count does not mark a session as
// having background work.
func Enrich(sessions []registry.Session, finder *transcript.Finder, counts map[int]background.Counts, opts ...Option) []Info {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	infos := make([]Info, len(sessions))
	for i, s := range sessions {
		var act transcript.Activity
		if !o.busyOnly || s.State == registry.StateBusy {
			act = finder.Last(s.SessionID)
		}
		bg := counts[s.PID]
		s.TurnEnded = act.TurnEnded
		s.HasBackground = bg.Agents > 0 || bg.Shells > 0
		infos[i] = Info{Session: s, Activity: act.Text, Background: bg}
	}
	return infos
}

package hosting

import "time"

// SetStartupCheckDelay changes how long a new session must stay alive to count
// as started, and returns a function restoring the previous delay.
func SetStartupCheckDelay(d time.Duration) (restore func()) {
	prev := startupCheckDelay
	startupCheckDelay = d
	return func() { startupCheckDelay = prev }
}

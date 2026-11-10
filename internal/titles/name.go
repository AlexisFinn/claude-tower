// Package titles gives the sessions that Claude Code named by default a short
// title that says what they work on, generated from the requests of the user.
package titles

import (
	"path/filepath"
	"strings"
)

// IsDefaultName reports whether name is the name Claude Code gives a session
// nobody named: the base name of its directory, a dash and two to eight
// lowercase hexadecimal digits. An empty name counts as a default one.
func IsDefaultName(name, cwd string) bool {
	if name == "" {
		return true
	}
	rest, ok := strings.CutPrefix(name, filepath.Base(cwd)+"-")
	if !ok || len(rest) < 2 || len(rest) > 8 {
		return false
	}
	for _, r := range rest {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

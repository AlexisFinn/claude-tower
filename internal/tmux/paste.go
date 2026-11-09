package tmux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
)

// pasteDelay separates the end of the paste from the Enter key, so that the
// Enter arrives after the closing bracketed-paste sequence.
var pasteDelay = 50 * time.Millisecond

var (
	paneLocks sync.Map // socket + pane ID -> *sync.Mutex
	bufferSeq atomic.Uint64
)

// Sanitize makes text safe to paste: CRLF and lone CR become LF, then every
// control character (C0, DEL and C1, ESC included) is dropped except LF and
// TAB. The result does not depend on how a given tmux version filters pastes.
func Sanitize(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Map(func(r rune) rune {
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}

// Paste sends text to the pane as a bracketed paste, then presses Enter. The
// text goes through the standard input of load-buffer, so it is neither
// limited in size nor visible in a process list. Two pastes into the same
// pane never interleave.
func (s Server) Paste(ctx context.Context, pane, text string) error {
	text = Sanitize(text)
	if strings.TrimSpace(text) == "" {
		return errors.New("texte vide après filtrage des caractères de contrôle")
	}
	mu := paneLock(s.Socket, pane)
	mu.Lock()
	defer mu.Unlock()

	// An Enter sent to a pane in copy mode would be consumed by the mode.
	if _, err := s.Exec(ctx, nil, "if-shell", "-F", "-t", pane, "#{pane_in_mode}", "send-keys -t "+pane+" -X cancel"); err != nil {
		return fmt.Errorf("annulation du mode copie : %w", err)
	}
	buf := fmt.Sprintf("claude-tower-%d-%d", os.Getpid(), bufferSeq.Add(1))
	if _, err := s.Exec(ctx, strings.NewReader(text), "load-buffer", "-b", buf, "-"); err != nil {
		return fmt.Errorf("load-buffer : %w", err)
	}
	if _, err := s.Exec(ctx, nil, "paste-buffer", "-p", "-d", "-b", buf, "-t", pane); err != nil {
		s.Exec(ctx, nil, "delete-buffer", "-b", buf)
		return fmt.Errorf("paste-buffer : %w", err)
	}
	select {
	case <-time.After(pasteDelay):
	case <-ctx.Done():
		return ctx.Err()
	}
	if _, err := s.Exec(ctx, nil, "send-keys", "-t", pane, "Enter"); err != nil {
		return fmt.Errorf("send-keys : %w", err)
	}
	return nil
}

func paneLock(socket, pane string) *sync.Mutex {
	mu, _ := paneLocks.LoadOrStore(socket+"\x00"+pane, new(sync.Mutex))
	return mu.(*sync.Mutex)
}

package core

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"claude-tower/internal/titles"
)

// titleStore returns a title store whose claude always answers out, and the
// number of times it ran.
func titleStore(t *testing.T, out string) (*titles.Store, func() int) {
	t.Helper()
	var mu sync.Mutex
	runs := 0
	s := titles.New(titles.Options{
		Path:   filepath.Join(t.TempDir(), "titles.json"),
		Dir:    filepath.Join(t.TempDir(), "work"),
		Claude: func(context.Context, []string) (string, error) { return "/bin/claude", nil },
		Env:    func(context.Context) []string { return nil },
		Run: func(context.Context, titles.Call) ([]byte, error) {
			mu.Lock()
			defer mu.Unlock()
			runs++
			return []byte(out), nil
		},
	})
	return s, func() int { mu.Lock(); defer mu.Unlock(); return runs }
}

func titleOf(t *testing.T, c *Core, pid int) (name, title string) {
	t.Helper()
	cards, err := c.Cards(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, card := range cards {
		if card.PID == pid {
			return card.Name, card.Title
		}
	}
	t.Fatalf("no card for %d", pid)
	return "", ""
}

const promptLine = `{"type":"user","message":{"content":"Corrige l'export des objectifs"}}`

func TestCardTitleChosenByUserIsNeverGenerated(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","name":"Refonte export","cwd":"/home/u/zest","status":"idle"`)
	e.transcript(idA, promptLine)
	store, runs := titleStore(t, "Titre généré")
	c := e.core(func(o *Options) { o.Titles = store })
	for range 2 {
		name, title := titleOf(t, c, 100)
		store.Wait()
		if name != "Refonte export" || title != "Refonte export" {
			t.Errorf("name %q, title %q", name, title)
		}
	}
	if runs() != 0 {
		t.Errorf("claude ran %d times for a named session", runs())
	}
}

func TestCardTitleGeneratedForDefaultName(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","name":"zest-f6","cwd":"/home/u/zest","status":"idle"`)
	e.transcript(idA, promptLine)
	store, _ := titleStore(t, "Titre généré")
	c := e.core(func(o *Options) { o.Titles = store })
	name, title := titleOf(t, c, 100)
	if name != "zest-f6" || title != "" {
		t.Errorf("first cycle: name %q, title %q", name, title)
	}
	store.Wait()
	name, title = titleOf(t, c, 100)
	if name != "zest-f6" || title != "Titre généré" {
		t.Errorf("second cycle: name %q, title %q", name, title)
	}
}

func TestCardTitleEmptyWithoutStore(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","name":"zest-f6","cwd":"/home/u/zest","status":"idle"`)
	e.transcript(idA, promptLine)
	if name, title := titleOf(t, e.core(), 100); name != "zest-f6" || title != "" {
		t.Errorf("name %q, title %q", name, title)
	}
}

func TestCardTitleEmptyForSessionWithoutID(t *testing.T) {
	e := newEnv(t)
	e.session(200, `"cwd":"/home/u/zest","name":"zest-aa","status":"idle"`)
	store, runs := titleStore(t, "Titre")
	c := e.core(func(o *Options) { o.Titles = store })
	if _, title := titleOf(t, c, 200); title != "" {
		t.Errorf("title %q", title)
	}
	store.Wait()
	if runs() != 0 {
		t.Errorf("claude ran %d times", runs())
	}
}

func TestCardTitleWithoutTranscript(t *testing.T) {
	e := newEnv(t)
	e.session(100, `"sessionId":"`+idA+`","name":"zest-f6","cwd":"/home/u/zest","status":"idle"`)
	store, runs := titleStore(t, "Titre")
	c := e.core(func(o *Options) { o.Titles = store })
	if _, title := titleOf(t, c, 100); title != "" {
		t.Errorf("title %q", title)
	}
	store.Wait()
	if runs() != 0 {
		t.Errorf("claude ran %d times", runs())
	}
}

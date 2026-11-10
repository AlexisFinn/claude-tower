package transcript

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type followed struct {
	f      Follower
	path   string
	lines  []string
	resets int
}

func newFollowed(t *testing.T, lines ...string) *followed {
	t.Helper()
	w := &followed{path: filepath.Join(t.TempDir(), "s.jsonl")}
	w.write(t, lines...)
	return w
}

func (w *followed) write(t *testing.T, lines ...string) {
	t.Helper()
	var data string
	if len(lines) > 0 {
		data = strings.Join(lines, "\n") + "\n"
	}
	if err := os.WriteFile(w.path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (w *followed) append(t *testing.T, data string) {
	t.Helper()
	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(data); err != nil {
		t.Fatal(err)
	}
}

// read returns the lines that the Follower passed since the previous read.
func (w *followed) read(t *testing.T, path string) []string {
	t.Helper()
	w.lines = nil
	err := w.f.Read(path, func() { w.resets++ }, func(l []byte) { w.lines = append(w.lines, string(l)) })
	if err != nil {
		t.Fatal(err)
	}
	return w.lines
}

func TestFollowerReadsOnlyWhatWasAdded(t *testing.T) {
	w := newFollowed(t, "a", "b")
	if got := w.read(t, w.path); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("first read: %v", got)
	}
	if got := w.read(t, w.path); len(got) != 0 {
		t.Fatalf("unchanged file: %v", got)
	}
	w.append(t, "c\n")
	if got := w.read(t, w.path); !reflect.DeepEqual(got, []string{"c"}) {
		t.Errorf("after append: %v", got)
	}
}

func TestFollowerPartialTrailingLineWaitsForItsEnd(t *testing.T) {
	w := newFollowed(t)
	w.append(t, "hel")
	if got := w.read(t, w.path); len(got) != 0 {
		t.Fatalf("partial line passed: %v", got)
	}
	w.append(t, "lo")
	if got := w.read(t, w.path); len(got) != 0 {
		t.Fatalf("line without its newline passed: %v", got)
	}
	w.append(t, "\n")
	if got := w.read(t, w.path); !reflect.DeepEqual(got, []string{"hello"}) {
		t.Errorf("got %v", got)
	}
}

func TestFollowerPartialLineFollowedByCompleteLineInTheSameRead(t *testing.T) {
	w := newFollowed(t)
	w.append(t, "hel")
	w.read(t, w.path)
	w.append(t, "lo\nworld\n")
	if got := w.read(t, w.path); !reflect.DeepEqual(got, []string{"hello", "world"}) {
		t.Errorf("got %v", got)
	}
}

func TestFollowerLineLargerThanTheReadChunk(t *testing.T) {
	big := strings.Repeat("x", 3*readChunk)
	w := newFollowed(t, big, "next")
	if got := w.read(t, w.path); len(got) != 2 || got[0] != big || got[1] != "next" {
		t.Errorf("got %d lines", len(got))
	}
}

func TestFollowerReadBufferIsAllocatedOnDemandAndKept(t *testing.T) {
	w := newFollowed(t)
	w.read(t, w.path)
	if w.f.buf != nil {
		t.Fatal("buffer allocated for an empty transcript")
	}
	w.append(t, "a\n")
	w.read(t, w.path)
	if w.f.buf == nil {
		t.Fatal("buffer not allocated")
	}
	first := &w.f.buf[0]
	w.append(t, "b\n")
	w.read(t, w.path)
	if &w.f.buf[0] != first {
		t.Error("buffer reallocated")
	}
}

func TestFollowerStartsOverWhenTheFileShrinks(t *testing.T) {
	w := newFollowed(t, "aaa", "bbb", "ccc")
	w.read(t, w.path)
	resets := w.resets
	w.write(t, "d")
	if got := w.read(t, w.path); !reflect.DeepEqual(got, []string{"d"}) {
		t.Errorf("got %v", got)
	}
	if w.resets != resets+1 {
		t.Errorf("resets=%d want %d", w.resets, resets+1)
	}
}

func TestFollowerStartsOverWhenTheFileIsReplaced(t *testing.T) {
	w := newFollowed(t, "a")
	w.read(t, w.path)
	other := filepath.Join(filepath.Dir(w.path), "other")
	if err := os.WriteFile(other, []byte("bbb\nccc\nddd\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(other, w.path); err != nil {
		t.Fatal(err)
	}
	if got := w.read(t, w.path); !reflect.DeepEqual(got, []string{"bbb", "ccc", "ddd"}) {
		t.Errorf("got %v", got)
	}
}

func TestFollowerStartsOverForAnotherPath(t *testing.T) {
	w := newFollowed(t, "a")
	w.read(t, w.path)
	other := filepath.Join(filepath.Dir(w.path), "other.jsonl")
	if err := os.WriteFile(other, []byte("b\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := w.read(t, other); !reflect.DeepEqual(got, []string{"b", "c"}) {
		t.Errorf("got %v", got)
	}
}

func TestFollowerFirstReadResets(t *testing.T) {
	w := newFollowed(t, "a")
	w.read(t, w.path)
	if w.resets != 1 {
		t.Errorf("resets=%d want 1", w.resets)
	}
}

func TestFollowerMissingFileKeepsPosition(t *testing.T) {
	w := newFollowed(t, "a")
	w.read(t, w.path)
	if err := os.Remove(w.path); err != nil {
		t.Fatal(err)
	}
	if err := w.f.Read(w.path, func() { t.Error("reset") }, func([]byte) {}); err == nil {
		t.Error("expected an error")
	}
	if w.f.off == 0 {
		t.Error("position lost")
	}
}

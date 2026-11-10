package locate

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"

	"claude-tower/internal/registry"
	"claude-tower/internal/tmux"
)

func TestParsePPID(t *testing.T) {
	cases := []struct {
		name, line string
		want       int
		ok         bool
	}{
		{"plain", "123 (bash) S 45 123 123 0", 45, true},
		{"command with spaces and parentheses", "123 (a) b (c)) S 77 1 1", 77, true},
		{"no parenthesis", "123 bash S 45", 0, false},
		{"short", "123 (x) S", 0, false},
		{"non numeric", "123 (x) S abc 1", 0, false},
	}
	for _, c := range cases {
		got, err := parsePPID(c.line)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("%s: got %d %v", c.name, got, err)
		}
	}
}

func TestParsePS(t *testing.T) {
	out := "    1     0\n  200     1\n  300   200\nbad line here\n  x  y\n\n  400\n"
	want := Tree{1: 0, 200: 1, 300: 200}
	if got := parsePS(out); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestReadProcFixture(t *testing.T) {
	dir := t.TempDir()
	for pid, line := range map[string]string{
		"10":   "10 (init) S 0 1 1",
		"20":   "20 (my (shell)) S 10 20 20",
		"junk": "1 (x) S 2",
		"30":   "garbage",
	} {
		if err := os.MkdirAll(filepath.Join(dir, pid), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, pid, "stat"), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := readProc(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Tree{10: 0, 20: 10}); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestReadProcSkipsEntriesWithoutFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "40"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "41"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "41", "stat"), []byte("41 (sh) S 40 41 41"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "42"), []byte("42 (sh) S 1 1 1"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readProc(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := (Tree{41: 40}); !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

func TestReadProcMissingDirFails(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent")
	if _, err := readProc(missing); err == nil {
		t.Error("missing directory ignored by readProc")
	}
	if _, err := readProcAttachments(missing); err == nil {
		t.Error("missing directory ignored by readProcAttachments")
	}
}

func TestReadTreeOnThisSystem(t *testing.T) {
	run := func(context.Context, io.Reader, string, ...string) ([]byte, error) {
		return nil, errors.New("ps unavailable")
	}
	tree, err := ReadTree(context.Background(), run)
	if runtime.GOOS != "linux" {
		if err == nil {
			t.Error("ps error ignored")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if tree[os.Getpid()] != os.Getppid() {
		t.Errorf("own ppid: got %d want %d", tree[os.Getpid()], os.Getppid())
	}
}

func TestReadTreeLinuxReadsProcAndNotPS(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "20"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "20", "stat"), []byte("20 (sh) S 10 20 20"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(context.Context, io.Reader, string, ...string) ([]byte, error) {
		t.Error("ps run on linux")
		return nil, nil
	}
	got, err := readTree(context.Background(), run, "linux", dir)
	if err != nil || !reflect.DeepEqual(got, Tree{20: 10}) {
		t.Errorf("got %v, %v", got, err)
	}
}

func TestReadTreeOtherSystemsRunPS(t *testing.T) {
	var gotArgs []string
	run := func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)
		return []byte("  20 10\n"), nil
	}
	got, err := readTree(context.Background(), run, "darwin", t.TempDir())
	if err != nil || !reflect.DeepEqual(got, Tree{20: 10}) {
		t.Errorf("got %v, %v", got, err)
	}
	if want := []string{"ps", "-A", "-o", "pid=,ppid="}; !reflect.DeepEqual(gotArgs, want) {
		t.Errorf("command: %v", gotArgs)
	}
	run = func(context.Context, io.Reader, string, ...string) ([]byte, error) {
		return nil, errors.New("ps unavailable")
	}
	if _, err := readTree(context.Background(), run, "darwin", t.TempDir()); err == nil {
		t.Error("ps error ignored")
	}
}

func TestDescendants(t *testing.T) {
	tree := Tree{2: 1, 10: 2, 11: 2, 12: 10, 13: 12, 20: 3, 30: 31, 31: 30}
	cases := []struct {
		name string
		pid  int
		want []int
	}{
		{"subtree without the process itself", 2, []int{10, 11, 12, 13}},
		{"leaf", 13, nil},
		{"other branch", 3, []int{20}},
		{"unknown pid", 999, nil},
		{"cycle", 30, []int{31}},
	}
	for _, c := range cases {
		got := tree.Descendants(c.pid)
		sort.Ints(got)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestAncestors(t *testing.T) {
	tree := Tree{500: 400, 400: 300, 300: 1, 1: 0, 10: 20, 20: 10}
	cases := []struct {
		name string
		pid  int
		want []int
	}{
		{"chain stops before init", 500, []int{500, 400, 300}},
		{"unknown pid", 999, []int{999}},
		{"init", 1, nil},
		{"cycle", 10, []int{10, 20}},
	}
	for _, c := range cases {
		if got := tree.Ancestors(c.pid); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	deep := Tree{}
	for i := 2; i < 200; i++ {
		deep[i] = i - 1
	}
	// The walk keeps at most 32 entries: the start PID and its 31 nearest ancestors.
	got := deep.Ancestors(199)
	if len(got) != 32 || got[0] != 199 || got[31] != 168 {
		t.Errorf("depth limit: len=%d last=%d", len(got), got[len(got)-1])
	}
}

func TestMatch(t *testing.T) {
	tree := Tree{
		1000: 900, 900: 800, 800: 700,
		2000: 1999, 1999: 600,
		3000: 2900,
	}
	panes := []tmux.Pane{
		{ID: "%1", PID: 700},
		{ID: "%2", PID: 600},
		{ID: "%3", PID: 650},
	}
	sessions := []registry.Session{{PID: 1000}, {PID: 2000}, {PID: 3000}, {PID: 600}}
	got := Match(sessions, panes, tree)
	if len(got) != 3 || got[1000].ID != "%1" || got[2000].ID != "%2" || got[600].ID != "%2" {
		t.Errorf("got %v", got)
	}
	if _, ok := got[3000]; ok {
		t.Error("session outside tmux matched")
	}
}

func TestMatchPrefersNearestAncestor(t *testing.T) {
	tree := Tree{300: 200, 200: 100}
	panes := []tmux.Pane{{ID: "%outer", PID: 100}, {ID: "%inner", PID: 200}}
	if got := Match([]registry.Session{{PID: 300}}, panes, tree); got[300].ID != "%inner" {
		t.Errorf("got %v", got)
	}
}

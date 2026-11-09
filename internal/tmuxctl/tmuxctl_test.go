package tmuxctl

import (
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

type call struct{ name, args string }

// stub replaces runCmd and parentPID for one test. parents maps a pid to its
// parent; panePID is what display-message answers.
func stub(t *testing.T, panePID string, displayErr error, parents map[int]int) *[]call {
	t.Helper()
	oldRun, oldParent := runCmd, parentPID
	t.Cleanup(func() { runCmd, parentPID = oldRun, oldParent })
	var calls []call
	runCmd = func(name string, args ...string) ([]byte, error) {
		calls = append(calls, call{name, strings.Join(args, " ")})
		if args[0] == "display-message" {
			return []byte(panePID + "\n"), displayErr
		}
		return nil, nil
	}
	parentPID = func(pid int) (int, error) {
		p, ok := parents[pid]
		if !ok {
			return 0, errors.New("gone")
		}
		return p, nil
	}
	return &calls
}

func TestJumpSuccess(t *testing.T) {
	calls := stub(t, "100", nil, map[int]int{300: 200, 200: 100})
	if err := Jump("/dev/pts/3", "%7", 300); err != nil {
		t.Fatal(err)
	}
	want := []call{
		{"tmux", "display-message -p -t %7 #{pane_pid}"},
		{"tmux", "switch-client -c /dev/pts/3 -t %7"},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Errorf("got %v", *calls)
	}
}

func TestJumpSamePID(t *testing.T) {
	stub(t, "300", nil, nil)
	if err := Jump("c", "%1", 300); err != nil {
		t.Fatal(err)
	}
}

func TestJumpClientOmitted(t *testing.T) {
	for _, client := range []string{"", "#{client_name}"} {
		calls := stub(t, "300", nil, nil)
		if err := Jump(client, "%1", 300); err != nil {
			t.Fatal(err)
		}
		if got := (*calls)[1].args; got != "switch-client -t %1" {
			t.Errorf("client %q: got %q", client, got)
		}
	}
}

func TestJumpInvalidPane(t *testing.T) {
	calls := stub(t, "1", nil, nil)
	for _, pane := range []string{"", "7", "%", "%7x", "%7;x", "-t"} {
		err := Jump("c", pane, 1)
		if err == nil || err.Error() != "session hors tmux" {
			t.Errorf("pane %q: %v", pane, err)
		}
	}
	if len(*calls) != 0 {
		t.Errorf("tmux was called: %v", *calls)
	}
}

func TestJumpDisplayMessageFails(t *testing.T) {
	calls := stub(t, "", errors.New("no pane"), nil)
	err := Jump("c", "%1", 300)
	if err == nil || !strings.Contains(err.Error(), "introuvable") || len(*calls) != 1 {
		t.Errorf("err=%v calls=%v", err, *calls)
	}
}

func TestJumpUnrelatedPane(t *testing.T) {
	calls := stub(t, "999", nil, map[int]int{300: 200, 200: 1})
	err := Jump("c", "%1", 300)
	if err == nil || !strings.Contains(err.Error(), "introuvable") || len(*calls) != 1 {
		t.Errorf("err=%v calls=%v", err, *calls)
	}
}

func TestJumpBrokenParentChain(t *testing.T) {
	stub(t, "100", nil, map[int]int{300: 200})
	if err := Jump("c", "%1", 300); err == nil {
		t.Error("expected an error")
	}
}

func TestJumpBadPanePID(t *testing.T) {
	stub(t, "abc", nil, nil)
	if err := Jump("c", "%1", 300); err == nil {
		t.Error("expected an error")
	}
}

func TestJumpAncestorDepthLimit(t *testing.T) {
	parents := map[int]int{}
	for i := 1000; i > 900; i-- {
		parents[i] = i - 1
	}
	stub(t, "969", nil, parents)
	if err := Jump("c", "%1", 1000); err != nil {
		t.Errorf("ancestor 31 levels up: %v", err)
	}
	stub(t, "968", nil, parents)
	if err := Jump("c", "%1", 1000); err == nil {
		t.Error("ancestor 32 levels up: expected an error")
	}
}

func TestJumpSwitchFailsWithStderr(t *testing.T) {
	stub(t, "300", nil, nil)
	runCmd = func(name string, args ...string) ([]byte, error) {
		if args[0] == "display-message" {
			return []byte("300"), nil
		}
		return nil, &exec.ExitError{Stderr: []byte("can't find client: c\n")}
	}
	if err := Jump("c", "%1", 300); err == nil || err.Error() != "tmux switch-client: can't find client: c" {
		t.Errorf("err=%v", err)
	}
}

func TestJumpAncestorChainTooLong(t *testing.T) {
	parents := map[int]int{}
	for i := 1000; i > 900; i-- {
		parents[i] = i - 1
	}
	stub(t, "900", nil, parents)
	if err := Jump("c", "%1", 1000); err == nil {
		t.Error("expected an error beyond 32 levels")
	}
}

func TestJumpSwitchFails(t *testing.T) {
	stub(t, "300", nil, nil)
	runCmd = func(name string, args ...string) ([]byte, error) {
		if args[0] == "display-message" {
			return []byte("300"), nil
		}
		return nil, errors.New("boom")
	}
	if err := Jump("c", "%1", 300); err == nil || !strings.Contains(err.Error(), "switch-client") {
		t.Errorf("err=%v", err)
	}
}

func TestParsePPID(t *testing.T) {
	got, err := parsePPID("824693 (a) b (c d) S 4242 824693 824693 0 -1 4194560")
	if err != nil || got != 4242 {
		t.Errorf("got %d, %v", got, err)
	}
	for _, bad := range []string{"no paren", "1 (x) S", "1 (x) S abc"} {
		if _, err := parsePPID(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

func TestProcParentPID(t *testing.T) {
	if _, err := procParentPID(1 << 30); err == nil {
		t.Error("expected an error for a missing pid")
	}
	if p, err := procParentPID(1); err != nil || p != 0 {
		t.Errorf("pid 1: %d, %v", p, err)
	}
	if p, err := procParentPID(os.Getpid()); err != nil || p != os.Getppid() {
		t.Errorf("self: %d, %v, want %d", p, err, os.Getppid())
	}
}

package focus

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/jezek/xgb/xproto"
)

func TestSupportedOn(t *testing.T) {
	cases := []struct {
		name, goos, display, wayland string
		want                         bool
	}{
		{"X11", "linux", ":0", "", true},
		{"Wayland with Xwayland", "linux", ":0", "wayland-0", false},
		{"no display", "linux", "", "", false},
		{"macOS", "darwin", "", "", true},
		{"other system", "freebsd", ":0", "", false},
	}
	for _, c := range cases {
		if got := supportedOn(c.goos, c.display, c.wayland); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestParseCardinals(t *testing.T) {
	got := parseCardinals([]byte{1, 0, 0, 0, 0x34, 0x12, 0, 0, 9, 9})
	if want := []uint32{1, 0x1234}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
	if got := parseCardinals(nil); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestFirstWindowFollowsPIDOrderThenListOrder(t *testing.T) {
	windows := []window{{10, 500}, {11, 300}, {12, 300}}
	if w, ok := firstWindow([]int{100, 300, 500}, windows); !ok || w != 11 {
		t.Errorf("got %d ok=%v", w, ok)
	}
	if _, ok := firstWindow([]int{1, 2}, windows); ok {
		t.Error("window found for unknown pids")
	}
	if _, ok := firstWindow(nil, windows); ok {
		t.Error("window found without pids")
	}
}

func TestActiveWindowMessage(t *testing.T) {
	ev := activeWindowMessage(0x1234, 77)
	b := ev.Bytes()
	if len(b) != 32 || b[0] != 33 || b[1] != 32 {
		t.Fatalf("event header: %v", b)
	}
	if got := xproto.Window(uint32(b[4]) | uint32(b[5])<<8 | uint32(b[6])<<16 | uint32(b[7])<<24); got != 0x1234 {
		t.Errorf("window %#x", got)
	}
	if b[8] != 77 || b[12] != 2 {
		t.Errorf("atom or source indication wrong: %v", b)
	}
}

// fakePS answers `ps -o comm= -p <pid>` from a table.
func fakePS(comms map[int]string, calls *[]string) func(context.Context, io.Reader, string, ...string) ([]byte, error) {
	return func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
		*calls = append(*calls, name+" "+strings.Join(args, " "))
		if name != "ps" {
			return nil, nil
		}
		if comm, ok := comms[atoi(args[len(args)-1])]; ok {
			return []byte(comm + "\n"), nil
		}
		return nil, errors.New("no such process")
	}
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func TestFirstAppPID(t *testing.T) {
	var calls []string
	run := fakePS(map[int]string{
		30: "/bin/zsh",
		20: "/Applications/iTerm.app/Contents/MacOS/iTerm2",
		10: "/Applications/Other.app/Contents/MacOS/Other",
	}, &calls)
	pid, ok := firstAppPID(context.Background(), []int{40, 30, 20, 10}, run)
	if !ok || pid != 20 {
		t.Errorf("got %d ok=%v", pid, ok)
	}
	if _, ok := firstAppPID(context.Background(), []int{30, 40}, run); ok {
		t.Error("pid outside an application bundle accepted")
	}
}

func TestFirstAppPIDDetectsBundleDirectoryOnly(t *testing.T) {
	cases := []struct {
		comm string
		want bool
	}{
		{"/Applications/iTerm.app/Contents/MacOS/iTerm2", true},
		{"/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal", true},
		{"/Applications/iTerm.app", false},
		{"/usr/local/bin/myapp/run", false},
		{"/opt/foo.application/bin/x", false},
		{"/bin/zsh", false},
		{"", false},
	}
	for _, c := range cases {
		var calls []string
		run := fakePS(map[int]string{7: c.comm}, &calls)
		_, ok := firstAppPID(context.Background(), []int{7}, run)
		if ok != c.want {
			t.Errorf("%q: got %v want %v", c.comm, ok, c.want)
		}
		if !reflect.DeepEqual(calls, []string{"ps -o comm= -p 7"}) {
			t.Errorf("%q: calls %q", c.comm, calls)
		}
	}
}

func TestOsascriptArgs(t *testing.T) {
	want := []string{"-e", `tell application "System Events" to set frontmost of (first process whose unix id is 4242) to true`}
	if got := osascriptArgs(4242); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestRaiseMacOSRunsOsascriptForTheApplication(t *testing.T) {
	var calls []string
	run := fakePS(map[int]string{20: "/Applications/iTerm.app/Contents/MacOS/iTerm2"}, &calls)
	if err := raiseMacOS(context.Background(), []int{30, 20}, run); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"ps -o comm= -p 30",
		"ps -o comm= -p 20",
		`osascript -e tell application "System Events" to set frontmost of (first process whose unix id is 20) to true`,
	}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls: %q\nwant:  %q", calls, want)
	}
	calls = nil
	if err := raiseMacOS(context.Background(), []int{30}, run); err == nil {
		t.Error("no error without application")
	}
	for _, c := range calls {
		if strings.HasPrefix(c, "osascript") {
			t.Error("osascript run without application")
		}
	}
}

func TestRaiseMacOSReturnsOsascriptError(t *testing.T) {
	run := func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
		if name == "osascript" {
			return nil, errors.New("not allowed")
		}
		return []byte("/A.app/x"), nil
	}
	if err := raiseMacOS(context.Background(), []int{5}, run); err == nil || err.Error() != "not allowed" {
		t.Errorf("err=%v", err)
	}
}

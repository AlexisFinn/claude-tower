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

func TestAppProcesses(t *testing.T) {
	var calls []string
	run := fakePS(map[int]string{
		40: "/bin/zsh",
		30: "/Applications/iTerm.app/Contents/MacOS/iTermServer-3.5.0",
		20: "/usr/bin/login",
		10: "/Applications/iTerm.app/Contents/MacOS/iTerm2",
	}, &calls)
	got := appProcesses(context.Background(), []int{40, 30, 20, 10, 5}, run)
	want := []appProcess{{30, "/Applications/iTerm.app"}, {10, "/Applications/iTerm.app"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v want %+v", got, want)
	}
}

func TestAppProcessesDetectsBundleDirectoryOnly(t *testing.T) {
	cases := []struct {
		comm, bundle string
	}{
		{"/Applications/iTerm.app/Contents/MacOS/iTerm2", "/Applications/iTerm.app"},
		{"/Applications/Utilities/Terminal.app/Contents/MacOS/Terminal", "/Applications/Utilities/Terminal.app"},
		{"/Applications/Visual Studio Code.app/Contents/Frameworks/Code Helper.app/Contents/MacOS/Code Helper", "/Applications/Visual Studio Code.app"},
		{"/Applications/iTerm.app", ""},
		{"/usr/local/bin/myapp/run", ""},
		{"/opt/foo.application/bin/x", ""},
		{"/bin/zsh", ""},
		{"", ""},
	}
	for _, c := range cases {
		var calls []string
		apps := appProcesses(context.Background(), []int{7}, fakePS(map[int]string{7: c.comm}, &calls))
		got := ""
		if len(apps) == 1 {
			got = apps[0].bundle
		}
		if got != c.bundle {
			t.Errorf("%q: got %q want %q", c.comm, got, c.bundle)
		}
		if !reflect.DeepEqual(calls, []string{"ps -o comm= -p 7"}) {
			t.Errorf("%q: calls %q", c.comm, calls)
		}
	}
}

// fakeMac answers ps from comms, osascript with osa, and records the calls.
func fakeMac(comms map[int]string, osa func() ([]byte, error), calls *[]string) func(context.Context, io.Reader, string, ...string) ([]byte, error) {
	ps := fakePS(comms, calls)
	return func(ctx context.Context, r io.Reader, name string, args ...string) ([]byte, error) {
		switch name {
		case "ps":
			return ps(ctx, r, name, args...)
		case "osascript":
			*calls = append(*calls, "osascript "+strings.Join(args[2*len(raiseScript):], " "))
			return osa()
		}
		*calls = append(*calls, name+" "+strings.Join(args, " "))
		return nil, nil
	}
}

func TestOsascriptArgsPassTheScriptThenThePIDs(t *testing.T) {
	args := osascriptArgs([]appProcess{{30, "/A.app"}, {10, "/A.app"}})
	if len(args) != 2*len(raiseScript)+2 || args[0] != "-e" || args[1] != "on run argv" {
		t.Fatalf("args: %q", args)
	}
	if got := args[len(args)-2:]; !reflect.DeepEqual(got, []string{"30", "10"}) {
		t.Errorf("pids: %q", got)
	}
}

func TestRaiseMacOSAsksSystemEventsForEveryBundleProcess(t *testing.T) {
	var calls []string
	run := fakeMac(map[int]string{
		30: "/Applications/iTerm.app/Contents/MacOS/iTermServer-3.5.0",
		10: "/Applications/iTerm.app/Contents/MacOS/iTerm2",
	}, func() ([]byte, error) { return []byte("raised\n"), nil }, &calls)
	if err := raiseMacOS(context.Background(), []int{40, 30, 10}, run); err != nil {
		t.Fatal(err)
	}
	want := []string{"ps -o comm= -p 40", "ps -o comm= -p 30", "ps -o comm= -p 10", "osascript 30 10"}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls: %q\nwant:  %q", calls, want)
	}
}

func TestRaiseMacOSOpensTheBundleWhenNoProcessIsKnown(t *testing.T) {
	var calls []string
	run := fakeMac(map[int]string{30: "/Applications/iTerm.app/Contents/MacOS/iTermServer-3.5.0"},
		func() ([]byte, error) { return []byte("none\n"), nil }, &calls)
	if err := raiseMacOS(context.Background(), []int{30}, run); err != nil {
		t.Fatal(err)
	}
	if last := calls[len(calls)-1]; last != "open -a /Applications/iTerm.app" {
		t.Errorf("calls: %q", calls)
	}
}

func TestRaiseMacOSReturnsOpenError(t *testing.T) {
	run := func(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
		switch name {
		case "ps":
			return []byte("/A.app/x\n"), nil
		case "osascript":
			return []byte("none\n"), nil
		}
		return nil, errors.New("open failed")
	}
	if err := raiseMacOS(context.Background(), []int{5}, run); err == nil || err.Error() != "open failed" {
		t.Errorf("err=%v", err)
	}
}

func TestRaiseMacOSWithoutApplicationFails(t *testing.T) {
	var calls []string
	run := fakeMac(map[int]string{30: "/bin/zsh"}, func() ([]byte, error) { return nil, nil }, &calls)
	if err := raiseMacOS(context.Background(), []int{30}, run); err == nil {
		t.Error("no error without application")
	}
	for _, c := range calls {
		if !strings.HasPrefix(c, "ps ") {
			t.Errorf("unexpected call %q", c)
		}
	}
}

func TestRaiseMacOSReturnsOsascriptError(t *testing.T) {
	var calls []string
	run := fakeMac(map[int]string{5: "/A.app/x"}, func() ([]byte, error) { return nil, errors.New("not allowed") }, &calls)
	if err := raiseMacOS(context.Background(), []int{5}, run); err == nil || err.Error() != "not allowed" {
		t.Errorf("err=%v", err)
	}
	for _, c := range calls {
		if strings.HasPrefix(c, "open ") {
			t.Error("bundle opened after an osascript error")
		}
	}
}

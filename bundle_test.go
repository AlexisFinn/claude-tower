package main

import "testing"

func TestBundleToOpen(t *testing.T) {
	const exe = "/Applications/claude-tower.app/Contents/MacOS/claude-tower"
	cases := []struct {
		name, goos, exe string
		ppid            int
		want            string
	}{
		{"started from a shell", "darwin", exe, 4242, "/Applications/claude-tower.app"},
		{"started by launchd", "darwin", exe, 1, ""},
		{"outside a bundle", "darwin", "/Users/a/claude-tower/build/bin/claude-tower", 4242, ""},
		{"bundle path with spaces", "darwin", "/Users/a/My Apps/claude-tower.app/Contents/MacOS/claude-tower", 4242, "/Users/a/My Apps/claude-tower.app"},
		{"Linux", "linux", exe, 4242, ""},
		{"unknown executable", "darwin", "", 4242, ""},
	}
	for _, c := range cases {
		if got := bundleToOpen(c.goos, c.exe, c.ppid); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

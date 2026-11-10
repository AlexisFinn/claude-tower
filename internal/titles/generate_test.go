package titles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCleanTitle(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Export des objectifs", "Export des objectifs"},
		{"\n  \nExport des objectifs.\nautre ligne", "Export des objectifs"},
		{`"Export des objectifs"`, "Export des objectifs"},
		{"« Export des objectifs ».", "Export des objectifs"},
		{`"Export des objectifs."`, "Export des objectifs"},
		{"'Export' ...", "Export"},
		{"Export\x1b[0m des\tobjectifs", "Export [0m des objectifs"},
		{"", ""},
		{" \n\t\n", ""},
		{`""`, ""},
		{strings.Repeat("a", 59) + ". " + strings.Repeat("b", 10), strings.Repeat("a", 59)},
		{strings.Repeat("é", 80), strings.Repeat("é", 60)},
	}
	for _, tt := range tests {
		if got := cleanTitle(tt.in); got != tt.want {
			t.Errorf("cleanTitle(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildPrompt(t *testing.T) {
	got := buildPrompt([]string{"première", "dernière"})
	want := "Donne un titre de 2 à 5 mots en français qui dit sur quoi porte la session, le titre seul.\n\n" +
		"Demandes de l'utilisateur :\n- première\n- dernière\n"
	if got != want {
		t.Errorf("prompt:\n%s", got)
	}
}

func TestGenerateCallsClaude(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache", "titles")
	var gotCall Call
	var gotDeadline bool
	s := New(Options{
		Path: filepath.Join(t.TempDir(), "titles.json"),
		Dir:  dir,
		Claude: func(_ context.Context, env []string) (string, error) {
			if !reflect.DeepEqual(env, []string{"CLAUDE_CONFIG_DIR=/cfg"}) {
				t.Errorf("env given to Claude: %v", env)
			}
			return "/bin/claude", nil
		},
		Env: func(context.Context) []string { return []string{"CLAUDE_CONFIG_DIR=/cfg"} },
		Run: func(ctx context.Context, c Call) ([]byte, error) {
			gotCall = c
			_, gotDeadline = ctx.Deadline()
			return []byte("\"Titre généré.\"\n"), nil
		},
	})
	title, err := s.generate([]string{"a", "b"})
	if err != nil || title != "Titre généré" {
		t.Fatalf("title %q, error %v", title, err)
	}
	want := Call{
		Bin: "/bin/claude",
		Args: []string{"-p", "--model", "haiku", "--no-session-persistence", "--setting-sources", "local",
			"--strict-mcp-config", "--tools", ""},
		Dir:   dir,
		Env:   []string{"CLAUDE_CONFIG_DIR=/cfg"},
		Stdin: buildPrompt([]string{"a", "b"}),
	}
	if !reflect.DeepEqual(gotCall, want) {
		t.Errorf("call:\n got %+v\nwant %+v", gotCall, want)
	}
	if !gotDeadline {
		t.Error("no timeout")
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Errorf("working directory not created: %v", err)
	}
}

func TestGenerateFailures(t *testing.T) {
	newStore := func(claude func(context.Context, []string) (string, error), run Runner) *Store {
		return New(Options{Path: filepath.Join(t.TempDir(), "t.json"), Dir: filepath.Join(t.TempDir(), "d"), Claude: claude, Run: run})
	}
	found := func(context.Context, []string) (string, error) { return "/bin/claude", nil }
	tests := map[string]*Store{
		"claude not found": newStore(func(context.Context, []string) (string, error) { return "", errors.New("introuvable") }, nil),
		"run error":        newStore(found, func(context.Context, Call) ([]byte, error) { return []byte("x"), errors.New("boom") }),
		"empty output":     newStore(found, func(context.Context, Call) ([]byte, error) { return []byte(" \n\"\"\n"), nil }),
	}
	for name, s := range tests {
		if title, err := s.generate([]string{"a"}); err == nil || title != "" {
			t.Errorf("%s: title %q, error %v", name, title, err)
		}
	}
}

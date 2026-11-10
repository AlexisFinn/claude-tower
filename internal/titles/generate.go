package titles

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	generateTimeout = 60 * time.Second
	maxTitleRunes   = 60
	// killDelay bounds the wait for the output of a claude process that was
	// killed on timeout, when a child keeps its output open.
	killDelay = 5 * time.Second
)

// claudeArgs are the arguments of the claude process that writes a title: a
// single prompt on the standard input, no session kept, no tool, no setting
// source but the local one, and only the MCP servers given on the command
// line, which are none.
var claudeArgs = []string{
	"-p", "--model", "haiku", "--no-session-persistence",
	"--setting-sources", "local", "--strict-mcp-config", "--tools", "",
}

// Call describes a claude process to run.
type Call struct {
	Bin   string
	Args  []string
	Dir   string
	Env   []string
	Stdin string
}

// Runner runs a claude process and returns its standard output.
type Runner func(ctx context.Context, call Call) ([]byte, error)

func execRunner(ctx context.Context, call Call) ([]byte, error) {
	cmd := exec.CommandContext(ctx, call.Bin, call.Args...)
	cmd.Dir = call.Dir
	cmd.Env = call.Env
	cmd.Stdin = strings.NewReader(call.Stdin)
	cmd.WaitDelay = killDelay
	return cmd.Output()
}

// buildPrompt asks for a title from the requests.
func buildPrompt(requests []string) string {
	var b strings.Builder
	b.WriteString("Donne un titre de 2 à 5 mots en français qui dit sur quoi porte la session, le titre seul.\n\n")
	b.WriteString("Demandes de l'utilisateur :\n")
	for _, r := range requests {
		b.WriteString("- ")
		b.WriteString(r)
		b.WriteByte('\n')
	}
	return b.String()
}

// cleanTitle returns the first non-empty line of out without quotes around it
// nor final period, cut to maxTitleRunes characters. It returns "" when out
// holds no title.
func cleanTitle(out string) string {
	for _, line := range strings.Split(out, "\n") {
		title := trimDecoration(normalize(line))
		if title == "" {
			continue
		}
		if r := []rune(title); len(r) > maxTitleRunes {
			title = trimDecoration(string(r[:maxTitleRunes]))
		}
		return title
	}
	return ""
}

// trimDecoration removes the spaces, quotes and final periods around a title.
func trimDecoration(s string) string {
	const quotes = "\"'`«»“”‘’"
	for {
		t := strings.TrimSpace(s)
		t = strings.Trim(t, quotes)
		t = strings.TrimRight(t, ".…")
		if t == s {
			return t
		}
		s = t
	}
}

// generate asks claude for the title of a session from its requests.
func (s *Store) generate(requests []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), generateTimeout)
	defer cancel()
	env := s.opts.Env(ctx)
	bin, err := s.opts.Claude(ctx, env)
	if err != nil {
		return "", err
	}
	if s.opts.Dir == "" {
		return "", errors.New("dossier de cache inconnu")
	}
	if err := os.MkdirAll(s.opts.Dir, 0o700); err != nil {
		return "", err
	}
	out, err := s.opts.Run(ctx, Call{Bin: bin, Args: claudeArgs, Dir: s.opts.Dir, Env: env, Stdin: buildPrompt(requests)})
	if err != nil {
		return "", err
	}
	title := cleanTitle(string(out))
	if title == "" {
		return "", errors.New("titre vide")
	}
	return title, nil
}

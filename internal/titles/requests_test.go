package titles

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTranscript(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func userLine(text string) string {
	return `{"type":"user","message":{"role":"user","content":` + quote(text) + `}}`
}

func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s) + `"`
}

func TestScanRequestsKeepsTypedTextOnly(t *testing.T) {
	path := writeTranscript(t,
		`{"type":"summary","summary":"x"}`,
		userLine("Corrige   le bug\nde l'export"),
		`{"type":"assistant","message":{"content":[{"type":"text","text":"Voilà"}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t","content":"ok"}]}}`,
		`{"type":"user","isMeta":true,"message":{"content":"Caveat: ignoré"}}`,
		userLine("<command-name>/clear</command-name>"),
		userLine("<local-command-stdout>sortie</local-command-stdout>"),
		userLine("[Request interrupted by user for tool use]"),
		`{"type":"user","isCompactSummary":true,"isVisibleInTranscriptOnly":true,"message":{"content":"This session is being continued"}}`,
		`{"type":"user","isSidechain":true,"message":{"content":"Brief d'un sous-agent"}}`,
		`{"type":"user","message":{"content":[{"type":"text","text":"Ajoute un test"},{"type":"image","source":{}}]}}`,
		// Typed quotes are escaped in the transcript, so a request naming the field is not taken for a tool result.
		userLine(`pourquoi "tool_use_id" est vide ?`),
		`not json "user"`,
		``,
	)
	got, err := ScanRequests(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Requests{Count: 3, Kept: []string{"Corrige le bug de l'export", "Ajoute un test", `pourquoi "tool_use_id" est vide ?`}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestScanRequestsAddsLastPromptUnlessRepeated(t *testing.T) {
	long := strings.Repeat("é", 400)
	path := writeTranscript(t,
		userLine("Première demande"),
		`{"type":"last-prompt","lastPrompt":"Première demande","sessionId":"s"}`,
		userLine(long),
		`{"type":"last-prompt","lastPrompt":"`+strings.Repeat("é", 200)+`"}`,
		`{"type":"last-prompt","lastPrompt":"Demande absente du transcript"}`,
		userLine("oui merci"),
		userLine("autre chose"),
		// A short request that an older one starts with is a request of its own.
		`{"type":"last-prompt","lastPrompt":"oui"}`,
	)
	got, err := ScanRequests(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Requests{Count: 6, Kept: []string{"Première demande", strings.Repeat("é", 300), "Demande absente du transcript", "oui"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestScanRequestsKeepsFirstThreeAndLast(t *testing.T) {
	var lines []string
	for _, n := range []string{"un", "deux", "trois", "quatre", "cinq", "six"} {
		lines = append(lines, userLine(n))
	}
	got, err := ScanRequests(writeTranscript(t, lines...))
	if err != nil {
		t.Fatal(err)
	}
	want := Requests{Count: 6, Kept: []string{"un", "deux", "trois", "six"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestScanRequestsFourRequests(t *testing.T) {
	got, err := ScanRequests(writeTranscript(t, userLine("a"), userLine("b"), userLine("c"), userLine("d")))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "b", "c", "d"}; !reflect.DeepEqual(got.Kept, want) || got.Count != 4 {
		t.Errorf("got %+v", got)
	}
}

func TestScanRequestsEmptyAndMissing(t *testing.T) {
	got, err := ScanRequests(writeTranscript(t, `{"type":"assistant"}`))
	if err != nil || got.Count != 0 || len(got.Kept) != 0 {
		t.Errorf("got %+v, %v", got, err)
	}
	if _, err := ScanRequests(filepath.Join(t.TempDir(), "absent.jsonl")); err == nil {
		t.Error("missing file accepted")
	}
}

func TestScanRequestsLastLineWithoutNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(path, []byte(userLine("a")+"\n"+userLine("b")), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ScanRequests(path)
	if err != nil || got.Count != 2 {
		t.Errorf("got %+v, %v", got, err)
	}
}

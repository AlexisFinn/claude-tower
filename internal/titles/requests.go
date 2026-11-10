package titles

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"unicode"
)

const (
	// maxRequestRunes is the length at which a request is cut.
	maxRequestRunes = 300
	// firstKept is the number of earliest requests kept for the prompt.
	firstKept = 3
)

// Requests is what a transcript says about the requests of the user.
type Requests struct {
	// Count is the number of requests in the transcript.
	Count int
	// Kept holds the first requests and the last one, in order, each cut to
	// maxRequestRunes characters.
	Kept []string
}

type record struct {
	Type       string `json:"type"`
	IsMeta     bool   `json:"isMeta"`
	LastPrompt string `json:"lastPrompt"`
	Message    struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type block struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ScanRequests reads the transcript at path and returns its requests: the
// text the user typed, and the "last-prompt" records that no earlier request
// accounts for.
func ScanRequests(path string) (Requests, error) {
	f, err := os.Open(path)
	if err != nil {
		return Requests{}, err
	}
	defer f.Close()
	var (
		seen []string
		out  Requests
	)
	r := bufio.NewReaderSize(f, 64<<10)
	for {
		line, err := r.ReadBytes('\n')
		text, isLastPrompt := requestOf(line)
		if text != "" && !(isLastPrompt && repeats(seen, text)) {
			seen = append(seen, text)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Requests{}, err
		}
	}
	out.Count = len(seen)
	if out.Count > firstKept {
		out.Kept = append(append(out.Kept, seen[:firstKept]...), seen[out.Count-1])
	} else {
		out.Kept = seen
	}
	return out, nil
}

// requestOf decodes one transcript line and returns the request it holds,
// cut to maxRequestRunes characters, "" when it holds none. isLastPrompt
// reports a "last-prompt" record.
func requestOf(line []byte) (text string, isLastPrompt bool) {
	if !bytes.Contains(line, []byte(`"user"`)) && !bytes.Contains(line, []byte(`"last-prompt"`)) {
		return "", false
	}
	var rec record
	if json.Unmarshal(line, &rec) != nil {
		return "", false
	}
	switch {
	case rec.Type == "last-prompt":
		return cut(normalize(rec.LastPrompt)), true
	case rec.Type == "user" && !rec.IsMeta:
		text = normalize(typedText(rec.Message.Content))
		if strings.HasPrefix(text, "<") {
			return "", false
		}
		return cut(text), false
	}
	return "", false
}

// typedText returns the text the user typed in a message content, "" when the
// content carries a tool result or no text.
func typedText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []block
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		switch b.Type {
		case "tool_result":
			return ""
		case "text":
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// repeats reports whether one of the requests starts with text, which is how
// a "last-prompt" record repeats a request, possibly cut short.
func repeats(requests []string, text string) bool {
	for i := len(requests) - 1; i >= 0; i-- {
		if strings.HasPrefix(requests[i], text) {
			return true
		}
	}
	return false
}

func cut(text string) string {
	if r := []rune(text); len(r) > maxRequestRunes {
		return string(r[:maxRequestRunes])
	}
	return text
}

// normalize collapses runs of whitespace and control characters into a single
// space and trims the result.
func normalize(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}), " ")
}

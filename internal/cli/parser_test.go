package cli

import (
	"strings"
	"testing"
)

// Regression tests for the multi-tool-call parsing failure observed with
// Qwen 2.5 Coder: responses containing several fenced JSON blocks plus prose
// previously parsed as ZERO tool calls, so nothing executed while the model
// fabricated "tool_result:" narration.

func TestParseMultipleFencedToolCallsWithProse(t *testing.T) {
	text := "Let's start.\n" +
		"```json\n{\"tool\":\"create_file\",\"input\":{\"path\":\"a.css\",\"content\":\"body{}\"}}\n```\n" +
		"Now the toggle.\n" +
		"```json\n{\"tool\":\"create_file\",\"input\":{\"path\":\"b.js\",\"content\":\"x\"}}\n```\n" +
		"Finally include them."
	calls, answer, err := parseToolCalls(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("want 2 calls, got %d: %+v", len(calls), calls)
	}
	if calls[0].Name != "create_file" || calls[0].Input["path"] != "a.css" {
		t.Errorf("call 0 = %+v", calls[0])
	}
	if calls[1].Input["path"] != "b.js" {
		t.Errorf("call 1 = %+v", calls[1])
	}
	if strings.Contains(answer, "create_file") {
		t.Errorf("answer should not contain executed tool JSON: %q", answer)
	}
}

func TestParseSingleObjectEmbeddedInProse(t *testing.T) {
	text := `I will inspect first. {"tool":"list_files","input":{"path":"."}} then report.`
	calls, _, err := parseToolCalls(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(calls) != 1 || calls[0].Name != "list_files" {
		t.Fatalf("calls = %+v", calls)
	}
}

func TestParseEscapedContentSurvives(t *testing.T) {
	text := "```json\n{\"tool\":\"write_file\",\"input\":{\"path\":\"main.go\",\"content\":\"package main\\n\\nfunc main() {\\n}\\n\"}}\n```"
	calls, _, err := parseToolCalls(text)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %+v", calls)
	}
	want := "package main\n\nfunc main() {\n}\n"
	if calls[0].Input["content"] != want {
		t.Errorf("escaped content mangled:\n got %q\nwant %q", calls[0].Input["content"], want)
	}
}

func TestParsePlainAnswerYieldsNoCalls(t *testing.T) {
	calls, answer, _ := parseToolCalls("Dark mode is ready. Toggle added to the navbar.")
	if len(calls) != 0 {
		t.Errorf("plain text misparsed as calls: %+v", calls)
	}
	if answer == "" {
		t.Error("answer must be preserved")
	}
}

func TestParseToolCallsWrapper(t *testing.T) {
	text := `{"tool_calls":[{"tool":"read_file","input":{"path":"a.go"}},{"tool":"git_status","input":{}}]}`
	calls, _, _ := parseToolCalls(text)
	if len(calls) != 2 || calls[0].Name != "read_file" || calls[1].Name != "git_status" {
		t.Fatalf("calls = %+v", calls)
	}
}

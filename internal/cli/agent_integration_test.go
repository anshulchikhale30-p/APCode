package cli

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"apcode/internal/model"
	"apcode/internal/runtime"
	"apcode/internal/tools"
)

// scriptedRuntime plays a fixed sequence of model outputs, simulating a model
// that inspects then edits then answers. Test-only.
type scriptedRuntime struct {
	outputs []string
	calls   int
	loaded  bool
}

func (s *scriptedRuntime) Type() runtime.RuntimeType { return runtime.RuntimeTypeMock }
func (s *scriptedRuntime) Name() string              { return "scripted" }
func (s *scriptedRuntime) IsCompatible(*model.ModelMetadata) bool {
	return true
}
func (s *scriptedRuntime) Load(context.Context, *model.ModelMetadata) error {
	s.loaded = true
	return nil
}
func (s *scriptedRuntime) Unload(context.Context) error { s.loaded = false; return nil }
func (s *scriptedRuntime) Close() error                 { return nil }
func (s *scriptedRuntime) Cancel(context.Context) error { return nil }
func (s *scriptedRuntime) Status(context.Context) (runtime.RuntimeStatus, error) {
	return runtime.RuntimeStatus{Type: "mock", State: "ready", Available: true}, nil
}
func (s *scriptedRuntime) Generate(_ context.Context, _ runtime.GenerateRequest) (*runtime.GenerateResponse, error) {
	if s.calls >= len(s.outputs) {
		return &runtime.GenerateResponse{Text: "done"}, nil
	}
	out := s.outputs[s.calls]
	s.calls++
	return &runtime.GenerateResponse{Text: out, FinishReason: "stop"}, nil
}
func (s *scriptedRuntime) Stream(ctx context.Context, req runtime.GenerateRequest) (<-chan runtime.StreamChunk, error) {
	ch := make(chan runtime.StreamChunk, 1)
	resp, err := s.Generate(ctx, req)
	if err != nil {
		return nil, err
	}
	ch <- runtime.StreamChunk{Token: resp.Text, Done: true, FinishReason: "stop"}
	close(ch)
	return ch, nil
}

var _ runtime.InferenceRuntime = (*scriptedRuntime)(nil)

func newTestREPL(t *testing.T, ws string, rt runtime.InferenceRuntime, stdin string) *REPL {
	t.Helper()
	src, err := tools.DefaultRegistryWithWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	tools.RegisterSpecTools(src, ws)
	j := NewJournal()
	reg, err := wrapRegistryWithJournal(src, ws, j)
	if err != nil {
		t.Fatal(err)
	}
	meta := &model.ModelMetadata{ID: "test-model", Name: "Test Model", MinimumRAMBytes: 1}
	repl := &REPL{
		In:        strings.NewReader(stdin),
		Out:       io.Discard,
		ErrOut:    io.Discard,
		Workspace: ws,
		Model:     meta,
		Runtime:   rt,
		Registry:  reg,
		Journal:   j,
		History:   []Message{},
	}
	repl.reader = bufio.NewReader(strings.NewReader(stdin))
	return repl
}

func TestAgentFlowInspectEditApproveRollback(t *testing.T) {
	ws := t.TempDir()
	writeFile := func(rel, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(ws, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(ws, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFile("theme.txt", "mode: light\n")

	rt := &scriptedRuntime{outputs: []string{
		`{"tool":"read_file","input":{"path":"theme.txt"}}`,
		`Plan:
1. Inspect theme file
2. Switch mode to dark
3. Verify content`,
		`{"tool":"edit_file","input":{"path":"theme.txt","old_string":"mode: light","new_string":"mode: dark"}}`,
		"Dark mode implemented.",
	}}

	// Approvals: read is automatic; edit needs one "y"; validation skipped (no stack).
	stdin := strings.Repeat("y\n", 5)
	repl := newTestREPL(t, ws, rt, stdin)

	repl.Journal.BeginGroup()
	response, err := repl.runAgent(context.Background(), "add dark mode")
	repl.Journal.EndGroup()
	if err != nil {
		t.Fatalf("runAgent: %v", err)
	}
	if response != "Dark mode implemented." {
		t.Errorf("response = %q", response)
	}

	data, _ := os.ReadFile(filepath.Join(ws, "theme.txt"))
	if string(data) != "mode: dark\n" {
		t.Fatalf("edit not applied: %q", data)
	}

	// Rollback restores original.
	restored, err := repl.Journal.Undo()
	if err != nil || len(restored) == 0 {
		t.Fatalf("rollback failed: %v %v", restored, err)
	}
	data, _ = os.ReadFile(filepath.Join(ws, "theme.txt"))
	if string(data) != "mode: light\n" {
		t.Errorf("rollback did not restore original: %q", data)
	}
}

func TestAgentFlowWriteRequiresApprovalAndDenialIsSafe(t *testing.T) {
	ws := t.TempDir()
	rt := &scriptedRuntime{outputs: []string{
		`{"tool":"write_file","input":{"path":"new.txt","content":"hello"}}`,
		"The user declined; stopping.",
	}}
	// User says "n".
	repl := newTestREPL(t, ws, rt, "n\n")
	repl.Journal.BeginGroup()
	_, err := repl.runAgent(context.Background(), "create a file")
	repl.Journal.EndGroup()
	if err != nil {
		t.Fatalf("runAgent: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(ws, "new.txt")); !os.IsNotExist(statErr) {
		t.Fatal("file was written despite user denial")
	}
}

func TestSlashCommandHandlersSmoke(t *testing.T) {
	ws := t.TempDir()
	rt := &scriptedRuntime{}
	repl := newTestREPL(t, ws, rt, "")
	repl.ProjectCtx = nil

	repl.handlePermissions() // must not panic
	repl.handleModel()       // no model path
	repl.handlePlan()        // empty plan path
	repl.handleCompact()     // short history
	repl.handleRollback()    // empty journal
	repl.handleFiles()
	repl.handleSearch(context.Background(), "/search anything")

	if repl.Journal.UndoCount() != 0 {
		t.Error("journal should still be empty")
	}
}

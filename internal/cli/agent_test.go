package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"apcode/internal/tools"
)

func TestJournalUndoRestoresContent(t *testing.T) {
	ws := t.TempDir()
	j := NewJournal()

	// Existing file is modified by APCode.
	existing := filepath.Join(ws, "existing.txt")
	if err := os.WriteFile(existing, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	// New file created by APCode.
	created := filepath.Join(ws, "created.txt")

	j.BeginGroup()
	j.Record(existing)
	if err := os.WriteFile(existing, []byte("modified"), 0o644); err != nil {
		t.Fatal(err)
	}
	j.Record(created)
	if err := os.WriteFile(created, []byte("brand new"), 0o644); err != nil {
		t.Fatal(err)
	}
	j.EndGroup()

	if j.UndoCount() != 1 {
		t.Fatalf("UndoCount = %d, want 1", j.UndoCount())
	}

	restored, err := j.Undo()
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if len(restored) != 2 {
		t.Errorf("restored = %v, want 2 entries", restored)
	}
	data, _ := os.ReadFile(existing)
	if string(data) != "original" {
		t.Errorf("existing file not restored: %q", data)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Error("created file should have been removed on rollback")
	}
	if j.UndoCount() != 0 {
		t.Errorf("journal should be empty after undo, got %d", j.UndoCount())
	}
}

func TestWrapRegistryWithJournal(t *testing.T) {
	ws := t.TempDir()
	src, err := tools.DefaultRegistryWithWorkspace(ws)
	if err != nil {
		t.Fatal(err)
	}
	tools.RegisterSpecTools(src, ws)
	j := NewJournal()
	wrapped, err := wrapRegistryWithJournal(src, ws, j)
	if err != nil {
		t.Fatal(err)
	}

	j.BeginGroup()
	res, err := wrapped.Execute(context.Background(), "write_file", tools.Input{"path": "j.txt", "content": "hi"})
	if err != nil || res.Err != nil {
		t.Fatalf("wrapped write failed: %v %v", err, res.Err)
	}
	j.EndGroup()
	if j.UndoCount() != 1 {
		t.Fatalf("expected 1 group, got %d", j.UndoCount())
	}
	restored, _ := j.Undo()
	if len(restored) == 0 || !strings.Contains(restored[0], "created by APCode") {
		t.Errorf("unexpected restore report: %v", restored)
	}
	if _, err := os.Stat(filepath.Join(ws, "j.txt")); !os.IsNotExist(err) {
		t.Error("rollback did not remove created file")
	}
}

func TestRequiresUserApprovalPolicy(t *testing.T) {
	readCases := []struct {
		name string
		in   tools.Input
	}{
		{"read_file", tools.Input{"path": "a.go"}},
		{"search", tools.Input{"query": "x"}},
		{"git_status", tools.Input{}},
	}
	for _, tc := range readCases {
		if requiresUserApproval(tc.name, tc.in) {
			t.Errorf("%s should be automatic", tc.name)
		}
	}
	writeCases := []struct {
		name string
		in   tools.Input
	}{
		{"write_file", tools.Input{"path": "a.go"}},
		{"create_file", tools.Input{"path": "a.go"}},
		{"delete_file", tools.Input{"path": "a.go"}},
		{"apply_patch", tools.Input{"patch": "..."}},
		{"shell", tools.Input{"command": "npm install x"}},
		{"RunCommand", tools.Input{"command": "pip install x"}},
	}
	for _, tc := range writeCases {
		if !requiresUserApproval(tc.name, tc.in) {
			t.Errorf("%s should require approval", tc.name)
		}
	}
	// Safe terminal commands are automatic.
	if requiresUserApproval("shell", tools.Input{"command": "go test ./..."}) {
		t.Error("safe shell command should be automatic")
	}
}

func TestExtractPlan(t *testing.T) {
	text := "I'll inspect first.\nPlan:\n1. Inspect UI\n2. Find theme system\n3. Implement dark mode\n4. Run tests\nDone."
	got := extractPlan(text)
	if !strings.Contains(got, "1. Inspect UI") || !strings.Contains(got, "4. Run tests") {
		t.Errorf("plan extraction failed: %q", got)
	}
	if extractPlan("just a plain answer") != "" {
		t.Error("plain text should not produce a plan")
	}
	numberedOnly := "Steps:\n1) one\n2) two\n3) three"
	if got := extractPlan(numberedOnly); len(got) == 0 {
		t.Error("numbered-only plan should be captured")
	}
}

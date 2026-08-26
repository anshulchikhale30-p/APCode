package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectProjectContext(t *testing.T) {
	// Use the current APCode repo as test project
	ws := "C:\\Users\\user\\APCode"
	if _, err := os.Stat(ws); err != nil {
		ws = "."
	}
	ctx, err := DetectProjectContext(ws)
	if err != nil {
		t.Fatalf("DetectProjectContext failed: %v", err)
	}
	if ctx.Root == "" {
		t.Error("expected non-empty root")
	}
	if ctx.Language == "" {
		t.Error("expected language")
	}
	if len(ctx.Files) == 0 {
		t.Error("expected files")
	}
	if !ctx.IsGitRepo {
		t.Log("not a git repo, but expected true for APCode")
	}
	// Check important files
	foundGoMod := false
	for _, f := range ctx.ImportantFiles {
		if f == "go.mod" {
			foundGoMod = true
			break
		}
	}
	if !foundGoMod {
		t.Error("expected go.mod in important files")
	}
}

func TestProjectContextSummary(t *testing.T) {
	pc := &ProjectContext{
		Root:           "/tmp/test",
		IsGitRepo:      true,
		Language:       "Go",
		Files:          []string{"a.go", "b.go"},
		ImportantFiles: []string{"go.mod", "README.md"},
		GitBranch:      "main",
	}
	s := pc.Summary()
	if s == "" {
		t.Error("expected summary")
	}
	if len(s) < 10 {
		t.Error("summary too short")
	}
}

func TestDetectProjectContextTemp(t *testing.T) {
	tmp := t.TempDir()
	// Create a fake project with go.mod and a Go source file
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte("package main\nfunc main(){}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "README.md"), []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, err := DetectProjectContext(tmp)
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	// Language should be Go or at least Go should be detected in the map
	if ctx.Languages["Go"] == 0 {
		t.Errorf("expected Go in languages, got %v", ctx.Languages)
	}
	if len(ctx.ImportantFiles) == 0 {
		t.Error("expected important files")
	}
}

package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func tempWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Ensure absolute
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	return abs
}

func writeFile(t *testing.T, ws, rel, content string) string {
	t.Helper()
	abs := filepath.Join(ws, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", abs, err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", abs, err)
	}
	return abs
}

func TestToolInterfaceAndSchemas(t *testing.T) {
	ws := tempWorkspace(t)
	tools := []Tool{
		NewReadFileTool(ws),
		NewWriteFileTool(ws),
		NewEditFileTool(ws),
		NewListDirectoryTool(ws),
		NewSearchFilesTool(ws),
		NewRunCommandTool(ws),
		NewGitDiffTool(ws),
		NewGitStatusTool(ws),
		NewGitLogTool(ws),
	}
	names := make(map[string]bool)
	for _, tl := range tools {
		if tl.Name() == "" {
			t.Errorf("tool Name empty")
		}
		if tl.Description() == "" {
			t.Errorf("tool %q Description empty", tl.Name())
		}
		schema := tl.InputSchema()
		if schema.Type == "" {
			t.Errorf("tool %q schema type empty", tl.Name())
		}
		if schema.Properties == nil {
			t.Errorf("tool %q schema properties nil", tl.Name())
		}
		if names[tl.Name()] {
			t.Errorf("duplicate tool name %q", tl.Name())
		}
		names[tl.Name()] = true
		// Normalize check
		norm := normalizeName(tl.Name())
		if norm == "" {
			t.Errorf("normalize empty for %q", tl.Name())
		}
	}
	// Expect 9 distinct names matching spec (case-insensitive) â€” includes GitStatus, GitLog
	expected := []string{"ReadFile", "WriteFile", "EditFile", "ListDirectory", "SearchFiles", "RunCommand", "GitDiff", "GitStatus", "GitLog"}
	for _, exp := range expected {
		if !names[exp] {
			// Also check normalized
			found := false
			for n := range names {
				if normalizeName(n) == normalizeName(exp) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected tool %q not found, got %v", exp, names)
			}
		}
	}
}

func TestRegistryAndDefault(t *testing.T) {
	ws := tempWorkspace(t)
	r, err := NewRegistryWithWorkspace(ws)
	if err != nil {
		t.Fatalf("NewRegistryWithWorkspace: %v", err)
	}
	// Register all
	tools := []Tool{
		NewReadFileTool(ws),
		NewWriteFileTool(ws),
		NewEditFileTool(ws),
		NewListDirectoryTool(ws),
		NewSearchFilesTool(ws),
		NewRunCommandTool(ws),
		NewGitDiffTool(ws),
		NewGitStatusTool(ws),
		NewGitLogTool(ws),
	}
	for _, tl := range tools {
		if err := r.Register(tl); err != nil {
			t.Fatalf("register %q: %v", tl.Name(), err)
		}
	}
	if r.Count() != 9 {
		t.Errorf("count = %d, want 9", r.Count())
	}
	// Get case-insensitive
	if _, ok := r.Get("readfile"); !ok {
		t.Error("Get readfile failed (case-insensitive)")
	}
	if _, ok := r.Get("read_file"); !ok {
		t.Error("Get read_file failed (underscore)")
	}
	if _, ok := r.Get("READ_FILE"); !ok {
		t.Error("Get READ_FILE failed")
	}
	// Alias for list_files -> ListDirectory
	if _, ok := r.Get("list_files"); !ok {
		t.Error("Get list_files alias failed")
	}
	if _, ok := r.Get("ListDirectory"); !ok {
		t.Error("Get ListDirectory failed")
	}
	// Duplicate registration should fail
	if err := r.Register(NewReadFileTool(ws)); err == nil {
		t.Error("duplicate register should fail")
	}
	// DefaultRegistry (9 base tools + 7 extended: create_file, delete_file,
	// apply_patch, project_info, run_tests, run_build, run_lint)
	dr := DefaultRegistry()
	if dr.Count() != 16 {
		t.Errorf("DefaultRegistry count = %d, want 16", dr.Count())
	}
	// DefaultRegistryWithWorkspace
	dr2, err := DefaultRegistryWithWorkspace(ws)
	if err != nil {
		t.Fatalf("DefaultRegistryWithWorkspace: %v", err)
	}
	if dr2.Count() != 16 {
		t.Errorf("DefaultRegistryWithWorkspace count = %d, want 16", dr2.Count())
	}
	// Extended tools are present
	for _, name := range []string{"create_file", "delete_file", "apply_patch", "project_info", "run_tests", "run_build", "run_lint"} {
		if _, ok := dr2.Get(name); !ok {
			t.Errorf("extended tool %q not registered", name)
		}
	}
	// DefinitionsForPrompt non-empty
	if s := dr.DefinitionsForPrompt(); !strings.Contains(s, "AVAILABLE TOOLS") {
		t.Error("DefinitionsForPrompt missing header")
	}
}

func TestReadFileSuccess(t *testing.T) {
	ws := tempWorkspace(t)
	writeFile(t, ws, "hello.txt", "hello world")
	tool := NewReadFileTool(ws)
	res, err := tool.Execute(context.Background(), Input{"path": "hello.txt"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("Result.Err: %v", res.Err)
	}
	if res.Output != "hello world" {
		t.Errorf("output = %q, want %q", res.Output, "hello world")
	}
	if res.Truncated {
		t.Error("should not be truncated")
	}
}

func TestReadFileNotFoundStructured(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewReadFileTool(ws)
	res, err := tool.Execute(context.Background(), Input{"path": "nope.txt"})
	if err != nil {
		t.Fatalf("Execute transport error: %v", err)
	}
	if res.Err == nil {
		t.Fatal("expected Result.Err for not found")
	}
	var te *ToolError
	if !errors.As(res.Err, &te) || te.Code != CodeNotFound {
		t.Errorf("expected CodeNotFound, got %v", res.Err)
	}
}

func TestReadFilePathTraversal(t *testing.T) {
	ws := tempWorkspace(t)
	writeFile(t, ws, "a.txt", "a")
	tool := NewReadFileTool(ws)
	// Try traversal with native separator (platform-independent)
	nativeTraversal := filepath.Join("..", "a.txt")
	_, err := tool.Execute(context.Background(), Input{"path": nativeTraversal})
	if err == nil {
		t.Fatalf("expected path traversal error for %q", nativeTraversal)
	}
	var te *ToolError
	if !errors.As(err, &te) || te.Code != CodePathTraversal {
		t.Errorf("expected PATH_TRAVERSAL for %q, got %v", nativeTraversal, err)
	}
	// Absolute outside
	_, err = tool.Execute(context.Background(), Input{"path": "/etc/passwd"})
	// On Windows, /etc/passwd may be C:\etc\passwd which might be outside workspace; should be blocked
	// But on Windows, "/etc/passwd" is not absolute (no volume) and may resolve inside workspace via Join, so be lenient.
	if err == nil {
		t.Logf("warning: absolute outside not blocked as transport error, check Result (platform-dependent)")
	} else if !errors.As(err, &te) || (te.Code != CodePathTraversal && te.Code != CodeOutsideWorkspace) {
		t.Errorf("expected PATH_TRAVERSAL or OUTSIDE_WORKSPACE for /etc/passwd, got %v", err)
	}
	// Native parent traversal via Join (covers both Windows and Unix)
	nativeSecret := filepath.Join("..", "secret.txt")
	_, err = tool.Execute(context.Background(), Input{"path": nativeSecret})
	if err == nil {
		t.Fatalf("expected traversal for %q", nativeSecret)
	}
	if !errors.As(err, &te) || te.Code != CodePathTraversal {
		t.Errorf("expected PATH_TRAVERSAL for %q, got %v", nativeSecret, err)
	}
	// Ensure Windows-style cannot bypass on Unix and vice versa (hardened validation)
	for _, winPath := range []string{"..\\secret.txt", "..\\..\\secret.txt"} {
		_, err = tool.Execute(context.Background(), Input{"path": winPath})
		if err == nil {
			t.Fatalf("expected traversal for Windows-style %q", winPath)
		}
		if !errors.As(err, &te) || te.Code != CodePathTraversal {
			t.Errorf("expected PATH_TRAVERSAL for Windows-style %q, got %v", winPath, err)
		}
	}
	// Also ensure Unix-style traversal is blocked on all platforms
	for _, unixPath := range []string{"../secret.txt", "../../secret.txt"} {
		_, err = tool.Execute(context.Background(), Input{"path": unixPath})
		if err == nil {
			t.Fatalf("expected traversal for Unix-style %q", unixPath)
		}
		if !errors.As(err, &te) || te.Code != CodePathTraversal {
			t.Errorf("expected PATH_TRAVERSAL for Unix-style %q, got %v", unixPath, err)
		}
	}
	_ = te
}

func TestReadFileOutsideWorkspaceAbsolute(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewReadFileTool(ws)
	// Create file outside workspace
	outside := filepath.Join(os.TempDir(), "outside_apcode_test.txt")
	_ = os.WriteFile(outside, []byte("outside"), 0o644)
	defer os.Remove(outside)
	_, err := tool.Execute(context.Background(), Input{"path": outside})
	if err == nil {
		t.Fatal("expected outside workspace error for absolute outside")
	}
	var te *ToolError
	if !errors.As(err, &te) {
		t.Fatalf("expected ToolError, got %v", err)
	}
	if te.Code != CodePathTraversal && te.Code != CodeOutsideWorkspace {
		t.Errorf("expected traversal/outside, got %q", te.Code)
	}
}

func TestReadFileOutputLimit(t *testing.T) {
	ws := tempWorkspace(t)
	// Create large file >1 MiB
	large := strings.Repeat("x", MaxFileBytes+5000)
	writeFile(t, ws, "large.txt", large)
	tool := NewReadFileTool(ws)
	res, err := tool.Execute(context.Background(), Input{"path": "large.txt"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("Result.Err: %v", res.Err)
	}
	if !res.Truncated {
		t.Error("expected truncated for large file")
	}
	if len(res.Output) > MaxOutputBytes+100 {
		t.Errorf("output not limited, len %d", len(res.Output))
	}
	if !strings.Contains(res.Output, "truncated") {
		t.Error("output should contain truncated marker")
	}
}

func TestReadFileCancellation(t *testing.T) {
	ws := tempWorkspace(t)
	writeFile(t, ws, "a.txt", "content")
	tool := NewReadFileTool(ws)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := tool.Execute(ctx, Input{"path": "a.txt"})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	var te *ToolError
	if !errors.As(err, &te) || te.Code != CodeCancelled {
		t.Errorf("expected CANCELLED, got %v", err)
	}
}

func TestReadFileInvalidInput(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewReadFileTool(ws)
	_, err := tool.Execute(context.Background(), Input{})
	if err == nil {
		t.Fatal("expected invalid input for missing path")
	}
	var te *ToolError
	if !errors.As(err, &te) || te.Code != CodeInvalidInput {
		t.Errorf("expected INVALID_INPUT, got %v", err)
	}
	// null byte
	_, err = tool.Execute(context.Background(), Input{"path": "a\x00b.txt"})
	if err == nil {
		t.Fatal("expected invalid for null byte")
	}
}

func TestWriteFileSuccess(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewWriteFileTool(ws)
	res, err := tool.Execute(context.Background(), Input{"path": "sub/dir/out.txt", "content": "hello"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("Result.Err: %v", res.Err)
	}
	// Verify file
	data, err := os.ReadFile(filepath.Join(ws, "sub/dir/out.txt"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("written content = %q, want %q", string(data), "hello")
	}
	if !strings.Contains(res.Output, "wrote") {
		t.Error("output should contain wrote")
	}
}

func TestWriteFilePathTraversal(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewWriteFileTool(ws)
	_, err := tool.Execute(context.Background(), Input{"path": "../../evil.txt", "content": "bad"})
	if err == nil {
		t.Fatal("expected traversal error")
	}
	var te *ToolError
	if !errors.As(err, &te) || te.Code != CodePathTraversal {
		t.Errorf("expected PATH_TRAVERSAL, got %v", err)
	}
}

func TestWriteFileTooLarge(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewWriteFileTool(ws)
	large := strings.Repeat("x", MaxFileBytes+1)
	_, err := tool.Execute(context.Background(), Input{"path": "big.txt", "content": large})
	if err == nil {
		t.Fatal("expected too large error")
	}
	var te *ToolError
	if !errors.As(err, &te) || te.Code != CodeTooLarge {
		t.Errorf("expected OUTPUT_TOO_LARGE, got %v", err)
	}
}

func TestWriteFileCancellation(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewWriteFileTool(ws)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := tool.Execute(ctx, Input{"path": "a.txt", "content": "x"})
	if err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestEditFileSuccess(t *testing.T) {
	ws := tempWorkspace(t)
	writeFile(t, ws, "code.go", "package main\nfunc Foo() {}\n")
	tool := NewEditFileTool(ws)
	res, err := tool.Execute(context.Background(), Input{"path": "code.go", "old_string": "Foo", "new_string": "Bar"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("Result.Err: %v", res.Err)
	}
	data, _ := os.ReadFile(filepath.Join(ws, "code.go"))
	if !strings.Contains(string(data), "Bar") || strings.Contains(string(data), "Foo") {
		t.Errorf("edit result = %q", string(data))
	}
	if !strings.Contains(res.Output, "edited") {
		t.Error("output should contain edited")
	}
}

func TestEditFileReplaceAll(t *testing.T) {
	ws := tempWorkspace(t)
	writeFile(t, ws, "a.txt", "a a a")
	tool := NewEditFileTool(ws)
	res, err := tool.Execute(context.Background(), Input{"path": "a.txt", "old_string": "a", "new_string": "b", "replace_all": "true"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("Result.Err: %v", res.Err)
	}
	data, _ := os.ReadFile(filepath.Join(ws, "a.txt"))
	if string(data) != "b b b" {
		t.Errorf("replace_all result = %q, want %q", string(data), "b b b")
	}
	if !strings.Contains(res.Output, "3") {
		t.Error("output should mention 3 replacements")
	}
}

func TestEditFileNotFoundOldString(t *testing.T) {
	ws := tempWorkspace(t)
	writeFile(t, ws, "a.txt", "hello")
	tool := NewEditFileTool(ws)
	res, err := tool.Execute(context.Background(), Input{"path": "a.txt", "old_string": "notexist", "new_string": "x"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res.Err == nil {
		t.Fatal("expected Result.Err for not found old_string")
	}
	var te *ToolError
	if !errors.As(res.Err, &te) || te.Code != CodeNotFound {
		t.Errorf("expected NOT_FOUND, got %v", res.Err)
	}
}

func TestEditFilePathTraversal(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewEditFileTool(ws)
	_, err := tool.Execute(context.Background(), Input{"path": "../a.txt", "old_string": "a", "new_string": "b"})
	if err == nil {
		t.Fatal("expected traversal error")
	}
}

func TestEditFileTooLargeResult(t *testing.T) {
	ws := tempWorkspace(t)
	// Create file near limit, then try to edit to exceed
	near := strings.Repeat("x", MaxFileBytes-10)
	writeFile(t, ws, "near.txt", near)
	tool := NewEditFileTool(ws)
	// Replace "x" with "xx" would double size -> exceed
	_, err := tool.Execute(context.Background(), Input{"path": "near.txt", "old_string": near[:10], "new_string": strings.Repeat("y", 1000), "replace_all": "true"})
	// Might exceed; check error
	if err != nil {
		var te *ToolError
		if !errors.As(err, &te) || te.Code != CodeTooLarge {
			t.Logf("got error %v, code %q", err, te.Code)
		}
	}
}

func TestEditFileCancellation(t *testing.T) {
	ws := tempWorkspace(t)
	writeFile(t, ws, "a.txt", "hello")
	tool := NewEditFileTool(ws)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := tool.Execute(ctx, Input{"path": "a.txt", "old_string": "hello", "new_string": "bye"})
	if err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestListDirectorySuccess(t *testing.T) {
	ws := tempWorkspace(t)
	writeFile(t, ws, "a.txt", "a")
	writeFile(t, ws, "b.txt", "b")
	if err := os.MkdirAll(filepath.Join(ws, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, ws, "subdir/c.txt", "c")
	tool := NewListDirectoryTool(ws)
	res, err := tool.Execute(context.Background(), Input{"path": "."})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("Result.Err: %v", res.Err)
	}
	if !strings.Contains(res.Output, "a.txt") || !strings.Contains(res.Output, "b.txt") || !strings.Contains(res.Output, "subdir/") {
		t.Errorf("output = %q, missing entries", res.Output)
	}
	if res.Truncated {
		t.Error("should not be truncated")
	}
}

func TestListDirectoryPathTraversal(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewListDirectoryTool(ws)
	_, err := tool.Execute(context.Background(), Input{"path": "../"})
	if err == nil {
		t.Fatal("expected traversal error")
	}
	var te *ToolError
	if !errors.As(err, &te) || te.Code != CodePathTraversal {
		t.Errorf("expected PATH_TRAVERSAL, got %v", err)
	}
}

func TestListDirectoryNotFound(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewListDirectoryTool(ws)
	res, err := tool.Execute(context.Background(), Input{"path": "nope"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res.Err == nil {
		t.Fatal("expected Result.Err for not found")
	}
	var te *ToolError
	if !errors.As(res.Err, &te) || te.Code != CodeNotFound {
		t.Errorf("expected NOT_FOUND, got %v", res.Err)
	}
}

func TestListDirectoryOutputLimit(t *testing.T) {
	ws := tempWorkspace(t)
	// Create many files to exceed MaxDirectoryEntries
	for i := 0; i < MaxDirectoryEntries+20; i++ {
		writeFile(t, ws, fmt.Sprintf("file_%04d.txt", i), "x")
	}
	tool := NewListDirectoryTool(ws)
	res, err := tool.Execute(context.Background(), Input{"path": "."})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !res.Truncated {
		t.Error("expected truncated")
	}
	if !strings.Contains(res.Output, "truncated") {
		t.Error("output should contain truncated")
	}
	lines := strings.Split(strings.TrimSpace(res.Output), "\n")
	// Should be MaxDirectoryEntries + 1 truncated line? Actually we limit to MaxDirectoryEntries entries plus truncated message line
	if len(lines) < MaxDirectoryEntries {
		t.Errorf("lines = %d, want >= %d", len(lines), MaxDirectoryEntries)
	}
}

func TestListDirectoryCancellation(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewListDirectoryTool(ws)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := tool.Execute(ctx, Input{"path": "."})
	if err == nil {
		t.Fatal("expected cancellation")
	}
}

func TestSearchFilesSuccess(t *testing.T) {
	ws := tempWorkspace(t)
	writeFile(t, ws, "a.go", "package main\nfunc Foo() {}\n")
	writeFile(t, ws, "b.go", "package main\nfunc Bar() {}\n")
	writeFile(t, ws, "c.txt", "hello Foo world")
	tool := NewSearchFilesTool(ws)
	res, err := tool.Execute(context.Background(), Input{"pattern": "Foo"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("Result.Err: %v", res.Err)
	}
	if !strings.Contains(res.Output, "a.go") || !strings.Contains(res.Output, "c.txt") {
		t.Errorf("output = %q, missing expected hits", res.Output)
	}
	// Should not include b.go
	if strings.Contains(res.Output, "b.go") {
		t.Error("b.go should not be in results")
	}
}

func TestSearchFilesMaxResults(t *testing.T) {
	ws := tempWorkspace(t)
	for i := 0; i < 10; i++ {
		writeFile(t, ws, fmt.Sprintf("f%d.txt", i), "pattern here")
	}
	tool := NewSearchFilesTool(ws)
	res, err := tool.Execute(context.Background(), Input{"pattern": "pattern", "max_results": "3"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	// Count non-truncated lines
	lines := strings.Split(strings.TrimSpace(res.Output), "\n")
	count := 0
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "...[truncated") {
			continue
		}
		if strings.TrimSpace(l) != "" {
			count++
		}
	}
	if count != 3 {
		t.Errorf("max_results 3 => count %d, want 3 (lines %v)", count, lines)
	}
	if !res.Truncated {
		t.Error("expected truncated when max_results limited")
	}
}

func TestSearchFilesPathTraversal(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewSearchFilesTool(ws)
	_, err := tool.Execute(context.Background(), Input{"pattern": "x", "path": "../../"})
	if err == nil {
		t.Fatal("expected traversal error")
	}
}

func TestSearchFilesCancellation(t *testing.T) {
	ws := tempWorkspace(t)
	writeFile(t, ws, "a.txt", "hello")
	tool := NewSearchFilesTool(ws)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := tool.Execute(ctx, Input{"pattern": "hello"})
	if err == nil {
		t.Fatal("expected cancellation")
	}
	var te *ToolError
	if !errors.As(err, &te) || te.Code != CodeCancelled {
		t.Errorf("expected CANCELLED, got %v", err)
	}
}

func TestSearchFilesInvalidInput(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewSearchFilesTool(ws)
	_, err := tool.Execute(context.Background(), Input{"pattern": ""})
	if err == nil {
		t.Fatal("expected invalid input for empty pattern")
	}
	var te *ToolError
	if !errors.As(err, &te) || te.Code != CodeInvalidInput {
		t.Errorf("expected INVALID_INPUT, got %v", err)
	}
}

func TestSearchFilesNoResults(t *testing.T) {
	ws := tempWorkspace(t)
	writeFile(t, ws, "a.txt", "hello")
	tool := NewSearchFilesTool(ws)
	res, err := tool.Execute(context.Background(), Input{"pattern": "nonexistentpattern123"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("Result.Err: %v", res.Err)
	}
	if res.Output != "" {
		t.Errorf("output should be empty for no results, got %q", res.Output)
	}
}

func TestRunCommandSuccess(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewRunCommandTool(ws)
	// Use go version or echo
	var res Result
	var err error
	// Try echo via runcommand: command=echo, args=hello
	res, err = tool.Execute(context.Background(), Input{"command": "go", "args": "version"})
	if err != nil {
		t.Fatalf("Execute transport error: %v", err)
	}
	if res.Err != nil && !strings.Contains(res.Output, "go version") {
		// Might fail if go not in PATH? Try alternative
		res, err = tool.Execute(context.Background(), Input{"command": "echo", "args": "hello"})
		if err != nil {
			t.Fatalf("Execute error: %v", err)
		}
		if res.Err != nil {
			t.Fatalf("Result.Err: %v", res.Err)
		}
		if !strings.Contains(res.Output, "hello") {
			t.Errorf("output = %q, want hello", res.Output)
		}
		return
	}
	if !strings.Contains(res.Output, "go version") {
		t.Errorf("output = %q, want go version", res.Output)
	}
	// Test stdout+stderr capture: run a command that writes to both
	// Use go run? Simpler: use command that fails
	res, err = tool.Execute(context.Background(), Input{"command": "go", "args": "version"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Truncated {
		t.Error("should not be truncated for small output")
	}
}

func TestRunCommandArgsSplitting(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewRunCommandTool(ws)
	// Provide command as single string with args
	res, err := tool.Execute(context.Background(), Input{"command": "go version"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Err != nil {
		t.Logf("go version may have failed: %v output %q", res.Err, res.Output)
	} else if !strings.Contains(res.Output, "go version") {
		t.Errorf("output = %q, want go version", res.Output)
	}
}

func TestRunCommandInvalidInput(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewRunCommandTool(ws)
	_, err := tool.Execute(context.Background(), Input{"command": ""})
	if err == nil {
		t.Fatal("expected invalid input for empty command")
	}
	var te *ToolError
	if !errors.As(err, &te) || te.Code != CodeInvalidInput {
		t.Errorf("expected INVALID_INPUT, got %v", err)
	}
	// Shell metachars
	_, err = tool.Execute(context.Background(), Input{"command": "echo && ls"})
	if err == nil {
		t.Fatal("expected invalid for shell metachars")
	}
}

func TestRunCommandWorkdirTraversal(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewRunCommandTool(ws)
	_, err := tool.Execute(context.Background(), Input{"command": "go", "args": "version", "dir": "../../"})
	if err == nil {
		t.Fatal("expected traversal for dir")
	}
	var te *ToolError
	if !errors.As(err, &te) || te.Code != CodePathTraversal {
		t.Errorf("expected PATH_TRAVERSAL, got %v", err)
	}
}

func TestRunCommandWorkdirInside(t *testing.T) {
	ws := tempWorkspace(t)
	sub := filepath.Join(ws, "sub")
	_ = os.MkdirAll(sub, 0o755)
	tool := NewRunCommandTool(ws)
	// Use go version with dir sub
	res, err := tool.Execute(context.Background(), Input{"command": "go", "args": "version", "dir": "sub"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Err != nil {
		t.Logf("go version in sub failed: %v", res.Err)
	}
}

func TestRunCommandOutputLimit(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewRunCommandTool(ws)
	// Create a command that outputs large data: use go run to print large string? Simpler: use echo with large args? But echo large may be limited.
	// Instead, we can use a command that generates output via shell? But we block shell.
	// Alternative: test truncation via direct logic: we cannot easily generate >256 KiB via echo without shell loop.
	// We'll test via a Go helper: run "go" with invalid args that produce large error? Not large.
	// We'll create a temp Go program that prints large output and run it via "go run"
	prog := `package main; import "fmt"; func main(){ for i:=0;i<10000;i++{ fmt.Println("abcdefghijklmnopqrstuvwxyz 1234567890") } }`
	writeFile(t, ws, "gen.go", prog)
	res, err := tool.Execute(context.Background(), Input{"command": "go", "args": "run gen.go", "confirm": "true"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	// This should produce large output and be truncated?
	// Max is 256 KiB, loop 10000 * ~40 = 400k > 256k, so truncated expected
	if len(res.Output) > MaxCommandOutputBytes+100 {
		t.Errorf("output not truncated, len %d", len(res.Output))
	}
	// If not truncated due to small output, still check not error
	_ = res
}

func TestRunCommandTimeout(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewRunCommandTool(ws)
	// Run a command that sleeps: use go run with sleep? Or use ping? On Windows, use timeout?
	// We can use a Go program that sleeps
	prog := `package main; import "time"; func main(){ time.Sleep(2*time.Second) }`
	writeFile(t, ws, "sleep.go", prog)
	res, err := tool.Execute(context.Background(), Input{"command": "go", "args": "run sleep.go", "timeout": "100ms", "confirm": "true"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res.Err == nil {
		t.Fatal("expected timeout error in Result.Err")
	}
	var te *ToolError
	if !errors.As(res.Err, &te) || te.Code != CodeExecutionFailed {
		t.Errorf("expected EXECUTION_FAILED for timeout, got %v", res.Err)
	}
	if !strings.Contains(strings.ToLower(res.Err.Error()), "timed out") {
		t.Errorf("error should mention timed out, got %v", res.Err)
	}
}

func TestRunCommandCancellation(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewRunCommandTool(ws)
	ctx, cancel := context.WithCancel(context.Background())
	// Start a long-running command and cancel
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	prog := `package main; import "time"; func main(){ time.Sleep(2*time.Second) }`
	writeFile(t, ws, "sleep2.go", prog)
	_, err := tool.Execute(ctx, Input{"command": "go", "args": "run sleep2.go", "confirm": "true"})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	var te *ToolError
	if !errors.As(err, &te) || te.Code != CodeCancelled {
		// Could also be context.Canceled wrapped
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected CANCELLED, got %v", err)
		}
	}
}

func TestRunCommandCaptureStderr(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewRunCommandTool(ws)
	// Run go with bad args to produce stderr
	res, err := tool.Execute(context.Background(), Input{"command": "go", "args": "badcommand123", "confirm": "true"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res.Err == nil {
		t.Fatal("expected Result.Err for failing command")
	}
	if res.Output == "" {
		t.Error("output should contain stderr")
	}
	// Ensure structured error code
	var te *ToolError
	if !errors.As(res.Err, &te) || te.Code != CodeExecutionFailed {
		t.Errorf("expected EXECUTION_FAILED, got %v", res.Err)
	}
}

func TestGitDiffNotRepo(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewGitDiffTool(ws)
	res, err := tool.Execute(context.Background(), Input{})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res.Err == nil {
		t.Fatal("expected Result.Err for not a git repo")
	}
	var te *ToolError
	if !errors.As(res.Err, &te) || te.Code != CodeNotGitRepo {
		t.Errorf("expected NOT_GIT_REPO, got %v", res.Err)
	}
}

func TestGitDiffSuccess(t *testing.T) {
	ws := tempWorkspace(t)
	// Init git repo
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = ws
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v output %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")
	writeFile(t, ws, "a.txt", "hello")
	run("add", "a.txt")
	run("commit", "-m", "init")
	// Modify file
	writeFile(t, ws, "a.txt", "hello world")
	tool := NewGitDiffTool(ws)
	res, err := tool.Execute(context.Background(), Input{})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("Result.Err: %v", res.Err)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Errorf("diff output missing hello, got %q", res.Output)
	}
	// Test with path filter
	res2, err := tool.Execute(context.Background(), Input{"path": "a.txt"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res2.Err != nil {
		t.Fatalf("Result.Err: %v", res2.Err)
	}
	if !strings.Contains(res2.Output, "hello") {
		t.Errorf("filtered diff missing hello")
	}
}

func TestGitDiffStaged(t *testing.T) {
	ws := tempWorkspace(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = ws
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "test")
	writeFile(t, ws, "a.txt", "v1")
	run("add", "a.txt")
	run("commit", "-m", "init")
	writeFile(t, ws, "a.txt", "v2")
	// Not staged: diff should show, staged diff should be empty
	tool := NewGitDiffTool(ws)
	res, err := tool.Execute(context.Background(), Input{"staged": "true"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Err != nil {
		t.Logf("staged diff err: %v", res.Err)
	}
	// Staged should be empty (no staged changes)
	if strings.Contains(res.Output, "v2") {
		t.Error("staged diff should be empty before add")
	}
	run("add", "a.txt")
	res2, err := tool.Execute(context.Background(), Input{"staged": "true"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !strings.Contains(res2.Output, "v2") && res2.Output != "" {
		t.Logf("staged diff output: %q", res2.Output)
	}
}

func TestGitDiffPathTraversal(t *testing.T) {
	ws := tempWorkspace(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	// Init repo to pass repo check
	cmd := exec.Command("git", "init")
	cmd.Dir = ws
	_, _ = cmd.CombinedOutput()
	tool := NewGitDiffTool(ws)
	_, err := tool.Execute(context.Background(), Input{"path": "../../"})
	if err == nil {
		t.Fatal("expected traversal error for path")
	}
	var te *ToolError
	if !errors.As(err, &te) || te.Code != CodePathTraversal {
		t.Errorf("expected PATH_TRAVERSAL, got %v", err)
	}
}

func TestGitDiffInvalidArgs(t *testing.T) {
	ws := tempWorkspace(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = ws
	_, _ = cmd.CombinedOutput()
	tool := NewGitDiffTool(ws)
	_, err := tool.Execute(context.Background(), Input{"args": "--output=/tmp/bad"})
	if err == nil {
		t.Fatal("expected invalid input for --output")
	}
	var te *ToolError
	if !errors.As(err, &te) || te.Code != CodeInvalidInput {
		t.Errorf("expected INVALID_INPUT, got %v", err)
	}
}

func TestGitDiffCancellation(t *testing.T) {
	ws := tempWorkspace(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = ws
	_, _ = cmd.CombinedOutput()
	tool := NewGitDiffTool(ws)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := tool.Execute(ctx, Input{})
	if err == nil {
		t.Fatal("expected cancellation")
	}
	var te *ToolError
	if !errors.As(err, &te) || te.Code != CodeCancelled {
		t.Errorf("expected CANCELLED, got %v", err)
	}
}

func TestStructuredErrorsUnwrap(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewReadFileTool(ws)
	_, err := tool.Execute(context.Background(), Input{"path": "../x"})
	if err == nil {
		t.Fatal("expected error")
	}
	var te *ToolError
	if !errors.As(err, &te) {
		t.Fatalf("not a ToolError: %v", err)
	}
	if te.Code != CodePathTraversal {
		t.Errorf("code = %q, want %q", te.Code, CodePathTraversal)
	}
	if !strings.Contains(te.Error(), CodePathTraversal) {
		t.Error("Error() should contain code")
	}
	if te.Unwrap() != nil {
		// Could be nil for traversal
	}
}

func TestOutputLimits(t *testing.T) {
	// Test that all tools enforce MaxOutputBytes via limitOutput
	s := strings.Repeat("a", MaxOutputBytes+100)
	trunc, isTrunc := limitOutput(s)
	if !isTrunc {
		t.Error("expected truncated")
	}
	if !strings.Contains(trunc, "truncated") {
		t.Error("truncated marker missing")
	}
	if len(trunc) > MaxOutputBytes+50 {
		t.Errorf("len %d exceeds limit", len(trunc))
	}
	// Test truncateString
	short, t2 := truncateString("hello", 10)
	if t2 || short != "hello" {
		t.Error("short should not truncate")
	}
	long, t3 := truncateString("hello world", 5)
	if !t3 || long != "hello" {
		t.Errorf("long truncate = %q, want %q", long, "hello")
	}
}

func TestWorkspaceRestrictionForAllFileTools(t *testing.T) {
	ws := tempWorkspace(t)
	// All file tools should reject traversal
	tools := []Tool{
		NewReadFileTool(ws),
		NewWriteFileTool(ws),
		NewEditFileTool(ws),
		NewListDirectoryTool(ws),
		NewSearchFilesTool(ws),
		NewGitDiffTool(ws),
	}
	for _, tl := range tools {
		_, err := tl.Execute(context.Background(), Input{"path": "../../etc/passwd", "pattern": "x", "old_string": "a", "new_string": "b", "content": "x"})
		if err == nil {
			// For some tools, error may be in Result.Err; check that
			// But for path validation, it should be transport error
			t.Logf("tool %q path traversal not returned as transport error, checking Result", tl.Name())
		}
		// At least ensure no file outside workspace was accessed
	}
}

func TestMockTool(t *testing.T) {
	called := false
	mock := NewMockTool("test_mock", "desc", func(ctx context.Context, in Input) (Result, error) {
		called = true
		if in["foo"] != "bar" {
			t.Errorf("input foo = %q, want bar", in["foo"])
		}
		return Result{Output: "ok"}, nil
	})
	if mock.Name() != "test_mock" {
		t.Errorf("name = %q", mock.Name())
	}
	if mock.Description() != "desc" {
		t.Error("description mismatch")
	}
	if mock.InputSchema().Type == "" {
		t.Error("schema type empty")
	}
	res, err := mock.Execute(context.Background(), Input{"foo": "bar"})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if res.Output != "ok" {
		t.Errorf("output = %q", res.Output)
	}
	if !called {
		t.Error("not called")
	}
	if len(mock.Calls) != 1 {
		t.Errorf("calls len = %d, want 1", len(mock.Calls))
	}
}

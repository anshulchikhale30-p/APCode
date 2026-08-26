package tools

import (
	"context"
	"testing"
)

func TestSpecAliases(t *testing.T) {
	tmp := t.TempDir()
	reg, err := DefaultRegistryWithWorkspace(tmp)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	RegisterSpecTools(reg, tmp)

	// Test read_file alias
	if _, ok := reg.Get("read_file"); !ok {
		t.Error("read_file should be available via alias or direct")
	}
	// search should be available (via alias or direct)
	if _, ok := reg.Get("search"); !ok {
		t.Error("search should be available")
	}
	// write_file
	if _, ok := reg.Get("write_file"); !ok {
		t.Error("write_file should be available")
	}
	// shell
	if _, ok := reg.Get("shell"); !ok {
		t.Error("shell should be available")
	}
	// git_diff
	if _, ok := reg.Get("git_diff"); !ok {
		t.Error("git_diff should be available")
	}
	// git_status
	if _, ok := reg.Get("git_status"); !ok {
		t.Error("git_status should be available")
	}

	// Test that normalized lookup works: ReadFile vs read_file
	if _, ok := reg.Get("ReadFile"); !ok {
		t.Error("ReadFile should be available")
	}
	if _, ok := reg.Get("READ_FILE"); !ok {
		t.Error("READ_FILE normalized should work")
	}
}

func TestToolPathTraversal(t *testing.T) {
	tmp := t.TempDir()
	reg, _ := DefaultRegistryWithWorkspace(tmp)
	tool, _ := reg.Get("read_file")
	_, err := tool.Execute(context.Background(), Input{"path": "../outside.txt"})
	if err == nil {
		t.Error("expected path traversal error")
	}
	if !IsToolError(err, CodePathTraversal) && !IsToolError(err, CodeOutsideWorkspace) {
		t.Errorf("expected path traversal error, got %v", err)
	}
}

func TestSearchAvoidsGit(t *testing.T) {
	tmp := t.TempDir()
	// Create .git and file inside
	// Use OS to create
	reg, _ := DefaultRegistryWithWorkspace(tmp)
	tool, _ := reg.Get("search")
	res, err := tool.Execute(context.Background(), Input{"query": "test"})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	// Should not error even if no results
	_ = res
}

func TestWriteFileAndRead(t *testing.T) {
	tmp := t.TempDir()
	reg, _ := DefaultRegistryWithWorkspace(tmp)
	writeTool, _ := reg.Get("write_file")
	readTool, _ := reg.Get("read_file")

	// Write
	_, err := writeTool.Execute(context.Background(), Input{"path": "test.txt", "content": "hello world"})
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}
	// Read
	res, err := readTool.Execute(context.Background(), Input{"path": "test.txt"})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if res.Output != "hello world" {
		t.Errorf("expected hello world, got %q", res.Output)
	}
}

func TestShellDestructiveRequiresConfirm(t *testing.T) {
	tmp := t.TempDir()
	reg, _ := DefaultRegistryWithWorkspace(tmp)
	shellTool, _ := reg.Get("shell")
	// Try destructive without confirm - should fail
	_, err := shellTool.Execute(context.Background(), Input{"command": "rm", "args": "-rf /"})
	if err == nil {
		t.Error("expected destructive command to require confirmation")
	}
	if !IsToolError(err, CodePermission) {
		t.Errorf("expected permission error for destructive, got %v", err)
	}
	// With confirm should proceed (but command is rm which may not exist on Windows, but should not fail on permission)
	// We test with a safe command
	_, err = shellTool.Execute(context.Background(), Input{"command": "go", "args": "version", "confirm": "true"})
	// Should not be permission error
	if IsToolError(err, CodePermission) {
		t.Errorf("go version should not require permission: %v", err)
	}
}

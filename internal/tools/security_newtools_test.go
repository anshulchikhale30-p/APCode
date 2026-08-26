package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyCommandTiers(t *testing.T) {
	cases := []struct {
		cmd  string
		want CommandClass
	}{
		{"go test ./...", ClassSafe},
		{"go build ./...", ClassSafe},
		{"npm test", ClassSafe},
		{"pytest -q", ClassSafe},
		{"git status --short", ClassSafe},
		{"ls -la", ClassSafe},
		{"dir /b", ClassSafe},
		{"echo hi", ClassSafe},
		{"goto test", ClassApprovalRequired}, // token boundary respected
		{"npm install foo", ClassApprovalRequired},
		{"pip install requests", ClassApprovalRequired},
		{"git reset --hard HEAD~1", ClassApprovalRequired},
		{"rm file.txt", ClassApprovalRequired},
		{"del file.txt", ClassApprovalRequired},
		{"mkfs.ext4 /dev/sda1", ClassBlocked},
		{"format C:", ClassBlocked},
		{"shutdown /s", ClassBlocked},
		{"dd if=/dev/zero of=/dev/sda", ClassBlocked},
	}
	for _, tc := range cases {
		if got := ClassifyCommand(tc.cmd); got != tc.want {
			t.Errorf("ClassifyCommand(%q) = %s, want %s", tc.cmd, got, tc.want)
		}
	}
}

func TestRunCommandBlockedEvenWithConfirm(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewRunCommandTool(ws)
	for _, cmd := range []string{"mkfs.ext4", "format", "shutdown"} {
		_, err := tool.Execute(context.Background(), Input{"command": cmd, "confirm": "true"})
		if err == nil {
			t.Errorf("%q executed despite BLOCKED class", cmd)
			continue
		}
		if !IsToolError(err, CodePermission) && !strings.Contains(strings.ToUpper(err.Error()), "BLOCKED") {
			t.Errorf("%q wrong error: %v", cmd, err)
		}
	}
}

func TestRunCommandApprovalRequiredWithoutConfirm(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewRunCommandTool(ws)
	if _, err := tool.Execute(context.Background(), Input{"command": "pip", "args": "install requests"}); err == nil {
		t.Fatal("non-safe command ran without confirm")
	}
	res, err := tool.Execute(context.Background(), Input{"command": "go", "args": "version"})
	if err != nil || res.Err != nil {
		t.Fatalf("safe command should run without confirm: %v %v", err, res.Err)
	}
}

func TestCreateFileRefusesExistingWithoutOverwrite(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewCreateFileTool(ws)
	writeFile(t, ws, "exists.txt", "original")

	res, err := tool.Execute(context.Background(), Input{"path": "exists.txt", "content": "new"})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res.Err == nil {
		t.Fatal("expected error creating over existing file without overwrite")
	}
	data, _ := os.ReadFile(filepath.Join(ws, "exists.txt"))
	if string(data) != "original" {
		t.Errorf("existing file was overwritten: %q", data)
	}
	// overwrite=true succeeds
	res, err = tool.Execute(context.Background(), Input{"path": "exists.txt", "content": "new", "overwrite": "true"})
	if err != nil || res.Err != nil {
		t.Fatalf("overwrite=true failed: %v %v", err, res.Err)
	}
	// New nested file is created
	res, err = tool.Execute(context.Background(), Input{"path": filepath.Join("sub", "new.txt"), "content": "x"})
	if err != nil || res.Err != nil {
		t.Fatalf("create new failed: %v %v", err, res.Err)
	}
}

func TestDeleteFileRequiresConfirmAndRejectsDirs(t *testing.T) {
	ws := tempWorkspace(t)
	tool := NewDeleteFileTool(ws)
	writeFile(t, ws, "gone.txt", "bye")

	// No confirm -> refused
	res, err := tool.Execute(context.Background(), Input{"path": "gone.txt"})
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	if res.Err == nil {
		t.Fatal("delete without confirm must be refused")
	}
	if !IsToolError(res.Err, CodePermission) {
		t.Errorf("want PERMISSION_DENIED, got %v", res.Err)
	}
	if _, err := os.Stat(filepath.Join(ws, "gone.txt")); err != nil {
		t.Fatal("file deleted without confirm!")
	}
	// With confirm -> deleted
	if _, err := tool.Execute(context.Background(), Input{"path": "gone.txt", "confirm": "true"}); err != nil {
		t.Fatalf("confirmed delete failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ws, "gone.txt")); !os.IsNotExist(err) {
		t.Fatal("file still exists after confirmed delete")
	}
	// Directory -> rejected
	if res, err := tool.Execute(context.Background(), Input{"path": ".", "confirm": "true"}); err == nil && res.Err == nil {
		t.Fatal("directory deletion must be rejected")
	}
}

func TestApplyPatchToolEndToEnd(t *testing.T) {
	ws := tempWorkspace(t)
	writeFile(t, ws, "app.txt", "hello\nworld\n")
	tool := NewApplyPatchTool(ws)
	res, err := tool.Execute(context.Background(), Input{
		"patch": "--- a/app.txt\n+++ b/app.txt\n@@ -1,2 +1,2 @@\n hello\n-world\n+apcode\n",
	})
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("apply failed: %v", res.Err)
	}
	data, _ := os.ReadFile(filepath.Join(ws, "app.txt"))
	if string(data) != "hello\napcode\n" {
		t.Errorf("unexpected content: %q", data)
	}
	// Mismatched context is rejected and leaves file intact
	bad := "--- a/app.txt\n+++ b/app.txt\n@@ -1,2 +1,2 @@\n nomatch\n-x\ny\n"
	if res, err := tool.Execute(context.Background(), Input{"patch": bad}); err == nil && res.Err == nil {
		t.Fatal("mismatched patch should fail")
	}
	data, _ = os.ReadFile(filepath.Join(ws, "app.txt"))
	if string(data) != "hello\napcode\n" {
		t.Errorf("file corrupted by failed patch: %q", data)
	}
}

func TestProjectInfoToolDetectsGo(t *testing.T) {
	ws := tempWorkspace(t)
	writeFile(t, ws, "go.mod", "module example.com/x\n\ngo 1.21\n")
	writeFile(t, ws, "main.go", "package main\nfunc main(){}\n")
	tool := NewProjectInfoTool(ws)
	res, err := tool.Execute(context.Background(), Input{})
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	if res.Err != nil {
		t.Fatalf("project_info: %v", res.Err)
	}
	out := res.Output
	for _, want := range []string{"go test ./...", "go build ./...", "Files discovered"} {
		if !strings.Contains(out, want) {
			t.Errorf("project_info output missing %q:\n%s", want, out)
		}
	}
}

func TestValidationToolsDetectStackOrReportMissing(t *testing.T) {
	ws := tempWorkspace(t)
	// Empty project: no stack detected -> structured NOT_FOUND
	tool := &validationTool{name: "run_tests", kind: "test", ws: ws}
	res, err := tool.Execute(context.Background(), Input{})
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	if !IsToolError(res.Err, CodeNotFound) {
		t.Errorf("want NOT_FOUND for empty project, got %v", res.Err)
	}
	// Go project: real command runs (go toolchain assumed present in CI/dev)
	writeFile(t, ws, "go.mod", "module tmpmod\n\ngo 1.21\n")
	tool2 := &validationTool{name: "run_build", kind: "build", ws: ws}
	res2, err := tool2.Execute(context.Background(), Input{})
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	// go build on empty module may succeed with no output; only assert no transport error
	_ = res2
}

func TestValidatePathSymlinkEscape(t *testing.T) {
	ws := tempWorkspace(t)
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o644); err != nil {
		t.Skipf("cannot write outside dir: %v", err)
	}
	link := filepath.Join(ws, "evil")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable on this platform/user: %v", err)
	}
	inside := filepath.Join(link, "secret.txt")
	if _, err := validatePath(ws, inside); err == nil {
		t.Fatal("symlink escape was not detected")
	} else if !IsToolError(err, CodePathTraversal) && !IsToolError(err, CodeOutsideWorkspace) {
		t.Errorf("wrong error class: %v", err)
	}
}

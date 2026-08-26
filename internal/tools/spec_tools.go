package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	apccontext "apcode/internal/context"
)

// ---------- CreateFile ----------

// CreateFileTool creates a new file, refusing to overwrite existing files
// unless overwrite=true is provided.
type CreateFileTool struct {
	workspace string
}

func NewCreateFileTool(workspace ...string) *CreateFileTool {
	ws := "."
	if len(workspace) > 0 && strings.TrimSpace(workspace[0]) != "" {
		ws = workspace[0]
	}
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	return &CreateFileTool{workspace: ws}
}

func (t *CreateFileTool) Name() string { return "create_file" }
func (t *CreateFileTool) Description() string {
	return "Create a new file within the workspace. Fails if the file already exists unless overwrite=true. Input: {\"path\": \"<relative path>\", \"content\": \"<text>\", \"overwrite\": \"true|false\"}"
}
func (t *CreateFileTool) InputSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Property{
			"path":      {Type: "string", Description: "Relative path of the file to create"},
			"content":   {Type: "string", Description: "Initial content"},
			"overwrite": {Type: "string", Description: "Allow replacing an existing file (default false)"},
		},
		Required: []string{"path"},
	}
}

func (t *CreateFileTool) Execute(ctx context.Context, in Input) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	p := strings.TrimSpace(in["path"])
	if p == "" {
		return Result{}, NewToolError(CodeInvalidInput, "create_file: missing required argument 'path'", nil)
	}
	content := in["content"]
	if len(content) > MaxFileBytes {
		return Result{}, NewToolError(CodeTooLarge, fmt.Sprintf("content exceeds limit %d bytes", MaxFileBytes), nil)
	}
	abs, err := validatePath(t.workspace, p)
	if err != nil {
		return Result{}, err
	}
	if _, statErr := os.Stat(abs); statErr == nil {
		if !strings.EqualFold(strings.TrimSpace(in["overwrite"]), "true") {
			return Result{Output: "", Err: NewToolError(CodeInvalidInput, fmt.Sprintf("file %q already exists; pass overwrite=true or use write_file/edit_file", p), nil)}, nil
		}
	}
	dir := filepath.Dir(abs)
	done := make(chan error, 1)
	go func() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			done <- err
			return
		}
		done <- os.WriteFile(abs, []byte(content), 0o644)
	}()
	select {
	case <-ctx.Done():
		return Result{}, NewToolError(CodeCancelled, "create_file cancelled", ctx.Err())
	case err := <-done:
		if err != nil {
			if os.IsPermission(err) {
				return Result{Output: "", Err: NewToolError(CodePermission, fmt.Sprintf("permission denied for %q", p), err)}, nil
			}
			return Result{Output: "", Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("create failed for %q", p), err)}, nil
		}
		meta := map[string]interface{}{"path": p, "abs": abs, "bytes": len(content)}
		return Result{Output: fmt.Sprintf("created %s (%d bytes)", p, len(content)), Metadata: meta}, nil
	}
}

// ---------- DeleteFile ----------

// DeleteFileTool deletes a file within the workspace. It always requires
// confirm=true (user approval) — this is enforced policy, not convention.
type DeleteFileTool struct {
	workspace string
}

func NewDeleteFileTool(workspace ...string) *DeleteFileTool {
	ws := "."
	if len(workspace) > 0 && strings.TrimSpace(workspace[0]) != "" {
		ws = workspace[0]
	}
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	return &DeleteFileTool{workspace: ws}
}

func (t *DeleteFileTool) Name() string { return "delete_file" }
func (t *DeleteFileTool) Description() string {
	return "Delete a file within the workspace. ALWAYS requires user approval via confirm=true after asking the user. Input: {\"path\": \"<relative path>\", \"confirm\": \"true\"}"
}
func (t *DeleteFileTool) InputSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Property{
			"path":    {Type: "string", Description: "Relative path of the file to delete"},
			"confirm": {Type: "string", Description: "Must be 'true' after the user approves the deletion"},
		},
		Required: []string{"path"},
	}
}

func (t *DeleteFileTool) Execute(ctx context.Context, in Input) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	p := strings.TrimSpace(in["path"])
	if p == "" {
		return Result{}, NewToolError(CodeInvalidInput, "delete_file: missing required argument 'path'", nil)
	}
	abs, err := validatePath(t.workspace, p)
	if err != nil {
		return Result{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(in["confirm"]), "true") {
		return Result{Output: "", Err: NewToolError(CodePermission, fmt.Sprintf("delete_file %q requires user approval: ask the user, then retry with confirm=true", p), nil)}, nil
	}
	fi, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return Result{Output: "", Err: NewToolError(CodeNotFound, fmt.Sprintf("file %q not found", p), err)}, nil
		}
		return Result{Output: "", Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("stat failed for %q", p), err)}, nil
	}
	if fi.IsDir() {
		return Result{Output: "", Err: NewToolError(CodeInvalidInput, fmt.Sprintf("%q is a directory; delete_file only removes files", p), nil)}, nil
	}
	done := make(chan error, 1)
	go func() { done <- os.Remove(abs) }()
	select {
	case <-ctx.Done():
		return Result{}, NewToolError(CodeCancelled, "delete_file cancelled", ctx.Err())
	case err := <-done:
		if err != nil {
			if os.IsPermission(err) {
				return Result{Output: "", Err: NewToolError(CodePermission, fmt.Sprintf("permission denied deleting %q", p), err)}, nil
			}
			return Result{Output: "", Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("delete failed for %q", p), err)}, nil
		}
		meta := map[string]interface{}{"path": p, "abs": abs, "deleted_bytes": fi.Size()}
		return Result{Output: fmt.Sprintf("deleted %s", p), Metadata: meta}, nil
	}
}

// ---------- ApplyPatch ----------

// ApplyPatchTool applies a unified diff to files in the workspace.
type ApplyPatchTool struct {
	workspace string
}

func NewApplyPatchTool(workspace ...string) *ApplyPatchTool {
	ws := "."
	if len(workspace) > 0 && strings.TrimSpace(workspace[0]) != "" {
		ws = workspace[0]
	}
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	return &ApplyPatchTool{workspace: ws}
}

func (t *ApplyPatchTool) Name() string { return "apply_patch" }
func (t *ApplyPatchTool) Description() string {
	return "Apply a unified diff (git-style patch) to one or more files in the workspace. Context lines must match exactly. Input: {\"patch\": \"--- a/file\\n+++ b/file\\n@@ ...\"}"
}
func (t *ApplyPatchTool) InputSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Property{
			"patch": {Type: "string", Description: "Unified diff text"},
		},
		Required: []string{"patch"},
	}
}

func (t *ApplyPatchTool) Execute(ctx context.Context, in Input) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	patch := in["patch"]
	if strings.TrimSpace(patch) == "" {
		return Result{}, NewToolError(CodeInvalidInput, "apply_patch: missing required argument 'patch'", nil)
	}
	patches, err := ParseUnifiedDiff(patch)
	if err != nil {
		return Result{}, err
	}
	var applied []string
	for _, fp := range patches {
		if err := checkContext(ctx); err != nil {
			return Result{}, err
		}
		abs, err := validatePath(t.workspace, fp.Path)
		if err != nil {
			return Result{}, err
		}
		data, readErr := os.ReadFile(abs)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				return Result{Output: "", Err: NewToolError(CodeNotFound, fmt.Sprintf("apply_patch: target file %q not found", fp.Path), readErr)}, nil
			}
			return Result{Output: "", Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("apply_patch: read failed for %q", fp.Path), readErr)}, nil
		}
		if len(data) > MaxFileBytes {
			return Result{}, NewToolError(CodeTooLarge, fmt.Sprintf("apply_patch: file %q exceeds size limit", fp.Path), nil)
		}
		patched, applyErr := ApplyFilePatch(string(data), fp)
		if applyErr != nil {
			return Result{Output: appliedReport(applied), Err: applyErr}, nil
		}
		if err := checkContext(ctx); err != nil {
			return Result{}, err
		}
		if writeErr := os.WriteFile(abs, []byte(patched), 0o644); writeErr != nil {
			if os.IsPermission(writeErr) {
				return Result{Output: appliedReport(applied), Err: NewToolError(CodePermission, fmt.Sprintf("permission denied writing %q", fp.Path), writeErr)}, nil
			}
			return Result{Output: appliedReport(applied), Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("apply_patch: write failed for %q", fp.Path), writeErr)}, nil
		}
		applied = append(applied, fp.Path)
	}
	return Result{Output: appliedReport(applied), Metadata: map[string]interface{}{"files": applied}}, nil
}

func appliedReport(files []string) string {
	if len(files) == 0 {
		return ""
	}
	return "applied patch to: " + strings.Join(files, ", ")
}

// ---------- ProjectInfo ----------

// ProjectInfoTool summarizes the detected project stack.
type ProjectInfoTool struct {
	workspace string
}

func NewProjectInfoTool(workspace ...string) *ProjectInfoTool {
	ws := "."
	if len(workspace) > 0 && strings.TrimSpace(workspace[0]) != "" {
		ws = workspace[0]
	}
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	return &ProjectInfoTool{workspace: ws}
}

func (t *ProjectInfoTool) Name() string { return "project_info" }
func (t *ProjectInfoTool) Description() string {
	return "Summarize the project: languages, file count, detected build/test tooling, and source directories. Use first to understand an unfamiliar project."
}
func (t *ProjectInfoTool) InputSchema() Schema {
	return Schema{Type: "object", Properties: map[string]Property{}, Required: []string{}}
}

func (t *ProjectInfoTool) Execute(ctx context.Context, _ Input) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	cfg := apccontext.DefaultConfig()
	cfg.Root = t.workspace
	type walkRes struct {
		res *apccontext.Result
		err error
	}
	ch := make(chan walkRes, 1)
	go func() {
		res, err := apccontext.WalkProject(t.workspace, cfg)
		ch <- walkRes{res, err}
	}()
	select {
	case <-ctx.Done():
		return Result{}, NewToolError(CodeCancelled, "project_info cancelled", ctx.Err())
	case r := <-ch:
		if r.err != nil {
			return Result{Output: "", Err: NewToolError(CodeExecutionFailed, "project_info walk failed", r.err)}, nil
		}
		stack := DetectStack(t.workspace)
		var b strings.Builder
		fmt.Fprintf(&b, "Root: %s\n", r.res.Root)
		fmt.Fprintf(&b, "Files discovered: %d\n", len(r.res.Files))
		var langs []string
		for l, c := range r.res.Languages {
			langs = append(langs, fmt.Sprintf("%s(%d)", l, c))
		}
		fmt.Fprintf(&b, "Languages: %s\n", strings.Join(langs, ", ")+"")
		fmt.Fprintf(&b, "Total tokens (approx): %d\n", r.res.TotalTokens)
		fmt.Fprintf(&b, "Test command: %s\n", orNone(stack.TestCmd))
		fmt.Fprintf(&b, "Build command: %s\n", orNone(stack.BuildCmd))
		fmt.Fprintf(&b, "Lint command: %s\n", orNone(stack.LintCmd))
		return Result{Output: b.String(), Metadata: map[string]interface{}{"stack_test": stack.TestCmd, "stack_build": stack.BuildCmd}}, nil
	}
}

func orNone(cmd []string) string {
	if len(cmd) == 0 {
		return "(none detected)"
	}
	return strings.Join(cmd, " ")
}

// Stack describes detected validation commands for a project.
type Stack struct {
	TestCmd  []string
	BuildCmd []string
	LintCmd  []string
}

// DetectStack inspects marker files in dir and returns the appropriate
// validation commands. Detection is file-presence based and offline.
func DetectStack(dir string) Stack {
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}
	switch {
	case has("go.mod"):
		return Stack{
			TestCmd:  []string{"go", "test", "./..."},
			BuildCmd: []string{"go", "build", "./..."},
			LintCmd:  []string{"go", "vet", "./..."},
		}
	case has("package.json"):
		s := Stack{}
		if data, err := os.ReadFile(filepath.Join(dir, "package.json")); err == nil {
			txt := string(data)
			if strings.Contains(txt, "\"test\"") {
				s.TestCmd = []string{"npm", "test"}
			}
			if strings.Contains(txt, "\"build\"") {
				s.BuildCmd = []string{"npm", "run", "build"}
			}
			if strings.Contains(txt, "\"lint\"") {
				s.LintCmd = []string{"npm", "run", "lint"}
			}
		}
		return s
	case has("pyproject.toml") || has("pytest.ini") || has("setup.py"):
		return Stack{
			TestCmd:  []string{"pytest"},
			BuildCmd: []string{"python", "-m", "compileall", "."},
		}
	case has("Cargo.toml"):
		return Stack{
			TestCmd:  []string{"cargo", "test"},
			BuildCmd: []string{"cargo", "build"},
		}
	default:
		return Stack{}
	}
}

// ---------- Validation tools ----------

// validationTool runs a detected stack command through RunCommandTool.
type validationTool struct {
	name string
	kind string // "test" | "build" | "lint"
	ws   string
}

func (t *validationTool) Name() string { return t.name }
func (t *validationTool) Description() string {
	switch t.kind {
	case "build":
		return "Run the project's build command (detected from go.mod/package.json/pyproject/Cargo). Read-only for the agent; executes project build tooling."
	default:
		return "Run the project's test/lint command (detected from go.mod/package.json/pyproject/Cargo). Executes project validation tooling."
	}
}
func (t *validationTool) InputSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Property{
			"args": {Type: "string", Description: "Extra arguments appended to the detected command (optional)"},
		},
		Required: []string{},
	}
}

func (t *validationTool) Execute(ctx context.Context, in Input) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	var cmd []string
	switch t.kind {
	case "test":
		cmd = DetectStack(t.ws).TestCmd
	case "build":
		cmd = DetectStack(t.ws).BuildCmd
	default:
		cmd = DetectStack(t.ws).LintCmd
	}
	if len(cmd) == 0 {
		return Result{Output: "", Err: NewToolError(CodeNotFound, fmt.Sprintf("%s: no %s command detected for this project (looked for go.mod, package.json, pyproject.toml/pytest.ini, Cargo.toml)", t.name, t.kind), nil)}, nil
	}
	if extra := strings.TrimSpace(in["args"]); extra != "" {
		cmd = append(append([]string{}, cmd...), splitArgs(extra)...)
	}
	runner := NewRunCommandTool(t.ws)
	in2 := Input{
		"command": cmd[0],
		"timeout": "120s",
	}
	if len(cmd) > 1 {
		in2["args"] = strings.Join(cmd[1:], " ")
	}
	res, err := runner.Execute(ctx, in2)
	if res.Metadata != nil {
		res.Metadata["validation_kind"] = t.kind
	}
	return res, err
}

func newValidationTools(workspace string) []Tool {
	return []Tool{
		&validationTool{name: "run_tests", kind: "test", ws: workspace},
		&validationTool{name: "run_build", kind: "build", ws: workspace},
		&validationTool{name: "run_lint", kind: "lint", ws: workspace},
	}
}

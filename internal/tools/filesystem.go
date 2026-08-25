package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ---------- ReadFile ----------

// ReadFileTool reads a file from the filesystem within workspace.
type ReadFileTool struct {
	workspace string
}

// NewReadFileTool creates a ReadFileTool. Workspace defaults to "." if not provided.
func NewReadFileTool(workspace ...string) *ReadFileTool {
	ws := "."
	if len(workspace) > 0 && strings.TrimSpace(workspace[0]) != "" {
		ws = workspace[0]
	}
	// Resolve to absolute for consistency, but keep as provided if it fails (validation will handle)
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	return &ReadFileTool{workspace: ws}
}

func (t *ReadFileTool) Name() string { return "ReadFile" }
func (t *ReadFileTool) Description() string {
	return "Read contents of a file within the project workspace. Input: {\"path\": \"<relative path>\"}"
}
func (t *ReadFileTool) InputSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Property{
			"path": {Type: "string", Description: "Relative path to file within workspace"},
		},
		Required: []string{"path"},
	}
}

func (t *ReadFileTool) Execute(ctx context.Context, in Input) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	p := strings.TrimSpace(in["path"])
	if p == "" {
		return Result{}, NewToolError(CodeInvalidInput, "ReadFile: missing required argument 'path'", nil)
	}
	abs, err := validatePath(t.workspace, p)
	if err != nil {
		return Result{}, err
	}

	// Check cancellation before IO
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}

	type readRes struct {
		data []byte
		err  error
	}
	ch := make(chan readRes, 1)
	go func() {
		data, err := os.ReadFile(abs)
		ch <- readRes{data, err}
	}()
	select {
	case <-ctx.Done():
		return Result{}, NewToolError(CodeCancelled, "ReadFile cancelled", ctx.Err())
	case res := <-ch:
		if res.err != nil {
			if os.IsNotExist(res.err) {
				return Result{Output: "", Err: NewToolError(CodeNotFound, fmt.Sprintf("file %q not found", p), res.err)}, nil
			}
			if os.IsPermission(res.err) {
				return Result{Output: "", Err: NewToolError(CodePermission, fmt.Sprintf("permission denied for %q", p), res.err)}, nil
			}
			return Result{Output: "", Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("read failed for %q", p), res.err)}, nil
		}
		content := string(res.data)
		truncated := false
		if len(content) > MaxFileBytes {
			content = content[:MaxFileBytes] + "\n...[truncated, limit 1 MiB]"
			truncated = true
		}
		// Also impose overall output limit
		if len(content) > MaxOutputBytes {
			content, _ = limitOutput(content)
			truncated = true
		}
		meta := map[string]interface{}{"path": p, "abs": abs, "size": len(res.data)}
		return Result{Output: content, Truncated: truncated, Metadata: meta}, nil
	}
}

// ---------- WriteFile ----------

// WriteFileTool writes content to a file within workspace.
type WriteFileTool struct {
	workspace string
}

func NewWriteFileTool(workspace ...string) *WriteFileTool {
	ws := "."
	if len(workspace) > 0 && strings.TrimSpace(workspace[0]) != "" {
		ws = workspace[0]
	}
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	return &WriteFileTool{workspace: ws}
}

func (t *WriteFileTool) Name() string { return "WriteFile" }
func (t *WriteFileTool) Description() string {
	return "Write content to a file within the project workspace. Input: {\"path\": \"<relative path>\", \"content\": \"<text>\"}"
}
func (t *WriteFileTool) InputSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Property{
			"path":    {Type: "string", Description: "Relative path to file within workspace"},
			"content": {Type: "string", Description: "Text content to write"},
		},
		Required: []string{"path", "content"},
	}
}

func (t *WriteFileTool) Execute(ctx context.Context, in Input) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	p := strings.TrimSpace(in["path"])
	if p == "" {
		return Result{}, NewToolError(CodeInvalidInput, "WriteFile: missing required argument 'path'", nil)
	}
	content := in["content"] // may be empty, that's allowed
	if len(content) > MaxFileBytes {
		return Result{}, NewToolError(CodeTooLarge, fmt.Sprintf("content exceeds limit %d bytes", MaxFileBytes), nil)
	}
	abs, err := validatePath(t.workspace, p)
	if err != nil {
		return Result{}, err
	}
	dir := filepath.Dir(abs)
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	ch := make(chan error, 1)
	go func() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			ch <- err
			return
		}
		ch <- os.WriteFile(abs, []byte(content), 0o644)
	}()
	select {
	case <-ctx.Done():
		return Result{}, NewToolError(CodeCancelled, "WriteFile cancelled", ctx.Err())
	case err := <-ch:
		if err != nil {
			if os.IsPermission(err) {
				return Result{Output: "", Err: NewToolError(CodePermission, fmt.Sprintf("permission denied for %q", p), err)}, nil
			}
			return Result{Output: "", Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("write failed for %q", p), err)}, nil
		}
		meta := map[string]interface{}{"path": p, "abs": abs, "bytes": len(content)}
		return Result{Output: fmt.Sprintf("wrote %d bytes to %s", len(content), p), Metadata: meta}, nil
	}
}

// ---------- ListDirectory ----------

// ListDirectoryTool lists files in a directory within workspace.
type ListDirectoryTool struct {
	workspace string
}

func NewListDirectoryTool(workspace ...string) *ListDirectoryTool {
	ws := "."
	if len(workspace) > 0 && strings.TrimSpace(workspace[0]) != "" {
		ws = workspace[0]
	}
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	return &ListDirectoryTool{workspace: ws}
}

func (t *ListDirectoryTool) Name() string { return "ListDirectory" }
func (t *ListDirectoryTool) Description() string {
	return "List files and directories within the project workspace. Input: {\"path\": \"<relative dir path, default '.'>\"}"
}
func (t *ListDirectoryTool) InputSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Property{
			"path": {Type: "string", Description: "Relative directory path within workspace, defaults to '.'"},
		},
		Required: []string{},
	}
}

func (t *ListDirectoryTool) Execute(ctx context.Context, in Input) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	p := strings.TrimSpace(in["path"])
	if p == "" {
		p = "."
	}
	abs, err := validatePath(t.workspace, p)
	if err != nil {
		return Result{}, err
	}
	type listRes struct {
		entries []os.DirEntry
		err     error
	}
	ch := make(chan listRes, 1)
	go func() {
		entries, err := os.ReadDir(abs)
		ch <- listRes{entries, err}
	}()
	select {
	case <-ctx.Done():
		return Result{}, NewToolError(CodeCancelled, "ListDirectory cancelled", ctx.Err())
	case res := <-ch:
		if res.err != nil {
			if os.IsNotExist(res.err) {
				return Result{Output: "", Err: NewToolError(CodeNotFound, fmt.Sprintf("directory %q not found", p), res.err)}, nil
			}
			if os.IsPermission(res.err) {
				return Result{Output: "", Err: NewToolError(CodePermission, fmt.Sprintf("permission denied for %q", p), res.err)}, nil
			}
			return Result{Output: "", Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("list failed for %q", p), res.err)}, nil
		}
		truncated := false
		entries := res.entries
		if len(entries) > MaxDirectoryEntries {
			entries = entries[:MaxDirectoryEntries]
			truncated = true
		}
		// Check context per entry? Already got entries, just format.
		if err := checkContext(ctx); err != nil {
			return Result{}, err
		}
		var b strings.Builder
		for _, e := range entries {
			select {
			case <-ctx.Done():
				return Result{}, NewToolError(CodeCancelled, "ListDirectory cancelled", ctx.Err())
			default:
			}
			name := e.Name()
			if e.IsDir() {
				name += "/"
			}
			b.WriteString(name + "\n")
		}
		output := b.String()
		if truncated {
			output += fmt.Sprintf("...[truncated, showing %d of %d entries, limit %d]\n", MaxDirectoryEntries, len(res.entries), MaxDirectoryEntries)
		}
		if len(output) > MaxOutputBytes {
			output, _ = limitOutput(output)
			truncated = true
		}
		meta := map[string]interface{}{"path": p, "abs": abs, "count": len(entries), "truncated": truncated}
		return Result{Output: output, Truncated: truncated, Metadata: meta}, nil
	}
}

// ListFilesTool is an alias for ListDirectoryTool for backward compatibility.
type ListFilesTool struct {
	inner *ListDirectoryTool
}

func NewListFilesTool(workspace ...string) *ListFilesTool {
	return &ListFilesTool{inner: NewListDirectoryTool(workspace...)}
}
func (t *ListFilesTool) Name() string        { return "list_files" }
func (t *ListFilesTool) Description() string { return t.inner.Description() }
func (t *ListFilesTool) InputSchema() Schema { return t.inner.InputSchema() }
func (t *ListFilesTool) Execute(ctx context.Context, in Input) (Result, error) {
	return t.inner.Execute(ctx, in)
}

// ---------- RunCommand ----------

// RunCommandTool runs a command within workspace without shell.
type RunCommandTool struct {
	workspace string
}

func NewRunCommandTool(workspace ...string) *RunCommandTool {
	ws := "."
	if len(workspace) > 0 && strings.TrimSpace(workspace[0]) != "" {
		ws = workspace[0]
	}
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	return &RunCommandTool{workspace: ws}
}

func (t *RunCommandTool) Name() string { return "RunCommand" }
func (t *RunCommandTool) Description() string {
	return "Run a terminal command within the workspace. Input: {\"command\": \"<executable>\", \"args\": \"<optional space-separated args>\", \"dir\": \"<optional relative workdir>\", \"timeout\": \"<optional duration like 5s>\"} Captures stdout/stderr, respects cancellation and output limits. Never uses shell."
}
func (t *RunCommandTool) InputSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Property{
			"command": {Type: "string", Description: "Executable name or path (e.g., go, git, ls)"},
			"args":    {Type: "string", Description: "Space-separated arguments (optional)"},
			"dir":     {Type: "string", Description: "Relative working directory within workspace (optional, defaults to workspace root)"},
			"timeout": {Type: "string", Description: "Timeout duration like 5s, 1m (optional, default 30s)"},
		},
		Required: []string{"command"},
	}
}

func (t *RunCommandTool) Execute(ctx context.Context, in Input) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	cmdStr := strings.TrimSpace(in["command"])
	if cmdStr == "" {
		return Result{}, NewToolError(CodeInvalidInput, "RunCommand: missing required argument 'command'", nil)
	}
	// Parse command: if command contains spaces and no args provided, split.
	command := cmdStr
	argsStr := strings.TrimSpace(in["args"])
	var args []string
	if argsStr != "" {
		// Simple split respecting quotes? Use naive fields for now, but handle quotes.
		args = splitArgs(argsStr)
	} else {
		// If command string itself contains spaces, try to split into command + args
		// But to avoid shell injection, we only do this if command contains space and args empty.
		// Prefer explicit args; if caller passed "go test ./..." as command, we split.
		if strings.Contains(command, " ") {
			parts := splitArgs(command)
			if len(parts) > 0 {
				command = parts[0]
				if len(parts) > 1 {
					args = parts[1:]
				}
			}
		}
	}
	// Validate command is not empty and not containing shell metachars when using direct exec.
	if strings.ContainsAny(command, "&|;`$><*?") {
		// We still allow but warn; since we don't use shell, these are literal.
		// To prevent silent shell injection, we reject commands that look like shell injection attempts
		// when they contain shell operators as separate tokens.
		// However single executable with such chars is suspicious; reject.
		return Result{}, NewToolError(CodeInvalidInput, fmt.Sprintf("RunCommand: command %q contains shell metacharacters, use explicit args instead", command), nil)
	}
	for _, a := range args {
		if a == "&&" || a == "||" || a == ";" || a == "|" {
			return Result{}, NewToolError(CodeInvalidInput, "RunCommand: shell operators not allowed, use explicit command without shell", nil)
		}
	}

	dirInput := strings.TrimSpace(in["dir"])
	workDir := t.workspace
	if dirInput != "" {
		absDir, err := validatePath(t.workspace, dirInput)
		if err != nil {
			return Result{}, err
		}
		// Verify it's a directory
		fi, err := os.Stat(absDir)
		if err != nil {
			if os.IsNotExist(err) {
				return Result{Output: "", Err: NewToolError(CodeNotFound, fmt.Sprintf("workdir %q not found", dirInput), err)}, nil
			}
			return Result{Output: "", Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("cannot stat workdir %q", dirInput), err)}, nil
		}
		if !fi.IsDir() {
			return Result{}, NewToolError(CodeInvalidInput, fmt.Sprintf("workdir %q is not a directory", dirInput), nil)
		}
		workDir = absDir
	} else {
		// Resolve workspace absolute
		if abs, err := filepath.Abs(t.workspace); err == nil {
			workDir = abs
		}
	}

	timeoutStr := strings.TrimSpace(in["timeout"])
	timeout := 30 * time.Second
	if timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr); err == nil && d > 0 {
			timeout = d
			if timeout > 5*time.Minute {
				timeout = 5 * time.Minute
			}
		} else {
			return Result{}, NewToolError(CodeInvalidInput, fmt.Sprintf("invalid timeout %q", timeoutStr), err)
		}
	}

	// Create context with timeout but respect parent cancellation
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Never use shell; direct exec
	cmd := exec.CommandContext(execCtx, command, args...)
	cmd.Dir = workDir
	// Ensure we capture both stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run
	err := cmd.Run()
	// Capture combined output with limits
	combined := stdout.String()
	if stderr.Len() > 0 {
		if combined != "" {
			combined += "\n"
		}
		combined += stderr.String()
	}
	truncated := false
	if len(combined) > MaxCommandOutputBytes {
		combined = combined[:MaxCommandOutputBytes] + "\n...[command output truncated, limit 256 KiB]"
		truncated = true
	}
	// Also check overall output limit
	if len(combined) > MaxOutputBytes {
		combined, _ = limitOutput(combined)
		truncated = true
	}

	if execCtx.Err() == context.DeadlineExceeded {
		return Result{Output: combined, Truncated: truncated, Metadata: map[string]interface{}{"command": command, "args": args, "dir": workDir, "timed_out": true}, Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("command timed out after %s", timeout), execCtx.Err())}, nil
	}
	if execCtx.Err() == context.Canceled {
		return Result{}, NewToolError(CodeCancelled, "RunCommand cancelled", execCtx.Err())
	}
	// Check parent ctx
	select {
	case <-ctx.Done():
		return Result{}, NewToolError(CodeCancelled, "RunCommand cancelled", ctx.Err())
	default:
	}

	meta := map[string]interface{}{"command": command, "args": args, "dir": workDir, "truncated": truncated}
	if err != nil {
		// Command failed: return output plus structured error in Result.Err (not transport error)
		meta["exit_error"] = err.Error()
		return Result{Output: combined, Truncated: truncated, Metadata: meta, Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("command %q failed: %v", command, err), err)}, nil
	}
	return Result{Output: combined, Truncated: truncated, Metadata: meta}, nil
}

// splitArgs splits a string into args respecting simple quoted strings.
func splitArgs(s string) []string {
	var args []string
	var cur strings.Builder
	inQuote := false
	quoteChar := rune(0)
	escaped := false
	for _, r := range s {
		if escaped {
			cur.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if inQuote {
			if r == quoteChar {
				inQuote = false
				continue
			}
			cur.WriteRune(r)
			continue
		}
		if r == '"' || r == '\'' {
			inQuote = true
			quoteChar = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' {
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}

// isWindows helper
func isWindows() bool { return filepath.Separator == '\\' }

// ---------- MockTool for tests ----------

// MockTool is a configurable tool for tests.
type MockTool struct {
	name        string
	description string
	ExecuteFunc func(ctx context.Context, in Input) (Result, error)
	Calls       []Input
}

func NewMockTool(name, description string, fn func(ctx context.Context, in Input) (Result, error)) *MockTool {
	if name == "" {
		name = "mock_tool"
	}
	if description == "" {
		description = "mock tool for testing"
	}
	if fn == nil {
		fn = func(_ context.Context, _ Input) (Result, error) {
			return Result{Output: "mock output"}, nil
		}
	}
	return &MockTool{name: name, description: description, ExecuteFunc: fn}
}

func (m *MockTool) Name() string        { return m.name }
func (m *MockTool) Description() string { return m.description }
func (m *MockTool) InputSchema() Schema {
	return Schema{
		Type:       "object",
		Properties: map[string]Property{"input": {Type: "string", Description: "mock input"}},
		Required:   []string{},
	}
}
func (m *MockTool) Execute(ctx context.Context, in Input) (Result, error) {
	m.Calls = append(m.Calls, in)
	return m.ExecuteFunc(ctx, in)
}

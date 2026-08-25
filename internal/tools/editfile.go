package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EditFileTool edits a file by replacing old_string with new_string.
type EditFileTool struct {
	workspace string
}

func NewEditFileTool(workspace ...string) *EditFileTool {
	ws := "."
	if len(workspace) > 0 && strings.TrimSpace(workspace[0]) != "" {
		ws = workspace[0]
	}
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	return &EditFileTool{workspace: ws}
}

func (t *EditFileTool) Name() string { return "EditFile" }
func (t *EditFileTool) Description() string {
	return "Edit a file within the workspace by replacing text. Input: {\"path\": \"<relative path>\", \"old_string\": \"<text to replace>\", \"new_string\": \"<replacement>\"} Supports cancellation, output limits, and workspace restriction."
}
func (t *EditFileTool) InputSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Property{
			"path":       {Type: "string", Description: "Relative path to file within workspace"},
			"old_string": {Type: "string", Description: "Text to search for and replace (must exist in file)"},
			"new_string": {Type: "string", Description: "Replacement text (may be empty)"},
		},
		Required: []string{"path", "old_string"},
	}
}

func (t *EditFileTool) Execute(ctx context.Context, in Input) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	p := strings.TrimSpace(in["path"])
	if p == "" {
		return Result{}, NewToolError(CodeInvalidInput, "EditFile: missing required argument 'path'", nil)
	}
	oldStr := in["old_string"]
	// Note: allow empty old_string? That would be ambiguous; require non-empty
	if oldStr == "" && in["old_string"] == "" {
		// Check if key missing vs empty value: we treat empty as invalid unless explicitly allowed for insertion?
		// For safety, require old_string to be present and non-empty unless file is empty?
		// But we allow creating file if old_string empty and file empty? Simplify: if old_string == "" and not provided, error.
		// Use presence check via map
		if _, ok := in["old_string"]; !ok {
			return Result{}, NewToolError(CodeInvalidInput, "EditFile: missing required argument 'old_string'", nil)
		}
		// If old_string is empty, we treat as error unless new_string is insertion at start?
		// Reject empty old_string to prevent replacing every position.
		return Result{}, NewToolError(CodeInvalidInput, "EditFile: old_string cannot be empty", nil)
	}
	newStr := in["new_string"] // may be empty (deletion)

	abs, err := validatePath(t.workspace, p)
	if err != nil {
		return Result{}, err
	}
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}

	// Read file with cancellation
	type readRes struct {
		data []byte
		err  error
	}
	ch := make(chan readRes, 1)
	go func() {
		data, err := os.ReadFile(abs)
		ch <- readRes{data, err}
	}()
	var data []byte
	select {
	case <-ctx.Done():
		return Result{}, NewToolError(CodeCancelled, "EditFile cancelled", ctx.Err())
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
		data = res.data
	}

	if len(data) > MaxFileBytes {
		return Result{}, NewToolError(CodeTooLarge, fmt.Sprintf("file %q exceeds size limit %d bytes", p, MaxFileBytes), nil)
	}
	content := string(data)
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	// Check old_string exists
	if !strings.Contains(content, oldStr) {
		return Result{Output: "", Err: NewToolError(CodeNotFound, fmt.Sprintf("old_string not found in %q", p), nil)}, nil
	}
	// Perform replacement: replace first occurrence only to be safe; if user wants replace all they can call multiple times.
	// We also support replace_all flag via Input["replace_all"] == "true"
	replaceAll := false
	if v, ok := in["replace_all"]; ok && strings.ToLower(strings.TrimSpace(v)) == "true" {
		replaceAll = true
	}
	var newContent string
	var count int
	if replaceAll {
		newContent = strings.ReplaceAll(content, oldStr, newStr)
		count = strings.Count(content, oldStr)
	} else {
		newContent = strings.Replace(content, oldStr, newStr, 1)
		count = 1
	}
	if len(newContent) > MaxFileBytes {
		return Result{}, NewToolError(CodeTooLarge, fmt.Sprintf("resulting file would exceed limit %d bytes", MaxFileBytes), nil)
	}
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	// Write back
	dir := filepath.Dir(abs)
	writeCh := make(chan error, 1)
	go func() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			writeCh <- err
			return
		}
		writeCh <- os.WriteFile(abs, []byte(newContent), 0o644)
	}()
	select {
	case <-ctx.Done():
		return Result{}, NewToolError(CodeCancelled, "EditFile cancelled during write", ctx.Err())
	case err := <-writeCh:
		if err != nil {
			if os.IsPermission(err) {
				return Result{Output: "", Err: NewToolError(CodePermission, fmt.Sprintf("permission denied for %q", p), err)}, nil
			}
			return Result{Output: "", Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("write failed for %q", p), err)}, nil
		}
		output := fmt.Sprintf("edited %s: replaced %d occurrence(s) of %q", p, count, truncatePreview(oldStr, 40))
		meta := map[string]interface{}{"path": p, "abs": abs, "replacements": count, "old_len": len(oldStr), "new_len": len(newStr)}
		return Result{Output: output, Metadata: meta}, nil
	}
}

func truncatePreview(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

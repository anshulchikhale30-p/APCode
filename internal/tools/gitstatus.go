package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GitStatusTool shows git status.
type GitStatusTool struct {
	workspace string
}

func NewGitStatusTool(workspace ...string) *GitStatusTool {
	ws := "."
	if len(workspace) > 0 && strings.TrimSpace(workspace[0]) != "" {
		ws = workspace[0]
	}
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	return &GitStatusTool{workspace: ws}
}

func (t *GitStatusTool) Name() string { return "GitStatus" }
func (t *GitStatusTool) Description() string {
	return "Show git status for workspace. Input: {\"short\": \"<true/false>\"}"
}
func (t *GitStatusTool) InputSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Property{
			"short": {Type: "string", Description: "If 'true', use --short format"},
		},
		Required: []string{},
	}
}
func (t *GitStatusTool) Execute(ctx context.Context, in Input) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	ws, err := resolveWorkspace(t.workspace)
	if err != nil {
		return Result{}, err
	}
	if err := checkGitRepo(ctx, ws); err != nil {
		return Result{Output: "", Err: err}, nil
	}
	args := []string{"status"}
	if strings.ToLower(strings.TrimSpace(in["short"])) == "true" {
		args = append(args, "--short")
	}
	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(execCtx, "git", args...)
	cmd.Dir = ws
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err = cmd.Run()
	combined := out.String()
	if errBuf.Len() > 0 {
		if combined != "" {
			combined += "\n"
		}
		combined += errBuf.String()
	}
	select {
	case <-ctx.Done():
		return Result{}, NewToolError(CodeCancelled, "GitStatus cancelled", ctx.Err())
	default:
	}
	if execCtx.Err() == context.DeadlineExceeded {
		return Result{Output: combined, Err: NewToolError(CodeExecutionFailed, "git status timed out", execCtx.Err())}, nil
	}
	truncated := false
	if len(combined) > MaxOutputBytes {
		combined, _ = limitOutput(combined)
		truncated = true
	}
	meta := map[string]interface{}{"args": args}
	if err != nil {
		return Result{Output: combined, Truncated: truncated, Metadata: meta, Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("git status failed: %v", err), err)}, nil
	}
	return Result{Output: combined, Truncated: truncated, Metadata: meta}, nil
}

// GitLogTool shows git log.
type GitLogTool struct {
	workspace string
}

func NewGitLogTool(workspace ...string) *GitLogTool {
	ws := "."
	if len(workspace) > 0 && strings.TrimSpace(workspace[0]) != "" {
		ws = workspace[0]
	}
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	return &GitLogTool{workspace: ws}
}

func (t *GitLogTool) Name() string { return "GitLog" }
func (t *GitLogTool) Description() string {
	return "Show git log for workspace. Input: {\"args\": \"<optional git log args>\", \"limit\": \"<max entries, default 20>\"}"
}
func (t *GitLogTool) InputSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Property{
			"args":  {Type: "string", Description: "Additional git log arguments (optional)"},
			"limit": {Type: "string", Description: "Max log entries (default 20)"},
		},
		Required: []string{},
	}
}
func (t *GitLogTool) Execute(ctx context.Context, in Input) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	ws, err := resolveWorkspace(t.workspace)
	if err != nil {
		return Result{}, err
	}
	if err := checkGitRepo(ctx, ws); err != nil {
		return Result{Output: "", Err: err}, nil
	}
	limitStr := strings.TrimSpace(in["limit"])
	limit := 20
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
		if limit <= 0 {
			limit = 20
		}
		if limit > 100 {
			limit = 100
		}
	}
	args := []string{"log", fmt.Sprintf("-n%d", limit), "--oneline", "--decorate"}
	argsStr := strings.TrimSpace(in["args"])
	if argsStr != "" {
		parts := splitArgs(argsStr)
		for _, p := range parts {
			if p == "&&" || p == "||" || p == ";" || p == "|" {
				return Result{}, NewToolError(CodeInvalidInput, "GitLog: shell operators not allowed", nil)
			}
		}
		args = append(args, parts...)
	}
	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(execCtx, "git", args...)
	cmd.Dir = ws
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err = cmd.Run()
	combined := out.String()
	if errBuf.Len() > 0 {
		if combined != "" {
			combined += "\n"
		}
		combined += errBuf.String()
	}
	select {
	case <-ctx.Done():
		return Result{}, NewToolError(CodeCancelled, "GitLog cancelled", ctx.Err())
	default:
	}
	truncated := false
	if len(combined) > MaxOutputBytes {
		combined, _ = limitOutput(combined)
		truncated = true
	}
	meta := map[string]interface{}{"args": args}
	if err != nil {
		return Result{Output: combined, Truncated: truncated, Metadata: meta, Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("git log failed: %v", err), err)}, nil
	}
	return Result{Output: combined, Truncated: truncated, Metadata: meta}, nil
}

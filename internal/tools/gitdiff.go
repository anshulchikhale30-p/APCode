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

// GitDiffTool shows git diff within workspace.
type GitDiffTool struct {
	workspace string
}

func NewGitDiffTool(workspace ...string) *GitDiffTool {
	ws := "."
	if len(workspace) > 0 && strings.TrimSpace(workspace[0]) != "" {
		ws = workspace[0]
	}
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	return &GitDiffTool{workspace: ws}
}

func (t *GitDiffTool) Name() string { return "GitDiff" }
func (t *GitDiffTool) Description() string {
	return "Show git diff for workspace. Input: {\"args\": \"<optional git diff args>\", \"path\": \"<optional relative path>\", \"staged\": \"<true/false>\", \"stat\": \"<true/false>\"}"
}
func (t *GitDiffTool) InputSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Property{
			"args":   {Type: "string", Description: "Additional git diff arguments (optional)"},
			"path":   {Type: "string", Description: "Relative path to limit diff (optional)"},
			"staged": {Type: "string", Description: "If 'true', show staged diff (--staged)"},
			"stat":   {Type: "string", Description: "If 'true', show --stat"},
		},
		Required: []string{},
	}
}

func (t *GitDiffTool) Execute(ctx context.Context, in Input) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	ws := t.workspace
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	ws, err := resolveWorkspace(ws)
	if err != nil {
		return Result{}, err
	}
	// Check repo
	if err := checkGitRepo(ctx, ws); err != nil {
		return Result{Output: "", Err: err}, nil
	}
	argsStr := strings.TrimSpace(in["args"])
	pathInput := strings.TrimSpace(in["path"])
	stagedStr := strings.ToLower(strings.TrimSpace(in["staged"]))
	statStr := strings.ToLower(strings.TrimSpace(in["stat"]))

	var args []string
	args = append(args, "diff")
	if stagedStr == "true" || stagedStr == "1" {
		args = append(args, "--staged")
	}
	if statStr == "true" || statStr == "1" {
		args = append(args, "--stat")
	}
	if argsStr != "" {
		parts := splitArgs(argsStr)
		for _, p := range parts {
			if p == "&&" || p == "||" || p == ";" || p == "|" || strings.Contains(p, "`") {
				return Result{}, NewToolError(CodeInvalidInput, fmt.Sprintf("GitDiff: shell operators not allowed in %q", p), nil)
			}
			if strings.HasPrefix(p, "--output=") {
				return Result{}, NewToolError(CodeInvalidInput, "GitDiff: --output not allowed", nil)
			}
			if strings.Contains(p, ";") || strings.Contains(p, "|") || strings.Contains(p, "&") {
				return Result{}, NewToolError(CodeInvalidInput, fmt.Sprintf("GitDiff: invalid arg %q", p), nil)
			}
		}
		args = append(args, parts...)
	}
	if pathInput != "" {
		abs, err := validatePath(ws, pathInput)
		if err != nil {
			return Result{}, err
		}
		rel, err := filepath.Rel(ws, abs)
		if err == nil {
			args = append(args, "--", filepath.ToSlash(rel))
		} else {
			args = append(args, "--", pathInput)
		}
	}
	execCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(execCtx, "git", args...)
	cmd.Dir = ws
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	combined := stdout.String()
	if stderr.Len() > 0 {
		if combined != "" {
			combined += "\n"
		}
		combined += stderr.String()
	}
	// Check cancellation
	select {
	case <-ctx.Done():
		return Result{}, NewToolError(CodeCancelled, "GitDiff cancelled", ctx.Err())
	default:
	}
	if execCtx.Err() == context.DeadlineExceeded {
		trunc := false
		if len(combined) > MaxOutputBytes {
			combined, _ = limitOutput(combined)
			trunc = true
		}
		return Result{Output: combined, Truncated: trunc, Metadata: map[string]interface{}{"args": args, "timed_out": true}, Err: NewToolError(CodeExecutionFailed, "git diff timed out", execCtx.Err())}, nil
	}
	if execCtx.Err() == context.Canceled {
		return Result{}, NewToolError(CodeCancelled, "GitDiff cancelled", ctx.Err())
	}
	truncated := false
	if len(combined) > MaxCommandOutputBytes {
		combined = combined[:MaxCommandOutputBytes] + "\n...[git diff output truncated, limit 256 KiB]"
		truncated = true
	}
	if len(combined) > MaxOutputBytes {
		combined, _ = limitOutput(combined)
		truncated = true
	}
	meta := map[string]interface{}{"args": args, "truncated": truncated}
	if err != nil {
		// git diff exit code with differences is 0, error only for real failures
		// If combined empty and error, check if not a repo etc already handled
		if combined != "" {
			return Result{Output: combined, Truncated: truncated, Metadata: meta, Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("git diff failed: %v", err), err)}, nil
		}
		// If error and no output, return error
		if strings.Contains(stderr.String(), "not a git repository") {
			return Result{Output: "", Err: NewToolError(CodeNotGitRepo, "not a git repository", err)}, nil
		}
		return Result{Output: combined, Truncated: truncated, Metadata: meta, Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("git diff failed: %v", err), err)}, nil
	}
	// Do not return "no changes" placeholder; return empty for no diff to match test expectations
	return Result{Output: combined, Truncated: truncated, Metadata: meta}, nil
}

func checkGitRepo(ctx context.Context, dir string) error {
	if err := checkContext(ctx); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return nil
	}
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		select {
		case <-ctx.Done():
			return NewToolError(CodeCancelled, "checkGitRepo cancelled", ctx.Err())
		default:
		}
		msg := strings.TrimSpace(out.String())
		if strings.Contains(strings.ToLower(msg), "not a git repository") {
			return NewToolError(CodeNotGitRepo, fmt.Sprintf("workspace %q is not a git repository", dir), err)
		}
		return NewToolError(CodeNotGitRepo, fmt.Sprintf("workspace %q is not a git repository", dir), err)
	}
	if strings.TrimSpace(out.String()) == "true" {
		return nil
	}
	return NewToolError(CodeNotGitRepo, fmt.Sprintf("workspace %q is not a git repository", dir), nil)
}

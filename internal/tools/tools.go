// Package tools defines the tool interface the agent will use to act on
// the user's project (read files, edit files, run commands).
//
// Milestone 11: safe Tool interface with path traversal prevention,
// workspace restriction, output limits, cancellation, and structured errors.
package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Limits.
const (
	// MaxFileBytes is the maximum file size allowed for read/write/edit.
	MaxFileBytes = 1 << 20 // 1 MiB

	// MaxOutputBytes is the maximum output size for any tool.
	MaxOutputBytes = 1 << 20 // 1 MiB

	// MaxDirectoryEntries limits directory listings.
	MaxDirectoryEntries = 500

	// MaxSearchResults limits search results.
	MaxSearchResults = 200

	// MaxCommandOutputBytes limits command stdout/stderr capture.
	MaxCommandOutputBytes = 256 * 1024 // 256 KiB
)

// Tool error codes for structured errors.
const (
	CodeInvalidInput     = "INVALID_INPUT"
	CodePathTraversal    = "PATH_TRAVERSAL"
	CodeOutsideWorkspace = "OUTSIDE_WORKSPACE"
	CodeNotFound         = "NOT_FOUND"
	CodePermission       = "PERMISSION_DENIED"
	CodeTooLarge         = "OUTPUT_TOO_LARGE"
	CodeCancelled        = "CANCELLED"
	CodeExecutionFailed  = "EXECUTION_FAILED"
	CodeNotGitRepo       = "NOT_GIT_REPO"
)

// Sentinel errors.
var (
	ErrNotImplemented        = errors.New("tools: not implemented")
	ErrToolNotFound          = errors.New("tools: tool not found")
	ErrToolAlreadyRegistered = errors.New("tools: tool already registered")
)

// ToolError is a structured error with a machine-readable code.
type ToolError struct {
	Code    string
	Message string
	Cause   error
}

func (e *ToolError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *ToolError) Unwrap() error { return e.Cause }

// NewToolError creates a ToolError.
func NewToolError(code, message string, cause error) *ToolError {
	return &ToolError{Code: code, Message: message, Cause: cause}
}

// IsToolError reports whether err is a ToolError with the given code.
func IsToolError(err error, code string) bool {
	var te *ToolError
	if errors.As(err, &te) {
		return te.Code == code
	}
	return false
}

// Input carries raw arguments for a tool invocation.
type Input map[string]string

// Output is the structured result content.
type Output struct {
	Content   string                 `json:"content"`
	Truncated bool                   `json:"truncated"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Result reports what a tool did.
// For backward compatibility, Output is a string; Truncated and Metadata provide structured flags.
// The Err field holds a tool-level application error (e.g., file not found), while the
// returned error is a transport/execution failure (e.g., context cancelled, output too large).
type Result struct {
	Output    string                 `json:"output"`
	Err       error                  `json:"-"`
	Truncated bool                   `json:"truncated"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Schema describes tool input parameters for prompt generation and validation.
type Schema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required"`
}

// Property describes a single input parameter.
type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// Tool is a single capability the agent may invoke.
type Tool interface {
	// Name returns the unique tool identifier.
	Name() string
	// Description explains what the tool does.
	Description() string
	// InputSchema returns the JSON schema for the tool's input.
	InputSchema() Schema
	// Execute runs the tool.
	Execute(ctx context.Context, in Input) (Result, error)
}

// Registry holds available agent tools and provides lookup.
type Registry struct {
	mu         sync.RWMutex
	tools      map[string]Tool
	normalized map[string]Tool // lower+no-underscore -> tool
	workspace  string
}

// NewRegistry creates an empty tool registry with no workspace restriction.
// For workspace-aware registry, use NewRegistryWithWorkspace.
func NewRegistry() *Registry {
	return &Registry{
		tools:      make(map[string]Tool),
		normalized: make(map[string]Tool),
		workspace:  "",
	}
}

// NewRegistryWithWorkspace creates a registry bound to a workspace directory.
func NewRegistryWithWorkspace(workspace string) (*Registry, error) {
	ws, err := resolveWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	return &Registry{
		tools:      make(map[string]Tool),
		normalized: make(map[string]Tool),
		workspace:  ws,
	}, nil
}

// Workspace returns the registry workspace, empty if unrestricted.
func (r *Registry) Workspace() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.workspace
}

// normalizeName returns a normalized form for case/underscore-insensitive lookup.
func normalizeName(name string) string {
	n := strings.TrimSpace(name)
	n = strings.ToLower(n)
	n = strings.ReplaceAll(n, "_", "")
	n = strings.ReplaceAll(n, "-", "")
	return n
}

// Register adds a tool to the registry. Returns error if name already exists or tool is nil.
func (r *Registry) Register(t Tool) error {
	if t == nil {
		return errors.New("tools: cannot register nil tool")
	}
	name := strings.TrimSpace(t.Name())
	if name == "" {
		return errors.New("tools: tool name cannot be empty")
	}
	norm := normalizeName(name)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("%w: %q", ErrToolAlreadyRegistered, name)
	}
	if _, exists := r.normalized[norm]; exists {
		return fmt.Errorf("%w: %q (normalized collision)", ErrToolAlreadyRegistered, name)
	}
	r.tools[name] = t
	r.normalized[norm] = t
	return nil
}

// MustRegister registers a tool and panics on error. Useful for init.
func (r *Registry) MustRegister(t Tool) {
	if err := r.Register(t); err != nil {
		panic(err)
	}
}

// Get returns a tool by name, using case/underscore-insensitive lookup.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	// exact first
	if t, ok := r.tools[strings.TrimSpace(name)]; ok {
		return t, true
	}
	// normalized
	norm := normalizeName(name)
	if t, ok := r.normalized[norm]; ok {
		return t, true
	}
	// Aliases for backward compatibility
	if norm == "listfiles" {
		if t, ok := r.normalized["listdirectory"]; ok {
			return t, true
		}
	}
	if norm == "listdirectory" {
		if t, ok := r.normalized["listfiles"]; ok {
			return t, true
		}
	}
	// search alias: pattern vs query
	if norm == "searchfiles" {
		// already normalized same, but handle?
		if t, ok := r.normalized["searchfiles"]; ok {
			return t, true
		}
	}
	// Spec aliases: search -> searchfiles, shell -> runcommand
	if norm == "search" {
		if t, ok := r.normalized["searchfiles"]; ok {
			return t, true
		}
		if t, ok := r.normalized["search"]; ok {
			return t, true
		}
	}
	if norm == "shell" {
		if t, ok := r.normalized["runcommand"]; ok {
			return t, true
		}
		if t, ok := r.normalized["shell"]; ok {
			return t, true
		}
	}
	return nil, false
}

// List returns all tools sorted by name for determinism.
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// Names returns sorted tool names.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Count returns number of registered tools.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// Execute is a helper that looks up and executes a tool.
func (r *Registry) Execute(ctx context.Context, name string, in Input) (Result, error) {
	t, ok := r.Get(name)
	if !ok {
		return Result{}, NewToolError(CodeNotFound, fmt.Sprintf("tool %q not found", name), ErrToolNotFound)
	}
	return t.Execute(ctx, in)
}

// DefaultRegistry returns a registry with built-in filesystem and terminal tools.
// Workspace defaults to current directory.
func DefaultRegistry() *Registry {
	r := NewRegistry()
	_ = r.Register(NewReadFileTool())
	_ = r.Register(NewWriteFileTool())
	_ = r.Register(NewEditFileTool())
	_ = r.Register(NewListDirectoryTool())
	_ = r.Register(NewSearchFilesTool())
	_ = r.Register(NewRunCommandTool())
	_ = r.Register(NewGitDiffTool())
	_ = r.Register(NewGitStatusTool())
	_ = r.Register(NewGitLogTool())
	registerExtendedTools(r, ".")
	return r
}

// DefaultRegistryWithWorkspace returns a registry with all tools bound to workspace.
func DefaultRegistryWithWorkspace(workspace string) (*Registry, error) {
	ws, err := resolveWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	r, _ := NewRegistryWithWorkspace(ws)
	_ = r.Register(NewReadFileTool(ws))
	_ = r.Register(NewWriteFileTool(ws))
	_ = r.Register(NewEditFileTool(ws))
	_ = r.Register(NewListDirectoryTool(ws))
	_ = r.Register(NewSearchFilesTool(ws))
	_ = r.Register(NewRunCommandTool(ws))
	_ = r.Register(NewGitDiffTool(ws))
	_ = r.Register(NewGitStatusTool(ws))
	_ = r.Register(NewGitLogTool(ws))
	registerExtendedTools(r, ws)
	return r, nil
}

// registerExtendedTools registers the spec-named tools (create_file,
// delete_file, apply_patch, project_info) and validation tools
// (run_tests, run_build, run_lint). Registration errors are ignored for the
// same reason as above: these constructors always produce valid tools.
func registerExtendedTools(r *Registry, ws string) {
	_ = r.Register(NewCreateFileTool(ws))
	_ = r.Register(NewDeleteFileTool(ws))
	_ = r.Register(NewApplyPatchTool(ws))
	_ = r.Register(NewProjectInfoTool(ws))
	for _, t := range newValidationTools(ws) {
		_ = r.Register(t)
	}
}

// DefinitionsForPrompt returns tool definitions formatted for inclusion in model prompts.
func (r *Registry) DefinitionsForPrompt() string {
	tools := r.List()
	if len(tools) == 0 {
		return "No tools available."
	}
	var b strings.Builder
	b.WriteString("Available tools:\n")
	for _, t := range tools {
		schema := t.InputSchema()
		props := ""
		if len(schema.Properties) > 0 {
			var parts []string
			for k, p := range schema.Properties {
				req := ""
				for _, r := range schema.Required {
					if r == k {
						req = " (required)"
						break
					}
				}
				parts = append(parts, fmt.Sprintf("%s: %s%s", k, p.Description, req))
			}
			sort.Strings(parts)
			props = " Input: {" + strings.Join(parts, ", ") + "}"
		}
		fmt.Fprintf(&b, "- %s: %s%s\n", t.Name(), t.Description(), props)
	}
	b.WriteString("\nTo invoke a tool, respond with JSON: {\"tool\":\"<name>\",\"input\":{...}}\n")
	b.WriteString("To provide a final answer, respond with the answer text without JSON tool call.\n")
	return b.String()
}

// ---- helpers ----

// resolveWorkspace resolves workspace to an absolute directory, or returns error.
func resolveWorkspace(workspace string) (string, error) {
	if strings.TrimSpace(workspace) == "" {
		workspace = "."
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", NewToolError(CodeInvalidInput, fmt.Sprintf("invalid workspace %q", workspace), err)
	}
	abs = filepath.Clean(abs)
	fi, err := os.Stat(abs)
	if err != nil {
		return "", NewToolError(CodeNotFound, fmt.Sprintf("workspace %q not found", abs), err)
	}
	if !fi.IsDir() {
		return "", NewToolError(CodeInvalidInput, fmt.Sprintf("workspace %q is not a directory", abs), nil)
	}
	return abs, nil
}

// validatePath ensures p is within workspace, preventing traversal.
// If workspace is empty, it uses current directory as workspace.
func validatePath(workspace, p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", NewToolError(CodeInvalidInput, "path is required", nil)
	}
	if strings.Contains(p, "\x00") {
		return "", NewToolError(CodeInvalidInput, "path contains null byte", nil)
	}
	// Resolve workspace
	ws := workspace
	if strings.TrimSpace(ws) == "" {
		var err error
		ws, err = filepath.Abs(".")
		if err != nil {
			return "", NewToolError(CodeExecutionFailed, "cannot resolve workspace", err)
		}
	} else {
		var err error
		ws, err = filepath.Abs(ws)
		if err != nil {
			return "", NewToolError(CodeInvalidInput, "invalid workspace", err)
		}
	}
	ws = filepath.Clean(ws)

	cleaned := filepath.Clean(p)
	var abs string
	if filepath.IsAbs(cleaned) {
		abs = filepath.Clean(cleaned)
	} else {
		abs = filepath.Join(ws, cleaned)
		abs = filepath.Clean(abs)
	}

	// On Windows, volume check: if workspace is C:\a and abs is D:\b, it's outside.
	if filepath.VolumeName(ws) != filepath.VolumeName(abs) {
		return "", NewToolError(CodePathTraversal, fmt.Sprintf("path %q escapes workspace %q (different volume)", p, ws), nil)
	}

	rel, err := filepath.Rel(ws, abs)
	if err != nil {
		return "", NewToolError(CodeOutsideWorkspace, fmt.Sprintf("path %q outside workspace %q", p, ws), err)
	}
	// If rel is absolute, it's outside (different volume case handled above, but also check)
	if filepath.IsAbs(rel) {
		return "", NewToolError(CodePathTraversal, fmt.Sprintf("path %q escapes workspace", p), nil)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "..\\") {
		return "", NewToolError(CodePathTraversal, fmt.Sprintf("path %q escapes workspace %q", p, ws), nil)
	}
	// Also reject Windows-style traversal explicitly on Unix (e.g. "..\\secret.txt" on Linux)
	// to prevent bypass when an attacker sends Windows separators to a Unix server.
	if strings.Contains(p, "..\\") || strings.Contains(p, "../") {
		// If p contains parent traversal with either separator and rel escapes, already handled.
		// This extra check ensures that a path like "..\\secret.txt" is treated as traversal
		// even when filepath.Clean on Unix does not split on backslash.
		// We verify by checking if cleaned path would escape if backslashes were separators.
		normalized := strings.ReplaceAll(p, "\\", "/")
		normCleaned := filepath.Clean(normalized)
		var normAbs string
		if filepath.IsAbs(normCleaned) {
			normAbs = filepath.Clean(normCleaned)
		} else {
			normAbs = filepath.Join(ws, normCleaned)
			normAbs = filepath.Clean(normAbs)
		}
		if normRel, err := filepath.Rel(ws, normAbs); err == nil {
			if normRel == ".." || strings.HasPrefix(normRel, "../") || strings.HasPrefix(normRel, "..\\") {
				return "", NewToolError(CodePathTraversal, fmt.Sprintf("path %q escapes workspace %q", p, ws), nil)
			}
		}
	}
	// Also block traversal that still contains ".." after clean? Clean already removed, but check original for suspicious.
	if strings.Contains(p, "..") {
		// Already handled via rel, but extra check for normalized case
		_ = rel
	}
	// Symlink hardening: ensure the real path (after resolving symlinks in
	// existing components) stays inside the workspace.
	if err := hardenPathAgainstSymlinks(ws, abs); err != nil {
		return "", err
	}
	return abs, nil
}

// checkContext returns a structured cancellation error if ctx is done.
func checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return NewToolError(CodeCancelled, "operation cancelled", ctx.Err())
	default:
		return nil
	}
}

// truncateString limits string to limit bytes, returning truncated flag.
func truncateString(s string, limit int) (string, bool) {
	if len(s) <= limit {
		return s, false
	}
	return s[:limit], true
}

// limitOutput checks if output exceeds MaxOutputBytes and truncates.
func limitOutput(s string) (string, bool) {
	if len(s) > MaxOutputBytes {
		return s[:MaxOutputBytes] + "\n...[output truncated, limit 1 MiB]", true
	}
	return s, false
}

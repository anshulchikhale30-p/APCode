package tools

// Spec-compliant tool aliases for REPL and agent.
// These are thin wrappers around the existing tools with names matching the spec:
//   read_file, search, write_file, shell, git_diff, git_status

// ReadFileSpec is an alias for ReadFileTool with spec name "read_file".
type ReadFileSpec struct {
	*ReadFileTool
}

func NewReadFileSpec(workspace ...string) *ReadFileSpec {
	return &ReadFileSpec{ReadFileTool: NewReadFileTool(workspace...)}
}
func (t *ReadFileSpec) Name() string { return "read_file" }

// SearchSpec is an alias for SearchFilesTool with spec name "search".
type SearchSpec struct {
	*SearchFilesTool
}

func NewSearchSpec(workspace ...string) *SearchSpec {
	return &SearchSpec{SearchFilesTool: NewSearchFilesTool(workspace...)}
}
func (t *SearchSpec) Name() string { return "search" }

// WriteFileSpec is an alias for WriteFileTool with spec name "write_file".
type WriteFileSpec struct {
	*WriteFileTool
}

func NewWriteFileSpec(workspace ...string) *WriteFileSpec {
	return &WriteFileSpec{WriteFileTool: NewWriteFileTool(workspace...)}
}
func (t *WriteFileSpec) Name() string { return "write_file" }

// ShellSpec is an alias for RunCommandTool with spec name "shell".
type ShellSpec struct {
	*RunCommandTool
}

func NewShellSpec(workspace ...string) *ShellSpec {
	return &ShellSpec{RunCommandTool: NewRunCommandTool(workspace...)}
}
func (t *ShellSpec) Name() string { return "shell" }
func (t *ShellSpec) Description() string {
	return "Execute a shell command within the workspace. Input: {\"command\": \"<cmd>\", \"args\": \"<args>\", \"dir\": \"<optional dir>\"} Requires approval for destructive commands."
}
func (t *ShellSpec) InputSchema() Schema {
	// Keep same as RunCommand but with shell-centric description
	return t.RunCommandTool.InputSchema()
}

// GitDiffSpec is an alias for GitDiffTool with spec name "git_diff".
type GitDiffSpec struct {
	*GitDiffTool
}

func NewGitDiffSpec(workspace ...string) *GitDiffSpec {
	return &GitDiffSpec{GitDiffTool: NewGitDiffTool(workspace...)}
}
func (t *GitDiffSpec) Name() string { return "git_diff" }

// GitStatusSpec is an alias for GitStatusTool with spec name "git_status".
type GitStatusSpec struct {
	*GitStatusTool
}

func NewGitStatusSpec(workspace ...string) *GitStatusSpec {
	return &GitStatusSpec{GitStatusTool: NewGitStatusTool(workspace...)}
}
func (t *GitStatusSpec) Name() string { return "git_status" }

// RegisterSpecTools registers all spec-compliant tools into the given registry.
func RegisterSpecTools(r *Registry, workspace string) {
	_ = r.Register(NewReadFileSpec(workspace))
	_ = r.Register(NewSearchSpec(workspace))
	_ = r.Register(NewWriteFileSpec(workspace))
	_ = r.Register(NewShellSpec(workspace))
	_ = r.Register(NewGitDiffSpec(workspace))
	_ = r.Register(NewGitStatusSpec(workspace))
	// Also register original tools for backward compat (if not already)
	// They are already registered via DefaultRegistry, but ensure spec names are primary
}

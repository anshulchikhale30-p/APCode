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
// EditFileSpec is an alias for EditFileTool with spec name "edit_file".
type EditFileSpec struct {
	*EditFileTool
}

func NewEditFileSpec(workspace ...string) *EditFileSpec {
	return &EditFileSpec{EditFileTool: NewEditFileTool(workspace...)}
}
func (t *EditFileSpec) Name() string { return "edit_file" }

// GitLogSpec is an alias for GitLogTool with spec name "git_log".
type GitLogSpec struct {
	*GitLogTool
}

func NewGitLogSpec(workspace ...string) *GitLogSpec {
	return &GitLogSpec{GitLogTool: NewGitLogTool(workspace...)}
}
func (t *GitLogSpec) Name() string { return "git_log" }

// ListFilesSpec is an alias for ListDirectoryTool with spec name "list_files".
type ListFilesSpec struct {
	*ListDirectoryTool
}

func NewListFilesSpec(workspace ...string) *ListFilesSpec {
	return &ListFilesSpec{ListDirectoryTool: NewListDirectoryTool(workspace...)}
}
func (t *ListFilesSpec) Name() string { return "list_files" }

// RegisterSpecTools registers the canonical snake_case tool names, replacing
// any legacy CamelCase spellings of the same capability. After this call the
// registry exposes exactly one identifier per capability, and that is what
// the model is told about.
func RegisterSpecTools(r *Registry, workspace string) {
	_ = r.ReplaceWithLegacy(NewReadFileSpec(workspace), "ReadFile")
	_ = r.ReplaceWithLegacy(NewSearchSpec(workspace), "SearchFiles")
	_ = r.ReplaceWithLegacy(NewWriteFileSpec(workspace), "WriteFile")
	_ = r.ReplaceWithLegacy(NewEditFileSpec(workspace), "EditFile")
	_ = r.ReplaceWithLegacy(NewShellSpec(workspace), "RunCommand")
	_ = r.ReplaceWithLegacy(NewGitDiffSpec(workspace), "GitDiff")
	_ = r.ReplaceWithLegacy(NewGitStatusSpec(workspace), "GitStatus")
	_ = r.ReplaceWithLegacy(NewGitLogSpec(workspace), "GitLog")
	_ = r.ReplaceWithLegacy(NewListFilesSpec(workspace), "ListDirectory", "list_files")
}

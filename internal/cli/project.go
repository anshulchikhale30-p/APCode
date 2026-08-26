// Package cli - project context detection for REPL
package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	projectcontext "apcode/internal/context"
)

// ProjectContext is a structured view of the current project for the REPL and agent.
type ProjectContext struct {
	Root           string                 `json:"root"`
	IsGitRepo      bool                   `json:"is_git_repo"`
	Language       string                 `json:"language"`
	Files          []string               `json:"files"`
	ImportantFiles []string               `json:"important_files"`
	GitBranch      string                 `json:"git_branch"`
	TotalFiles     int                    `json:"total_files"`
	Languages      map[string]int         `json:"languages"`
	RawResult      *projectcontext.Result `json:"-"`
}

// DetectProjectContext builds a ProjectContext for the given workspace.
// It reuses existing projectcontext.WalkProject and DetectProjectRoot where possible.
func DetectProjectContext(workspace string) (*ProjectContext, error) {
	if strings.TrimSpace(workspace) == "" {
		workspace = "."
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}

	// Detect project root
	root, err := projectcontext.DetectProjectRoot(abs)
	if err != nil {
		// Fallback to workspace itself
		root = abs
	}

	// Walk project to get files/languages
	cfg := projectcontext.DefaultConfig()
	cfg.Root = root
	result, err := projectcontext.WalkProject(root, cfg)
	if err != nil {
		// Return minimal context even if walk fails
		result = &projectcontext.Result{
			Root:  root,
			Files: []projectcontext.FileMeta{},
		}
	}

	// Detect git
	isGit := false
	branch := ""
	// Check root and parents for .git
	checkDir := root
	for {
		if _, err := os.Stat(filepath.Join(checkDir, ".git")); err == nil {
			isGit = true
			// Try to read branch
			if data, err := os.ReadFile(filepath.Join(checkDir, ".git", "HEAD")); err == nil {
				s := strings.TrimSpace(string(data))
				if strings.HasPrefix(s, "ref: refs/heads/") {
					branch = strings.TrimPrefix(s, "ref: refs/heads/")
				} else if len(s) >= 7 {
					branch = s[:7]
				}
			}
			break
		}
		parent := filepath.Dir(checkDir)
		if parent == checkDir {
			break
		}
		checkDir = parent
		// Also check abs workspace for .git
		if checkDir == filepath.Dir(abs) && abs != root {
			// Also check abs
			if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
				isGit = true
				break
			}
		}
		if len(checkDir) < len(filepath.VolumeName(checkDir))+3 {
			// Stop at drive root on Windows
			break
		}
	}

	// Determine primary language
	lang := "unknown"
	maxCount := 0
	for l, c := range result.Languages {
		if c > maxCount {
			maxCount = c
			lang = l
		}
	}
	// If no languages detected, try to infer from important files
	if lang == "unknown" {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			lang = "Go"
		} else if _, err := os.Stat(filepath.Join(root, "package.json")); err == nil {
			lang = "JavaScript"
		} else if _, err := os.Stat(filepath.Join(root, "Cargo.toml")); err == nil {
			lang = "Rust"
		} else if _, err := os.Stat(filepath.Join(root, "pyproject.toml")); err == nil {
			lang = "Python"
		}
	}

	// Collect file paths (relative)
	files := make([]string, 0, len(result.Files))
	for _, f := range result.Files {
		files = append(files, f.Path)
	}
	sort.Strings(files)

	// Important files: check for common config files
	importantCandidates := []string{
		"README.md", "README", "go.mod", "go.sum", "package.json", "pyproject.toml",
		"Cargo.toml", "pom.xml", "build.gradle", "Makefile", ".gitignore",
		"apcode.json", ".apcode/config.json", "install.sh", "install.ps1",
	}
	var important []string
	for _, name := range importantCandidates {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			important = append(important, name)
		}
	}
	// Also add any file that is in root with config-like name
	// Already covered

	return &ProjectContext{
		Root:           root,
		IsGitRepo:      isGit,
		Language:       lang,
		Files:          files,
		ImportantFiles: important,
		GitBranch:      branch,
		TotalFiles:     len(files),
		Languages:      result.Languages,
		RawResult:      result,
	}, nil
}

// Summary returns a human-readable summary for display.
func (p *ProjectContext) Summary() string {
	if p == nil {
		return "No project"
	}
	var b strings.Builder
	b.WriteString("Root: " + p.Root + "\n")
	b.WriteString("Language: " + p.Language + "\n")
	b.WriteString("Files: " + strings.Join(p.Files[:minInt(5, len(p.Files))], ", "))
	if len(p.Files) > 5 {
		b.WriteString(" ...")
	}
	b.WriteString("\n")
	if len(p.ImportantFiles) > 0 {
		b.WriteString("Important: " + strings.Join(p.ImportantFiles, ", ") + "\n")
	}
	if p.IsGitRepo {
		b.WriteString("Git: yes")
		if p.GitBranch != "" {
			b.WriteString(" (" + p.GitBranch + ")")
		}
		b.WriteString("\n")
	} else {
		b.WriteString("Git: no\n")
	}
	return b.String()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

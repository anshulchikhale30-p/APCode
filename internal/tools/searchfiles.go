package tools

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SearchFilesTool searches for text within workspace files.
type SearchFilesTool struct {
	workspace string
}

func NewSearchFilesTool(workspace ...string) *SearchFilesTool {
	ws := "."
	if len(workspace) > 0 && strings.TrimSpace(workspace[0]) != "" {
		ws = workspace[0]
	}
	if abs, err := filepath.Abs(ws); err == nil {
		ws = abs
	}
	return &SearchFilesTool{workspace: ws}
}

func (t *SearchFilesTool) Name() string { return "SearchFiles" }
func (t *SearchFilesTool) Description() string {
	return "Search for text within workspace files. Input: {\"query\": \"<search term>\", \"path\": \"<optional subdir>\"}"
}
func (t *SearchFilesTool) InputSchema() Schema {
	return Schema{
		Type: "object",
		Properties: map[string]Property{
			"query":       {Type: "string", Description: "Text to search for (case-insensitive)"},
			"pattern":     {Type: "string", Description: "Alias for query (pattern)"},
			"path":        {Type: "string", Description: "Subdirectory within workspace to search (optional)"},
			"max_results": {Type: "string", Description: "Maximum results to return (optional)"},
		},
		Required: []string{"query"},
	}
}

func (t *SearchFilesTool) Execute(ctx context.Context, in Input) (Result, error) {
	if err := checkContext(ctx); err != nil {
		return Result{}, err
	}
	query := strings.TrimSpace(in["query"])
	if query == "" {
		query = strings.TrimSpace(in["pattern"])
	}
	if query == "" {
		// Also accept case-insensitive keys and common aliases like "Pattern", "q"
		for k, v := range in {
			lk := strings.ToLower(strings.TrimSpace(k))
			if lk == "query" || lk == "pattern" || lk == "q" || lk == "search" || lk == "text" {
				if strings.TrimSpace(v) != "" {
					query = strings.TrimSpace(v)
					break
				}
			}
		}
	}
	if query == "" {
		return Result{}, NewToolError(CodeInvalidInput, "SearchFiles: missing required argument 'query' (or 'pattern')", nil)
	}
	subPath := strings.TrimSpace(in["path"])
	maxResults := MaxSearchResults
	if v := strings.TrimSpace(in["max_results"]); v != "" {
		var m int
		if _, err := fmt.Sscanf(v, "%d", &m); err == nil && m > 0 {
			if m > MaxSearchResults {
				m = MaxSearchResults
			}
			maxResults = m
		}
	}
	// Also accept maxResults without underscore?
	if maxResults == MaxSearchResults {
		if v := strings.TrimSpace(in["maxResults"]); v != "" {
			var m int
			if _, err := fmt.Sscanf(v, "%d", &m); err == nil && m > 0 {
				if m > MaxSearchResults {
					m = MaxSearchResults
				}
				maxResults = m
			}
		}
	}
	searchRoot := t.workspace
	if subPath != "" {
		abs, err := validatePath(t.workspace, subPath)
		if err != nil {
			return Result{}, err
		}
		fi, err := os.Stat(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return Result{Output: "", Err: NewToolError(CodeNotFound, fmt.Sprintf("path %q not found", subPath), err)}, nil
			}
			return Result{Output: "", Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("cannot stat %q", subPath), err)}, nil
		}
		if !fi.IsDir() {
			// If it's a file, just search that file
			searchRoot = filepath.Dir(abs)
			// We'll handle file case separately by checking single file
		} else {
			searchRoot = abs
		}
	}
	type searchRes struct {
		results []string
		err     error
	}
	ch := make(chan searchRes, 1)
	go func() {
		var hits []string
		qLower := strings.ToLower(query)
		count := 0
		err := filepath.WalkDir(searchRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if strings.HasPrefix(name, ".") && name != "." {
					return filepath.SkipDir
				}
				switch name {
				case "node_modules", "vendor", ".git", "__pycache__", "target", "dist", "build":
					return filepath.SkipDir
				}
				return nil
			}
			select {
			case <-ctx.Done():
				return fmt.Errorf("cancelled")
			default:
			}
			if count >= maxResults {
				return filepath.SkipAll
			}
			// Quick file size check
			fi, err := d.Info()
			if err != nil {
				return nil
			}
			if fi.Size() > MaxFileBytes {
				return nil
			}
			// Skip binary by extension
			ext := strings.ToLower(filepath.Ext(path))
			switch ext {
			case ".png", ".jpg", ".jpeg", ".gif", ".ico", ".pdf", ".zip", ".tar", ".gz", ".exe", ".bin", ".so", ".dylib", ".dll", ".o", ".a", ".class", ".pyc":
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			if bytes.Contains(data[:min(len(data), 8000)], []byte{0}) {
				return nil
			}
			lines := bytes.Split(data, []byte{'\n'})
			rel, _ := filepath.Rel(searchRoot, path)
			rel = filepath.ToSlash(rel)
			if subPath != "" && !d.IsDir() {
				rel = subPath
			}
			for i, line := range lines {
				if strings.Contains(strings.ToLower(string(line)), qLower) {
					preview := strings.TrimSpace(string(line))
					if len(preview) > 120 {
						preview = preview[:120]
					}
					hits = append(hits, fmt.Sprintf("%s:%d:%s", rel, i+1, preview))
					count++
					if count >= maxResults {
						return filepath.SkipAll
					}
				}
			}
			return nil
		})
		ch <- searchRes{hits, err}
	}()
	select {
	case <-ctx.Done():
		return Result{}, NewToolError(CodeCancelled, "SearchFiles cancelled", ctx.Err())
	case res := <-ch:
		if res.err != nil && res.err.Error() == "cancelled" {
			return Result{}, NewToolError(CodeCancelled, "SearchFiles cancelled", ctx.Err())
		}
		if res.err != nil {
			return Result{Output: "", Err: NewToolError(CodeExecutionFailed, fmt.Sprintf("search failed: %v", res.err), res.err)}, nil
		}
		truncated := len(res.results) >= maxResults
		output := strings.Join(res.results, "\n")
		if truncated {
			output += fmt.Sprintf("\n...[truncated, showing %d results, limit %d]", len(res.results), maxResults)
		}
		if len(output) > MaxOutputBytes {
			output, _ = limitOutput(output)
			truncated = true
		}
		meta := map[string]interface{}{"query": query, "count": len(res.results), "truncated": truncated}
		return Result{Output: output, Truncated: truncated, Metadata: meta}, nil
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

package codeintel

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// FileInfo describes an indexed file.
type FileInfo struct {
	Path     string   `json:"path"`
	Language Language `json:"language"`
	Size     int64    `json:"size"`
	Lines    int      `json:"lines"`
}

// FileRelationship describes an import/dependency edge between files.
type FileRelationship struct {
	From       string `json:"from"`        // importer file
	To         string `json:"to"`          // imported file (resolved local path) or raw import path if external
	ImportPath string `json:"import_path"` // raw import string
	Resolved   bool   `json:"resolved"`    // true if To is a local file
}

// Index holds the full code intelligence data for a directory.
type Index struct {
	mu            sync.RWMutex
	Root          string             `json:"root"`
	Files         []FileInfo         `json:"files"`
	Symbols       []Symbol           `json:"symbols"`
	Imports       []ImportInfo       `json:"imports"`
	Relationships []FileRelationship `json:"relationships"`

	// internal: map path -> content for search/references
	contents map[string][]byte
	// map import name -> file path for resolution
	symbolIndex map[string][]Symbol
	// set of existing relative paths for resolution
	fileSet map[string]bool
}

// NewIndex creates an empty index.
func NewIndex(root string) *Index {
	return &Index{
		Root:        root,
		contents:    make(map[string][]byte),
		symbolIndex: make(map[string][]Symbol),
		fileSet:     make(map[string]bool),
	}
}

// Build scans dir and populates the index.
// It is lightweight and offline, using only stdlib.
func (idx *Index) Build(dir string) error {
	absRoot, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("codeintel: invalid dir %q: %w", dir, err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return fmt.Errorf("codeintel: cannot stat %q: %w", absRoot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("codeintel: not a directory: %q", absRoot)
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	idx.Root = absRoot
	idx.Files = nil
	idx.Symbols = nil
	idx.Imports = nil
	idx.Relationships = nil
	idx.contents = make(map[string][]byte)
	idx.symbolIndex = make(map[string][]Symbol)
	idx.fileSet = make(map[string]bool)

	// Collect files first
	var paths []string
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		// Skip hidden and vendor dirs
		rel, _ := filepath.Rel(absRoot, path)
		if rel == "." {
			return nil
		}
		name := d.Name()
		// Skip dot dirs and common ignore
		if d.IsDir() {
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			switch name {
			case "node_modules", "vendor", "__pycache__", "target", "dist", "build", ".git", ".hg", ".svn", "out", "bin", "obj":
				return filepath.SkipDir
			}
			return nil
		}
		// Skip binary / large files? Check ext
		if isIgnoredFile(name) {
			return nil
		}
		// Limit size 2 MiB for practicality
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		if fi.Size() > 2*1024*1024 {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(paths)

	for _, p := range paths {
		content, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if isBinary(content) {
			continue
		}
		lang := DetectLanguage(p)
		// Refine via shebang if unknown
		if lang == LanguageUnknown {
			lang = DetectLanguageByContent(p, content)
		}
		rel, _ := filepath.Rel(absRoot, p)
		rel = filepath.ToSlash(rel)
		idx.fileSet[rel] = true
		// Also store absolute for resolution attempt
		idx.fileSet[p] = true

		lines := bytes.Count(content, []byte{'\n'}) + 1
		if len(content) > 0 && content[len(content)-1] == '\n' {
			lines--
		}
		fi := FileInfo{Path: rel, Language: lang, Size: int64(len(content)), Lines: lines}
		idx.Files = append(idx.Files, fi)
		idx.contents[rel] = content
		// Also map with abs?
		idx.contents[p] = content

		syms := DiscoverSymbols(rel, lang, content)
		for _, s := range syms {
			idx.Symbols = append(idx.Symbols, s)
			key := strings.ToLower(s.Name)
			idx.symbolIndex[key] = append(idx.symbolIndex[key], s)
		}
		imps := DiscoverImports(rel, lang, content)
		idx.Imports = append(idx.Imports, imps...)
	}
	// Build relationships: try to resolve imports to local files
	idx.Relationships = buildRelationships(idx.Files, idx.Imports, idx.fileSet, absRoot)
	// Sort for determinism
	sort.Slice(idx.Files, func(i, j int) bool { return idx.Files[i].Path < idx.Files[j].Path })
	sort.Slice(idx.Symbols, func(i, j int) bool {
		if idx.Symbols[i].Path != idx.Symbols[j].Path {
			return idx.Symbols[i].Path < idx.Symbols[j].Path
		}
		if idx.Symbols[i].Line != idx.Symbols[j].Line {
			return idx.Symbols[i].Line < idx.Symbols[j].Line
		}
		return idx.Symbols[i].Name < idx.Symbols[j].Name
	})
	sort.Slice(idx.Imports, func(i, j int) bool {
		if idx.Imports[i].Path != idx.Imports[j].Path {
			return idx.Imports[i].Path < idx.Imports[j].Path
		}
		return idx.Imports[i].ImportPath < idx.Imports[j].ImportPath
	})
	sort.Slice(idx.Relationships, func(i, j int) bool {
		if idx.Relationships[i].From != idx.Relationships[j].From {
			return idx.Relationships[i].From < idx.Relationships[j].From
		}
		return idx.Relationships[i].To < idx.Relationships[j].To
	})
	return nil
}

func isIgnoredFile(name string) bool {
	lower := strings.ToLower(name)
	// Images, binaries, etc.
	ignoredExts := []string{".png", ".jpg", ".jpeg", ".gif", ".ico", ".pdf", ".zip", ".tar", ".gz", ".exe", ".bin", ".so", ".dylib", ".dll", ".o", ".a", ".class", ".pyc", ".pyo", ".lock"}
	for _, e := range ignoredExts {
		if strings.HasSuffix(lower, e) {
			return true
		}
	}
	if strings.HasSuffix(lower, ".min.js") {
		return true
	}
	return false
}

func isBinary(content []byte) bool {
	// Heuristic: contains NUL byte in first 8000 bytes
	limit := len(content)
	if limit > 8000 {
		limit = 8000
	}
	for i := 0; i < limit; i++ {
		if content[i] == 0 {
			return true
		}
	}
	return false
}

func buildRelationships(files []FileInfo, imports []ImportInfo, fileSet map[string]bool, root string) []FileRelationship {
	var rels []FileRelationship
	// Build helper: map base name without ext -> path?
	baseMap := make(map[string][]string)
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f.Path), filepath.Ext(f.Path))
		baseMap[strings.ToLower(base)] = append(baseMap[strings.ToLower(base)], f.Path)
		// Also dir+base
	}
	for _, imp := range imports {
		to := imp.ImportPath
		resolved := false
		resolvedPath := to

		// Try to resolve: check if import path directly maps to a file
		// For Go: import path is not file path; we approximate by base name match
		// For Python/JS: import string may be file path
		candidates := []string{}
		// If import contains '/', try last segment
		base := filepath.Base(to)
		// Remove package prefix like github.com/.../pkg
		if idx := strings.LastIndex(to, "/"); idx >= 0 {
			base = to[idx+1:]
		}
		// Handle go imports with quotes already stripped
		// For relative imports like "./utils" or "../lib"
		cleanImp := strings.Trim(to, `"'`)
		cleanImp = filepath.ToSlash(cleanImp)
		// Try direct file match
		if fileSet[cleanImp] {
			resolved = true
			resolvedPath = cleanImp
		} else if fileSet[cleanImp+".go"] {
			resolved = true
			resolvedPath = cleanImp + ".go"
		} else if fileSet[cleanImp+".py"] {
			resolved = true
			resolvedPath = cleanImp + ".py"
		} else if fileSet[cleanImp+".js"] {
			resolved = true
			resolvedPath = cleanImp + ".js"
		} else if fileSet[cleanImp+".ts"] {
			resolved = true
			resolvedPath = cleanImp + ".ts"
		} else {
			// Try base map lookup
			if list, ok := baseMap[strings.ToLower(base)]; ok && len(list) > 0 {
				// Prefer file in same dir as importer? For now first
				// Check if importer dir close: we could rank by common prefix
				best := list[0]
				bestScore := -1
				for _, cand := range list {
					score := commonPrefixLen(filepath.Dir(imp.Path), filepath.Dir(cand))
					if score > bestScore {
						bestScore = score
						best = cand
					}
				}
				// Only resolve if plausible (e.g., Go package name equals base)
				// For determinism, we consider resolved if exactly one candidate or shared dir
				if len(list) == 1 || bestScore > 0 {
					resolved = true
					resolvedPath = best
				} else {
					// For ambiguous, keep first but mark resolved true to indicate local
					resolved = true
					resolvedPath = best
				}
			}
			// Check for fileSet with extension variations
			for _, ext := range []string{".go", ".py", ".js", ".ts", ".java", ".cpp", ".c", ".rs", ".rb"} {
				key := strings.ToLower(base + ext)
				for _, f := range files {
					if strings.ToLower(filepath.Base(f.Path)) == key {
						resolved = true
						resolvedPath = f.Path
						break
					}
				}
				if resolved {
					break
				}
			}
		}
		_ = candidates
		rels = append(rels, FileRelationship{From: imp.Path, To: resolvedPath, ImportPath: to, Resolved: resolved})
	}
	// Dedupe
	seen := make(map[string]bool)
	var uniq []FileRelationship
	for _, r := range rels {
		k := r.From + "|" + r.To + "|" + r.ImportPath
		if seen[k] {
			continue
		}
		seen[k] = true
		uniq = append(uniq, r)
	}
	return uniq
}

func commonPrefixLen(a, b string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

// Snapshot returns shallow copies for external use.
func (idx *Index) SnapshotFiles() []FileInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make([]FileInfo, len(idx.Files))
	copy(out, idx.Files)
	return out
}

func (idx *Index) SnapshotSymbols() []Symbol {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make([]Symbol, len(idx.Symbols))
	copy(out, idx.Symbols)
	return out
}

func (idx *Index) SnapshotImports() []ImportInfo {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make([]ImportInfo, len(idx.Imports))
	copy(out, idx.Imports)
	return out
}

func (idx *Index) SnapshotRelationships() []FileRelationship {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	out := make([]FileRelationship, len(idx.Relationships))
	copy(out, idx.Relationships)
	return out
}

// Lookup finds symbols by exact (case-insensitive) name or substring.
// It honors the original Indexer.Lookup contract but extends with richer matching.
func (idx *Index) Lookup(query string) ([]Symbol, error) {
	if query == "" {
		return nil, fmt.Errorf("codeintel: query cannot be empty")
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	q := strings.ToLower(strings.TrimSpace(query))
	var exact []Symbol
	var partial []Symbol
	for _, s := range idx.Symbols {
		nameLower := strings.ToLower(s.Name)
		if nameLower == q {
			exact = append(exact, s)
		} else if strings.Contains(nameLower, q) {
			partial = append(partial, s)
		}
	}
	// Prefer exact
	if len(exact) > 0 {
		return exact, nil
	}
	return partial, nil
}

// References finds where a symbol is referenced (simple textual grep).
// Returns list of occurrences excluding the definition line.
func (idx *Index) References(symbolName string) ([]Reference, error) {
	if symbolName == "" {
		return nil, fmt.Errorf("codeintel: symbol cannot be empty")
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	q := symbolName
	var out []Reference
	// Build symbol lookup to get definitions for filtering
	defSet := make(map[string]bool) // path:line -> true for definition
	for _, s := range idx.Symbols {
		if strings.EqualFold(s.Name, q) {
			key := fmt.Sprintf("%s:%d", s.Path, s.Line)
			defSet[key] = true
		}
	}
	for relPath, content := range idx.contents {
		// Only consider files that are actual indexed files (rel paths)
		if strings.Contains(relPath, "/") || strings.Contains(relPath, ".") {
			// Skip absolute dup entries that contain ":" or lead to double counting
			// Our contents has both rel and abs; dedupe by ensuring rel lacks leading "/"
			if filepath.IsAbs(relPath) {
				continue
			}
		}
		lines := bytes.Split(content, []byte{'\n'})
		for i, line := range lines {
			lineno := i + 1
			key := fmt.Sprintf("%s:%d", relPath, lineno)
			if defSet[key] {
				continue // skip definition line itself (where practical)
			}
			// Find all word-bounded occurrences on this line
			lineStr := string(line)
			lowLine := strings.ToLower(lineStr)
			lowQ := strings.ToLower(q)
			searchFrom := 0
			for {
				relIdx := strings.Index(lowLine[searchFrom:], lowQ)
				if relIdx < 0 {
					break
				}
				actualIdx := searchFrom + relIdx
				beforeOk := actualIdx == 0 || !isAlphaNum(rune(lowLine[actualIdx-1]))
				afterIdx := actualIdx + len(lowQ)
				afterOk := afterIdx >= len(lowLine) || !isAlphaNum(rune(lowLine[afterIdx]))
				if beforeOk && afterOk {
					out = append(out, Reference{
						Symbol: q,
						Path:   relPath,
						Line:   lineno,
						Column: actualIdx + 1,
						Text:   strings.TrimSpace(lineStr),
					})
				}
				searchFrom = actualIdx + len(lowQ)
				if searchFrom >= len(lowLine) {
					break
				}
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}

func containsWord(line, word string) bool {
	// Very simple word boundary check: ensure word appears surrounded by non-alnum or start/end
	lowLine := strings.ToLower(line)
	lowWord := strings.ToLower(word)
	idx := strings.Index(lowLine, lowWord)
	for idx >= 0 {
		beforeOk := idx == 0 || !isAlphaNum(rune(lowLine[idx-1]))
		afterIdx := idx + len(lowWord)
		afterOk := afterIdx >= len(lowLine) || !isAlphaNum(rune(lowLine[afterIdx]))
		if beforeOk && afterOk {
			return true
		}
		next := strings.Index(lowLine[idx+len(lowWord):], lowWord)
		if next < 0 {
			break
		}
		idx = idx + len(lowWord) + next
	}
	return false
}

func isAlphaNum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}

// Reference is an occurrence of a symbol outside its definition.
type Reference struct {
	Symbol string `json:"symbol"`
	Path   string `json:"path"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
	Text   string `json:"text"`
}

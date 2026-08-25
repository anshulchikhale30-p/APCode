package context

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Default ignored directory names (case-sensitive matching on base name).
var defaultIgnoredDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"build":        true,
	"dist":         true,
	"target":       true,
	"out":          true,
	"bin":          true,
	"obj":          true,
	".next":        true,
	".nuxt":        true,
	"coverage":     true,
	"__pycache__":  true,
	".venv":        true,
	"venv":         true,
	".idea":        true,
	".vscode":      true,
}

// Default ignored file extensions (lowercase, including dot).
var defaultIgnoredExts = map[string]bool{
	".exe":   true,
	".dll":   true,
	".so":    true,
	".dylib": true,
	".bin":   true,
	".o":     true,
	".a":     true,
	".class": true,
	".pyc":   true,
	".pyo":   true,
	".wasm":  true,
	".png":   true,
	".jpg":   true,
	".jpeg":  true,
	".gif":   true,
	".bmp":   true,
	".ico":   true,
	".mp4":   true,
	".mp3":   true,
	".avi":   true,
	".mov":   true,
	".zip":   true,
	".tar":   true,
	".gz":    true,
	".rar":   true,
	".7z":    true,
	".pdf":   true,
	".lock":  true, // package-lock, etc are often large generated
}

// Generated file patterns by filename suffix or substring.
var generatedPatterns = []string{
	".pb.go",
	".gen.go",
	".generated.",
	"mock_",
	"zz_generated",
	"_string.go",
}

// Gitignore holds parsed .gitignore patterns for a single directory.
// This is a practical subset: blank lines, comments, negation (!), and simple glob.
// Directory patterns ending with / are treated as prefix matches.
type Gitignore struct {
	patterns []gitPattern
	baseDir  string
}

type gitPattern struct {
	raw     string
	negate  bool
	dirOnly bool
	isGlob  bool
	pattern string
}

// LoadGitignore loads .gitignore from dir if present and returns a Gitignore.
// If file does not exist, returns empty Gitignore (no patterns) and no error.
func LoadGitignore(dir string) (*Gitignore, error) {
	gi := &Gitignore{baseDir: dir}
	path := filepath.Join(dir, ".gitignore")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return gi, nil
		}
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		gi.AddPattern(line)
	}
	return gi, scanner.Err()
}

// AddPattern adds a raw .gitignore pattern.
func (g *Gitignore) AddPattern(raw string) {
	p := strings.TrimSpace(raw)
	if p == "" || strings.HasPrefix(p, "#") {
		return
	}
	negate := false
	if strings.HasPrefix(p, "!") {
		negate = true
		p = strings.TrimPrefix(p, "!")
		p = strings.TrimSpace(p)
	}
	dirOnly := strings.HasSuffix(p, "/")
	if dirOnly {
		p = strings.TrimSuffix(p, "/")
	}
	// Remove leading slash.
	p = strings.TrimPrefix(p, "/")
	isGlob := strings.ContainsAny(p, "*?[]")
	g.patterns = append(g.patterns, gitPattern{
		raw:     raw,
		negate:  negate,
		dirOnly: dirOnly,
		isGlob:  isGlob,
		pattern: p,
	})
}

// Matches reports whether relPath (slash-separated, relative to baseDir) matches gitignore.
// relPath should be relative with forward slashes.
func (g *Gitignore) Matches(relPath string, isDir bool) bool {
	rel := filepath.ToSlash(relPath)
	// Normalize: remove leading ./
	rel = strings.TrimPrefix(rel, "./")
	matched := false
	for _, pat := range g.patterns {
		m := matchGitPattern(pat, rel, isDir)
		if m {
			if pat.negate {
				matched = false
			} else {
				matched = true
			}
		}
	}
	return matched
}

func matchGitPattern(pat gitPattern, rel string, isDir bool) bool {
	if pat.dirOnly {
		// Match directory prefix: "build" should match "build/file.txt" and "build"
		if rel == pat.pattern || strings.HasPrefix(rel, pat.pattern+"/") {
			return true
		}
		// Also match any segment equal to pattern (e.g., "a/b/build/c")
		parts := strings.Split(rel, "/")
		for _, part := range parts {
			if pat.isGlob {
				if ok, _ := filepath.Match(pat.pattern, part); ok {
					return true
				}
			} else if part == pat.pattern {
				return true
			}
		}
		return false
	}
	if pat.isGlob {
		// If pattern contains slash, match full path; else match basename.
		if strings.Contains(pat.pattern, "/") {
			if ok, _ := filepath.Match(pat.pattern, rel); ok {
				return true
			}
			// Also try matching with ** handling simplified: treat ** as *
			// Fallback: check suffix/prefix
			return false
		}
		base := filepath.Base(rel)
		if ok, _ := filepath.Match(pat.pattern, base); ok {
			return true
		}
		if ok, _ := filepath.Match(pat.pattern, rel); ok {
			return true
		}
		return false
	}
	// Plain pattern: match basename or full rel or prefix for dirs.
	if rel == pat.pattern {
		return true
	}
	if strings.HasPrefix(rel, pat.pattern+"/") {
		return true
	}
	base := filepath.Base(rel)
	if base == pat.pattern {
		return true
	}
	// Match any path segment
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if part == pat.pattern {
			return true
		}
	}
	return false
}

// IsDefaultIgnoredDir reports whether base name should be ignored by default.
func IsDefaultIgnoredDir(name string) bool {
	return defaultIgnoredDirs[name]
}

// IsDefaultIgnoredExt reports whether extension should be ignored.
func IsDefaultIgnoredExt(ext string) bool {
	return defaultIgnoredExts[strings.ToLower(ext)]
}

// IsGeneratedFileName reports whether filename looks generated.
func IsGeneratedFileName(name string) bool {
	lower := strings.ToLower(name)
	for _, pat := range generatedPatterns {
		if strings.Contains(lower, strings.ToLower(pat)) {
			return true
		}
	}
	return false
}

// ShouldIgnore decides if a path should be ignored, returning reason if so.
// relPath is slash-separated relative to root.
// cfg holds custom ignore patterns.
func ShouldIgnore(relPath string, isDir bool, cfg Config, gi *Gitignore) (bool, string) {
	base := filepath.Base(relPath)
	ext := strings.ToLower(filepath.Ext(relPath))

	// 1. Default dirs
	if isDir && IsDefaultIgnoredDir(base) {
		return true, "default ignored dir: " + base
	}
	// Also ignore if any segment is default ignored dir (nested node_modules)
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	for _, part := range parts {
		if defaultIgnoredDirs[part] {
			return true, "default ignored dir segment: " + part
		}
	}

	// 2. Default exts (files only)
	if !isDir && IsDefaultIgnoredExt(ext) {
		return true, "binary/asset extension: " + ext
	}

	// 3. Generated
	if !isDir && IsGeneratedFileName(base) {
		return true, "generated file pattern: " + base
	}

	// 4. Hidden files/dirs (dotfiles) unless explicitly allowed?
	// We ignore hidden dirs except .git which already handled. Respect IncludeHidden.
	if !cfg.IncludeHidden && strings.HasPrefix(base, ".") {
		// Allow .gitignore itself, but generally ignore dotfiles
		if base != ".gitignore" && base != ".gitattributes" {
			// If isDir and it's hidden, ignore
			if isDir {
				return true, "hidden dir: " + base
			}
			// For files, ignore hidden files
			// But allow some source dotfiles? For now ignore hidden files.
			return true, "hidden file: " + base
		}
	}

	// 5. Custom ignore patterns (glob against relPath and basename)
	for _, pat := range cfg.IgnorePatterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		// Support slash-separated patterns.
		if ok, _ := filepath.Match(pat, filepath.ToSlash(relPath)); ok {
			return true, "custom pattern: " + pat
		}
		if ok, _ := filepath.Match(pat, base); ok {
			return true, "custom pattern: " + pat
		}
		// Simple substring for patterns without glob
		if !strings.ContainsAny(pat, "*?[]") && strings.Contains(relPath, pat) {
			return true, "custom pattern: " + pat
		}
	}

	// 6. .gitignore
	if cfg.RespectGitignore && gi != nil {
		if gi.Matches(relPath, isDir) {
			return true, ".gitignore: " + relPath
		}
	}

	return false, ""
}

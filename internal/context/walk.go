package context

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config controls project walking.
type Config struct {
	// Root is project root; if empty, DetectProjectRoot is used.
	Root string
	// IgnorePatterns are custom glob patterns (matched against relative path and base).
	IgnorePatterns []string
	// MaxFileSize is max file size to consider for content reading; larger files are counted but not fully read.
	// 0 means default 512 KiB.
	MaxFileSize int64
	// TokenBudget for selection (0 = no limit; still computed)
	TokenBudget int
	// MaxTotalFiles limits discovered files (0 = no limit / 10000 default to avoid blind loading)
	MaxTotalFiles int
	// RespectGitignore enables .gitignore handling (default true)
	RespectGitignore bool
	// IncludeHidden includes dotfiles (default false)
	IncludeHidden bool
	// FollowSymlinks follows symlinks (default false for safety)
	FollowSymlinks bool
}

// FileMeta holds metadata for a discovered file.
type FileMeta struct {
	Path        string // relative to root, slash-separated
	AbsPath     string
	Size        int64
	ModTime     time.Time
	Language    string
	Ext         string
	Lines       int
	Tokens      int
	IsBinary    bool
	IsGenerated bool
}

// IgnoredEntry records an ignored file/dir with reason.
type IgnoredEntry struct {
	Path   string
	Reason string
	IsDir  bool
}

// Result holds context discovery outcome.
type Result struct {
	Root           string
	Files          []FileMeta
	Ignored        []IgnoredEntry
	Languages      map[string]int // language -> count
	TotalSize      int64
	TotalTokens    int
	Truncated      bool       // true if MaxTotalFiles or budget truncated
	Selected       []FileMeta // subset fitting budget (if budget set)
	SelectedTokens int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxFileSize:      512 * 1024, // 512 KiB
		MaxTotalFiles:    10000,
		RespectGitignore: true,
		IncludeHidden:    false,
		FollowSymlinks:   false,
		TokenBudget:      0,
	}
}

// WalkProject discovers project files under root according to cfg.
// It respects ignore rules, .gitignore, and avoids loading entire repo blindly
// by limiting file reads and total files.
func WalkProject(root string, cfg Config) (*Result, error) {
	if root == "" {
		var err error
		root, err = DetectProjectRoot(".")
		if err != nil {
			return nil, err
		}
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(absRoot)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		absRoot = filepath.Dir(absRoot)
	}

	// Default RespectGitignore to true when Config is used as zero-value (common in tests).
	isEmpty := cfg.MaxFileSize == 0 && cfg.MaxTotalFiles == 0 && cfg.TokenBudget == 0 && len(cfg.IgnorePatterns) == 0 && cfg.Root == "" && !cfg.IncludeHidden && !cfg.FollowSymlinks && !cfg.RespectGitignore
	if isEmpty {
		cfg.RespectGitignore = true
	}
	if cfg.MaxFileSize == 0 {
		cfg.MaxFileSize = 512 * 1024
	}
	if cfg.MaxTotalFiles == 0 {
		cfg.MaxTotalFiles = 10000
	}

	gi, err := LoadGitignore(absRoot)
	if err != nil {
		// Non-fatal: continue without gitignore
		gi = &Gitignore{baseDir: absRoot}
	}

	// Also load nested .gitignore? For simplicity we only use root .gitignore practical subset.
	// For better practical respect, we could walk and load .gitignore per dir, but we keep simple.

	res := &Result{
		Root:      absRoot,
		Languages: make(map[string]int),
	}

	// Walk
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Record ignored due to error and continue
			rel, _ := filepath.Rel(absRoot, path)
			rel = filepath.ToSlash(rel)
			res.Ignored = append(res.Ignored, IgnoredEntry{Path: rel, Reason: "walk error: " + walkErr.Error(), IsDir: d != nil && d.IsDir()})
			return nil
		}

		// Skip root itself
		if path == absRoot {
			return nil
		}

		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)

		// Handle symlinks
		if d.Type()&os.ModeSymlink != 0 {
			if !cfg.FollowSymlinks {
				res.Ignored = append(res.Ignored, IgnoredEntry{Path: relSlash, Reason: "symlink skipped", IsDir: d.IsDir()})
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			// If following, resolve and check if dir
			target, err := os.Stat(path)
			if err != nil {
				res.Ignored = append(res.Ignored, IgnoredEntry{Path: relSlash, Reason: "broken symlink", IsDir: false})
				return nil
			}
			if target.IsDir() {
				// Avoid loops: check if target is inside root? For now skip if outside
				// Continue walking but prevent infinite loops by not following if already visited
				// Simple: skip symlinked dirs
				res.Ignored = append(res.Ignored, IgnoredEntry{Path: relSlash, Reason: "symlinked dir skipped", IsDir: true})
				return filepath.SkipDir
			}
		}

		isDir := d.IsDir()

		// Check ignore
		if ignore, reason := ShouldIgnore(relSlash, isDir, cfg, gi); ignore {
			res.Ignored = append(res.Ignored, IgnoredEntry{Path: relSlash, Reason: reason, IsDir: isDir})
			if isDir {
				return filepath.SkipDir
			}
			return nil
		}

		if isDir {
			return nil
		}

		// Limit total files to avoid blind loading
		if len(res.Files) >= cfg.MaxTotalFiles {
			res.Truncated = true
			res.Ignored = append(res.Ignored, IgnoredEntry{Path: relSlash, Reason: "max files limit", IsDir: false})
			return nil
		}

		// Only consider relevant source files
		if !IsRelevantSource(relSlash) {
			res.Ignored = append(res.Ignored, IgnoredEntry{Path: relSlash, Reason: "non-source file", IsDir: false})
			return nil
		}

		// Stat for metadata
		info, err := d.Info()
		if err != nil {
			res.Ignored = append(res.Ignored, IgnoredEntry{Path: relSlash, Reason: "stat error", IsDir: false})
			return nil
		}
		size := info.Size()
		// If file is huge, record but avoid reading
		if size > cfg.MaxFileSize*4 {
			// Very large: treat as ignored for context but still count as ignored
			res.Ignored = append(res.Ignored, IgnoredEntry{Path: relSlash, Reason: "file too large", IsDir: false})
			return nil
		}

		// Check binary via extension already done, but also peek content for null bytes and generated header
		isBinary := false
		isGenerated := IsGeneratedFileName(filepath.Base(relSlash))
		lines := 0
		// Only read sample for small files to avoid loading entire repo
		if size <= cfg.MaxFileSize && size > 0 {
			sampleSize := size
			if sampleSize > 8000 {
				sampleSize = 8000
			}
			f, err := os.Open(path)
			if err == nil {
				buf := make([]byte, sampleSize)
				n, _ := f.Read(buf)
				buf = buf[:n]
				f.Close()
				if IsBinaryContent(buf) {
					isBinary = true
				}
				if !isGenerated && IsGeneratedContent(buf) {
					isGenerated = true
				}
				// Count lines in sample + estimate total lines via size? For accuracy, count lines if small.
				if size <= cfg.MaxFileSize {
					// For small files, count all lines by reading fully but limited to MaxFileSize
					// We already have sample; for full lines, read whole file if <= MaxFileSize
					f2, err := os.Open(path)
					if err == nil {
						// Count lines
						b := make([]byte, size)
						n2, _ := f2.Read(b)
						f2.Close()
						for i := 0; i < n2; i++ {
							if b[i] == '\n' {
								lines++
							}
						}
						if n2 > 0 && b[n2-1] != '\n' {
							lines++
						}
					}
				}
			}
		}
		if isBinary {
			res.Ignored = append(res.Ignored, IgnoredEntry{Path: relSlash, Reason: "binary content", IsDir: false})
			return nil
		}
		if isGenerated {
			res.Ignored = append(res.Ignored, IgnoredEntry{Path: relSlash, Reason: "generated content", IsDir: false})
			return nil
		}

		ext := strings.ToLower(filepath.Ext(relSlash))
		lang := DetectLanguage(ext)
		tokens := EstimateTokens(size)

		meta := FileMeta{
			Path:        relSlash,
			AbsPath:     path,
			Size:        size,
			ModTime:     info.ModTime(),
			Language:    lang,
			Ext:         ext,
			Lines:       lines,
			Tokens:      tokens,
			IsBinary:    isBinary,
			IsGenerated: isGenerated,
		}
		res.Files = append(res.Files, meta)
		res.Languages[lang]++
		res.TotalSize += size
		res.TotalTokens += tokens

		return nil
	})

	if err != nil {
		return res, err
	}

	// Selection respecting budget
	if cfg.TokenBudget > 0 {
		selected, total := SelectContext(res.Files, SelectOptions{TokenBudget: cfg.TokenBudget})
		res.Selected = selected
		res.SelectedTokens = total
		// If selection truncated, mark
		if len(selected) < len(res.Files) {
			res.Truncated = true
		}
	} else {
		res.Selected = res.Files
		res.SelectedTokens = res.TotalTokens
	}

	return res, nil
}

package context

import (
	"os"
	"path/filepath"
)

// Project markers used to detect root. Ordered by priority.
var rootMarkers = []string{
	".git",
	"go.mod",
	"go.sum",
	"package.json",
	"Cargo.toml",
	"pyproject.toml",
	"setup.py",
	"Gemfile",
	"pom.xml",
	"build.gradle",
	"Makefile",
	".apcode",
}

// homeDir caches user home for marker filtering.
var homeDir string

func init() {
	if h, err := os.UserHomeDir(); err == nil {
		homeDir = h
	}
}

// DetectProjectRoot walks up from startPath until a marker is found.
// If no marker is found, it returns the absolute start path.
// startPath may be a file or directory; empty means current directory.
func DetectProjectRoot(startPath string) (string, error) {
	if startPath == "" {
		startPath = "."
	}
	abs, err := filepath.Abs(startPath)
	if err != nil {
		return "", err
	}
	// If startPath is a file, start from its directory.
	fi, err := os.Stat(abs)
	if err == nil && !fi.IsDir() {
		abs = filepath.Dir(abs)
	}
	// Walk up.
	for {
		for _, marker := range rootMarkers {
			// Skip .apcode at home directory (model storage) to avoid
			// false positives when walking from temp dirs under home.
			if marker == ".apcode" && homeDir != "" && abs == homeDir {
				continue
			}
			p := filepath.Join(abs, marker)
			if _, err := os.Stat(p); err == nil {
				return abs, nil
			}
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			// Reached filesystem root.
			// Return original absolute dir if no marker found.
			orig, _ := filepath.Abs(startPath)
			if fi != nil && !fi.IsDir() {
				orig = filepath.Dir(orig)
			}
			// If orig is empty or ".", ensure it's absolute.
			if orig == "." {
				orig, _ = os.Getwd()
			}
			return orig, nil
		}
		abs = parent
	}
}

// FindProjectRoot is an alias for DetectProjectRoot for compatibility.
func FindProjectRoot(start string) (string, error) {
	return DetectProjectRoot(start)
}

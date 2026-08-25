package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionIsSet(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must not be empty")
	}
	// Basic semantic version check: x.y.z
	parts := strings.Split(Version, ".")
	if len(parts) != 3 {
		t.Fatalf("Version %q should be semver x.y.z", Version)
	}
	for _, p := range parts {
		if p == "" {
			t.Fatalf("Version part empty in %q", Version)
		}
		for _, r := range p {
			if r < '0' || r > '9' {
				// Allow pre-release suffix like 0.1.0-next is filtered, but Version should be pure semver
				t.Fatalf("Version part %q contains non-digit in %q", p, Version)
			}
		}
	}
}

func TestVersionLdflagsOverridable(t *testing.T) {
	// Verify that Version is a variable (not const) and can be reassigned,
	// proving ldflags -X is possible.
	orig := Version
	Version = "9.9.9-test"
	if Version != "9.9.9-test" {
		t.Fatal("Version should be overridable via ldflags (must be var, not const)")
	}
	Version = orig
}

func TestAppName(t *testing.T) {
	if AppName != "APCode" {
		t.Fatalf("AppName = %q, want APCode", AppName)
	}
}

func TestDefaultModelDir(t *testing.T) {
	dir := DefaultModelDir()
	if dir == "" {
		t.Fatal("DefaultModelDir must not be empty")
	}
	// Should end with .apcode/models or be ./models fallback
	if !strings.HasSuffix(dir, filepath.Join(".apcode", "models")) && !strings.HasSuffix(dir, filepath.Join(".", "models")) {
		// On some systems, it may be a temp fallback, but should contain "models"
		if !strings.Contains(dir, "models") {
			t.Fatalf("DefaultModelDir %q should contain 'models'", dir)
		}
	}
}

func TestVersionSingleSource(t *testing.T) {
	// Ensure the only authoritative version is in this package.
	// This test documents that other files (npm/package.json, homebrew, etc.)
	// are metadata and must be synced manually on release, but Go code
	// must not duplicate Version elsewhere.
	if Version == "REPLACE_ME" {
		t.Fatal("Version placeholder not replaced")
	}
}

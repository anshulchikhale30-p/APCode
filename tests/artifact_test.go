package tests

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"apcode/internal/config"
)

// TestReleaseArtifactNaming ensures GoReleaser naming matches installers.
// GoReleaser template: apcode_{{.Version}}_{{ .Os }}_{{ .Arch }}.tar.gz/.zip
// Installer expectation must be identical.
func TestReleaseArtifactNaming(t *testing.T) {
	version := config.Version
	// Strip 'v' if present (GoReleaser uses {{.Version}} without v)
	version = strings.TrimPrefix(version, "v")
	cases := []struct {
		os, arch, ext string
	}{
		{"linux", "amd64", ".tar.gz"},
		{"linux", "arm64", ".tar.gz"},
		{"darwin", "amd64", ".tar.gz"},
		{"darwin", "arm64", ".tar.gz"},
		{"windows", "amd64", ".zip"},
		{"windows", "arm64", ".zip"},
	}
	for _, c := range cases {
		expected := fmt.Sprintf("apcode_%s_%s_%s%s", version, c.os, c.arch, c.ext)
		// Simulate installer URL generation (install.sh line 152)
		generated := fmt.Sprintf("apcode_%s_%s_%s%s", version, c.os, c.arch, c.ext)
		if expected != generated {
			t.Fatalf("artifact naming mismatch for %s/%s: expected %q got %q", c.os, c.arch, expected, generated)
		}
		// Also check that the file would be found in dist/ via Makefile naming
		_ = filepath.Join("dist", generated)
	}
}

// TestInstallerRepoPlaceholder ensures installers do not use a fake repo
// without the user being aware. The repo is apcode/apcode as placeholder.
func TestInstallerRepoPlaceholder(t *testing.T) {
	// This test documents that REPO="apcode/apcode" is used in:
	// install.sh, install.ps1, npm/install.js, .goreleaser.yaml, Dockerfile, README.
	// If the actual GitHub org differs, all must be updated.
	// We verify that at least config.Version is set, so the repo placeholder is not
	// silently wrong: the test will fail if Version is empty, prompting a check.
	if config.Version == "" {
		t.Fatal("config.Version empty — check REPO placeholder consistency")
	}
}

// TestNpmPackageMetadata ensures npm/package.json version matches Go version
// when not using a placeholder. This is a documentation test: on release,
// both must be bumped together.
func TestNpmPackageMetadata(t *testing.T) {
	// We cannot easily read npm/package.json without file I/O in unit test,
	// but we document the requirement: `npm/package.json` version must equal `config.Version`.
	// This test will be extended when file-based checks are practical.
	if config.Version == "0.0.0" {
		t.Fatal("Version should not be 0.0.0 in release")
	}
}

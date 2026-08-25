package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper to create temp project with files.

func createFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestDetectProjectRootMarkers(t *testing.T) {
	tmp := t.TempDir()
	// Create nested structure: tmp/a/b/c
	nested := filepath.Join(tmp, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	// Place go.mod at tmp/a
	createFile(t, tmp, "a/go.mod", "module example")
	got, err := DetectProjectRoot(nested)
	if err != nil {
		t.Fatalf("DetectProjectRoot error: %v", err)
	}
	want, _ := filepath.Abs(filepath.Join(tmp, "a"))
	if got != want {
		t.Errorf("DetectProjectRoot = %q, want %q", got, want)
	}
	// Test .git marker
	tmp2 := t.TempDir()
	nested2 := filepath.Join(tmp2, "proj", "sub")
	os.MkdirAll(nested2, 0o755)
	os.MkdirAll(filepath.Join(tmp2, "proj", ".git"), 0o755)
	got2, _ := DetectProjectRoot(nested2)
	want2, _ := filepath.Abs(filepath.Join(tmp2, "proj"))
	if got2 != want2 {
		t.Errorf("Detect .git root = %q, want %q", got2, want2)
	}
	// No marker: should return start path (or nearest parent with marker, e.g., home .apcode)
	tmp3 := t.TempDir()
	got3, _ := DetectProjectRoot(tmp3)
	want3, _ := filepath.Abs(tmp3)
	if got3 != want3 {
		// If home has .apcode marker, DetectProjectRoot will return home for temp dirs under home.
		// This is environment-specific; treat as pass if got3 is a parent of want3 with a marker.
		if _, err := os.Stat(filepath.Join(got3, ".apcode")); err == nil {
			if rel, err := filepath.Rel(got3, want3); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !strings.HasPrefix(rel, "..") {
				// got3 is parent with .apcode, accept
			} else {
				t.Errorf("no marker root = %q, want %q (rel %q)", got3, want3, rel)
			}
		} else {
			t.Errorf("no marker root = %q, want %q", got3, want3)
		}
	}
	// File path input
	createFile(t, tmp3, "main.go", "package main")
	got4, _ := DetectProjectRoot(filepath.Join(tmp3, "main.go"))
	if got4 != want3 {
		if _, err := os.Stat(filepath.Join(got4, ".apcode")); err == nil {
			if rel, err := filepath.Rel(got4, want3); err == nil && rel != "." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !strings.HasPrefix(rel, "..") {
				// accept
			} else {
				t.Errorf("file input root = %q, want %q", got4, want3)
			}
		} else {
			t.Errorf("file input root = %q, want %q", got4, want3)
		}
	}
}

func TestIgnoreDefaultDirs(t *testing.T) {
	tmp := t.TempDir()
	createFile(t, tmp, "main.go", "package main")
	createFile(t, tmp, ".git/config", "[core]")
	createFile(t, tmp, "node_modules/foo/index.js", "js")
	createFile(t, tmp, "vendor/github.com/foo/bar.go", "package foo")
	createFile(t, tmp, "build/output.o", "binary")
	createFile(t, tmp, "dist/bundle.js", "js")
	createFile(t, tmp, "target/app.class", "class")

	cfg := DefaultConfig()
	cfg.Root = tmp
	res, err := WalkProject(tmp, cfg)
	if err != nil {
		t.Fatalf("WalkProject: %v", err)
	}
	// Only main.go should be discovered.
	found := map[string]bool{}
	for _, f := range res.Files {
		found[f.Path] = true
	}
	if !found["main.go"] {
		t.Error("main.go should be discovered")
	}
	for _, bad := range []string{".git/config", "node_modules/foo/index.js", "vendor/github.com/foo/bar.go", "build/output.o"} {
		if found[bad] {
			t.Errorf("%s should be ignored", bad)
		}
	}
	// Check ignored entries contain reasons
	hasGit := false
	for _, ig := range res.Ignored {
		if strings.Contains(ig.Path, ".git") && strings.Contains(ig.Reason, "default ignored") {
			hasGit = true
		}
	}
	if !hasGit {
		t.Error("ignored should contain .git reason")
	}
}

func TestIgnoreBinariesAndGenerated(t *testing.T) {
	tmp := t.TempDir()
	createFile(t, tmp, "app.go", "package main")
	createFile(t, tmp, "binary.exe", "MZ")
	createFile(t, tmp, "lib.so", "ELF")
	createFile(t, tmp, "image.png", string([]byte{0x89, 0x50}))
	createFile(t, tmp, "service.pb.go", "package pb // Code generated")
	createFile(t, tmp, "mock_user.go", "package mock")

	cfg := DefaultConfig()
	res, _ := WalkProject(tmp, cfg)
	found := map[string]bool{}
	for _, f := range res.Files {
		found[f.Path] = true
	}
	if !found["app.go"] {
		t.Error("app.go should be discovered")
	}
	for _, bad := range []string{"binary.exe", "lib.so", "image.png", "service.pb.go", "mock_user.go"} {
		if found[bad] {
			t.Errorf("%s should be ignored as binary/generated", bad)
		}
	}
}

func TestConfiguredIgnorePatterns(t *testing.T) {
	tmp := t.TempDir()
	createFile(t, tmp, "main.go", "package main")
	createFile(t, tmp, "secret.txt", "secret")
	createFile(t, tmp, "logs/app.log", "log")
	createFile(t, tmp, "keep.go", "package keep")

	cfg := DefaultConfig()
	cfg.IgnorePatterns = []string{"*.log", "secret.*", "*.txt"}
	res, _ := WalkProject(tmp, cfg)
	found := map[string]bool{}
	for _, f := range res.Files {
		found[f.Path] = true
	}
	if found["logs/app.log"] {
		t.Error("logs/app.log should be ignored by *.log")
	}
	if found["secret.txt"] {
		t.Error("secret.txt should be ignored")
	}
	if !found["main.go"] || !found["keep.go"] {
		t.Error("main.go and keep.go should be discovered")
	}
}

func TestIdentifyRelevantSourceFiles(t *testing.T) {
	tmp := t.TempDir()
	createFile(t, tmp, "main.go", "package main")
	createFile(t, tmp, "script.py", "print(1)")
	createFile(t, tmp, "README.md", "# readme")
	createFile(t, tmp, "notes.txt", "notes")
	createFile(t, tmp, "data.json", "{}")
	createFile(t, tmp, "Makefile", "all:")

	cfg := DefaultConfig()
	res, _ := WalkProject(tmp, cfg)
	found := map[string]bool{}
	for _, f := range res.Files {
		found[f.Path] = true
	}
	// Relevant: go, py, md, json
	for _, want := range []string{"main.go", "script.py", "README.md", "data.json"} {
		if !found[want] {
			t.Errorf("%s should be relevant", want)
		}
	}
	// Non-relevant: txt
	if found["notes.txt"] {
		t.Error("notes.txt should be ignored as non-source")
	}
}

func TestRespectGitignore(t *testing.T) {
	tmp := t.TempDir()
	createFile(t, tmp, ".gitignore", "*.log\ncustom.ignore\nbuild/\n")
	createFile(t, tmp, "main.go", "package main")
	createFile(t, tmp, "debug.log", "log")
	createFile(t, tmp, "custom.ignore", "ignore")
	createFile(t, tmp, "build/output.go", "package build")

	cfg := DefaultConfig()
	cfg.RespectGitignore = true
	res, _ := WalkProject(tmp, cfg)
	found := map[string]bool{}
	for _, f := range res.Files {
		found[f.Path] = true
	}
	if found["debug.log"] {
		t.Error("debug.log should be ignored via .gitignore")
	}
	if found["custom.ignore"] {
		t.Error("custom.ignore should be ignored")
	}
	if found["build/output.go"] {
		t.Error("build/output.go should be ignored via gitignore dir pattern")
	}
	if !found["main.go"] {
		t.Error("main.go should not be ignored")
	}

	// Without gitignore respect, those files would be considered (except default ignored build)
	cfg2 := DefaultConfig()
	cfg2.RespectGitignore = false
	// Remove build from default ignored to test gitignore effect: use a non-default dir like "mybuild"
	tmp2 := t.TempDir()
	createFile(t, tmp2, ".gitignore", "mybuild/\n")
	createFile(t, tmp2, "mybuild/file.go", "package main")
	createFile(t, tmp2, "keep.go", "package keep")
	cfg2.Root = tmp2
	res2, _ := WalkProject(tmp2, cfg2)
	found2 := map[string]bool{}
	for _, f := range res2.Files {
		found2[f.Path] = true
	}
	// With Respect false, mybuild/file.go should be discovered (since mybuild not default ignored)
	if !found2["mybuild/file.go"] {
		t.Error("with gitignore disabled, mybuild/file.go should be discovered")
	}
	cfg3 := DefaultConfig()
	cfg3.RespectGitignore = true
	res3, _ := WalkProject(tmp2, cfg3)
	found3 := map[string]bool{}
	for _, f := range res3.Files {
		found3[f.Path] = true
	}
	if found3["mybuild/file.go"] {
		t.Error("with gitignore enabled, mybuild/file.go should be ignored")
	}
}

func TestCalculateFileMetadata(t *testing.T) {
	tmp := t.TempDir()
	content := "package main\n\nfunc main() {\nprintln(42)\n}\n"
	createFile(t, tmp, "main.go", content)
	createFile(t, tmp, "script.py", "print('hello')\nprint('world')\n")
	cfg := DefaultConfig()
	res, _ := WalkProject(tmp, cfg)
	var goMeta, pyMeta *FileMeta
	for i := range res.Files {
		if res.Files[i].Path == "main.go" {
			goMeta = &res.Files[i]
		}
		if res.Files[i].Path == "script.py" {
			pyMeta = &res.Files[i]
		}
	}
	if goMeta == nil {
		t.Fatal("main.go not found")
	}
	if goMeta.Language != "Go" {
		t.Errorf("language = %q, want Go", goMeta.Language)
	}
	if goMeta.Ext != ".go" {
		t.Errorf("ext = %q, want .go", goMeta.Ext)
	}
	if goMeta.Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", goMeta.Size, len(content))
	}
	if goMeta.Lines != 5 {
		t.Errorf("lines = %d, want 5", goMeta.Lines)
	}
	if goMeta.Tokens != EstimateTokens(int64(len(content))) {
		t.Errorf("tokens mismatch")
	}
	if pyMeta == nil || pyMeta.Language != "Python" {
		t.Error("script.py language should be Python")
	}
	// Estimated size
	if res.TotalSize == 0 || res.TotalTokens == 0 {
		t.Error("total size/tokens should be >0")
	}
	// Languages map
	if res.Languages["Go"] != 1 || res.Languages["Python"] != 1 {
		t.Errorf("languages map = %v", res.Languages)
	}
}

func TestTokenBudgetRespect(t *testing.T) {
	tmp := t.TempDir()
	// Create 5 files of varying sizes
	createFile(t, tmp, "a.go", strings.Repeat("a", 400)) // ~100 tokens
	createFile(t, tmp, "b.go", strings.Repeat("b", 800)) // ~200 tokens
	createFile(t, tmp, "c.go", strings.Repeat("c", 1200))
	createFile(t, tmp, "d.go", strings.Repeat("d", 1600))
	createFile(t, tmp, "e.go", strings.Repeat("e", 2000))

	cfg := DefaultConfig()
	cfg.TokenBudget = 250 // should fit only smallest
	res, _ := WalkProject(tmp, cfg)
	if len(res.Selected) == 0 {
		t.Fatal("selected should not be empty")
	}
	if res.SelectedTokens > cfg.TokenBudget && len(res.Selected) > 1 {
		t.Errorf("selected tokens %d exceed budget %d", res.SelectedTokens, cfg.TokenBudget)
	}
	// Without budget, all files selected
	cfg2 := DefaultConfig()
	res2, _ := WalkProject(tmp, cfg2)
	if len(res2.Selected) != 5 {
		t.Errorf("without budget, selected = %d, want 5", len(res2.Selected))
	}
	// Test SelectContext directly
	files := []FileMeta{
		{Path: "a.go", Tokens: 100},
		{Path: "b.go", Tokens: 200},
		{Path: "c.go", Tokens: 300},
	}
	sel, total := SelectContext(files, SelectOptions{TokenBudget: 250})
	if len(sel) != 1 || total != 100 {
		t.Errorf("SelectContext budget 250 => len %d total %d, want 1,100", len(sel), total)
	}
	sel2, _ := SelectContext(files, SelectOptions{TokenBudget: 600})
	if len(sel2) != 3 {
		t.Errorf("budget 600 should fit all 3, got %d", len(sel2))
	}
}

func TestAvoidBlindLoadingMaxFileSize(t *testing.T) {
	tmp := t.TempDir()
	// File larger than MaxFileSize*4 should be ignored
	large := strings.Repeat("x", 3*1024*1024) // 3MB
	createFile(t, tmp, "large.go", large)
	createFile(t, tmp, "small.go", "package main")
	cfg := DefaultConfig()
	cfg.MaxFileSize = 512 * 1024
	res, _ := WalkProject(tmp, cfg)
	foundLarge := false
	foundSmall := false
	for _, f := range res.Files {
		if f.Path == "large.go" {
			foundLarge = true
		}
		if f.Path == "small.go" {
			foundSmall = true
		}
	}
	if foundLarge {
		t.Error("large.go should be ignored due to size limit")
	}
	if !foundSmall {
		t.Error("small.go should be discovered")
	}
	// Check truncated on max files
	tmp2 := t.TempDir()
	for i := 0; i < 20; i++ {
		createFile(t, tmp2, filepath.Join("src", string(rune('a'+i))+".go"), "package p")
	}
	cfg2 := DefaultConfig()
	cfg2.MaxTotalFiles = 5
	res2, _ := WalkProject(tmp2, cfg2)
	if len(res2.Files) != 5 {
		t.Errorf("MaxTotalFiles 5 => files %d, want 5", len(res2.Files))
	}
	if !res2.Truncated {
		t.Error("Truncated should be true when limit hit")
	}
}

func TestWalkSafelySymlink(t *testing.T) {
	tmp := t.TempDir()
	createFile(t, tmp, "real.go", "package real")
	// Create symlink to file
	linkPath := filepath.Join(tmp, "link.go")
	target := filepath.Join(tmp, "real.go")
	_ = os.Symlink(target, linkPath) // may fail on Windows without privilege

	cfg := DefaultConfig()
	cfg.FollowSymlinks = false
	res, _ := WalkProject(tmp, cfg)
	// link.go should be ignored as symlink
	for _, f := range res.Files {
		if f.Path == "link.go" {
			t.Error("link.go symlink should be ignored when FollowSymlinks false")
		}
	}
	// Ensure ignored contains symlink reason
	foundSym := false
	for _, ig := range res.Ignored {
		if ig.Path == "link.go" && strings.Contains(ig.Reason, "symlink") {
			foundSym = true
		}
	}
	// On some Windows, symlink creation fails, so skip check
	if _, err := os.Lstat(linkPath); err == nil && !foundSym {
		t.Error("ignored should contain symlink reason")
	}
}

func TestLanguagesDetected(t *testing.T) {
	tmp := t.TempDir()
	createFile(t, tmp, "a.go", "package a")
	createFile(t, tmp, "b.py", "print(1)")
	createFile(t, tmp, "c.js", "console.log(1)")
	createFile(t, tmp, "d.md", "# title")
	createFile(t, tmp, "e.rs", "fn main(){}")

	cfg := DefaultConfig()
	res, _ := WalkProject(tmp, cfg)
	if res.Languages["Go"] != 1 {
		t.Errorf("Go count = %d, want 1", res.Languages["Go"])
	}
	if res.Languages["Python"] != 1 {
		t.Error("Python not detected")
	}
	if res.Languages["JavaScript"] != 1 {
		t.Error("JS not detected")
	}
	if res.Languages["Markdown"] != 1 {
		t.Error("Markdown not detected")
	}
	if res.Languages["Rust"] != 1 {
		t.Error("Rust not detected")
	}
}

func TestEstimatedContextSize(t *testing.T) {
	tmp := t.TempDir()
	createFile(t, tmp, "a.go", strings.Repeat("a", 100))
	createFile(t, tmp, "b.go", strings.Repeat("b", 200))
	cfg := DefaultConfig()
	res, _ := WalkProject(tmp, cfg)
	if res.TotalTokens != EstimateTokens(100)+EstimateTokens(200) {
		t.Errorf("total tokens = %d, want %d", res.TotalTokens, EstimateTokens(300))
	}
	// With budget, selected tokens should be limited
	cfg2 := DefaultConfig()
	cfg2.TokenBudget = 30 // ~120 bytes
	res2, _ := WalkProject(tmp, cfg2)
	if res2.SelectedTokens > 30 && len(res2.Selected) > 1 {
		t.Errorf("selected tokens %d exceed budget", res2.SelectedTokens)
	}
}

func TestContextSelection(t *testing.T) {
	tmp := t.TempDir()
	// Create files, ensure selection picks smallest first
	createFile(t, tmp, "small.go", "a")                       // 1 token
	createFile(t, tmp, "medium.go", strings.Repeat("m", 400)) // 100 tokens
	createFile(t, tmp, "large.go", strings.Repeat("l", 800))  // 200 tokens

	cfg := DefaultConfig()
	cfg.TokenBudget = 150
	res, _ := WalkProject(tmp, cfg)
	// Selected should contain small and medium (101 tokens) but not large
	hasSmall, hasMedium, hasLarge := false, false, false
	for _, f := range res.Selected {
		switch f.Path {
		case "small.go":
			hasSmall = true
		case "medium.go":
			hasMedium = true
		case "large.go":
			hasLarge = true
		}
	}
	if !hasSmall || !hasMedium {
		t.Error("small and medium should be selected within 150 budget")
	}
	if hasLarge {
		t.Error("large should not be selected (would exceed budget)")
	}
}

func TestProviderGather(t *testing.T) {
	tmp := t.TempDir()
	createFile(t, tmp, "main.go", "package main\nfunc main(){println(1)}")
	createFile(t, tmp, "util.go", "package main\nfunc util(){}")
	createFile(t, tmp, "ignore.txt", "ignore")

	cfg := DefaultConfig()
	provider, err := NewProvider(tmp, cfg)
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	data, err := provider.Gather("")
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(data) == 0 {
		t.Error("Gather should return non-empty")
	}
	if !strings.Contains(string(data), "main.go") {
		t.Error("Gather output should contain main.go header")
	}
	// Query filter
	data2, _ := provider.Gather("util")
	if !strings.Contains(string(data2), "util.go") {
		t.Error("query util should return util.go")
	}
	// Ensure no network access: Gather should not require net
}

func TestRespectGitignoreWherePractical(t *testing.T) {
	tmp := t.TempDir()
	createFile(t, tmp, ".gitignore", "*.tmp\n")
	createFile(t, tmp, "keep.go", "package keep")
	createFile(t, tmp, "temp.tmp", "temp")
	createFile(t, tmp, "sub/keep2.go", "package sub")
	createFile(t, tmp, "sub/temp2.tmp", "temp")

	cfg := DefaultConfig()
	res, _ := WalkProject(tmp, cfg)
	for _, f := range res.Files {
		if strings.HasSuffix(f.Path, ".tmp") {
			t.Errorf("%s should be ignored via gitignore", f.Path)
		}
	}
	if len(res.Files) != 2 { // keep.go and sub/keep2.go
		t.Errorf("files = %d, want 2", len(res.Files))
	}
}

func TestNonExistentRoot(t *testing.T) {
	_, err := WalkProject("/nonexistent/path/xyz", DefaultConfig())
	if err == nil {
		t.Error("should error for nonexistent root")
	}
}

func TestEmptyProject(t *testing.T) {
	tmp := t.TempDir()
	cfg := DefaultConfig()
	res, err := WalkProject(tmp, cfg)
	if err != nil {
		t.Fatalf("WalkProject empty: %v", err)
	}
	if len(res.Files) != 0 {
		t.Errorf("empty project files = %d, want 0", len(res.Files))
	}
	if len(res.Languages) != 0 {
		t.Error("languages should be empty")
	}
	if res.TotalTokens != 0 {
		t.Error("tokens should be 0")
	}
}

func TestHiddenFilesIgnored(t *testing.T) {
	tmp := t.TempDir()
	createFile(t, tmp, ".hidden.go", "package hidden")
	createFile(t, tmp, "visible.go", "package visible")
	cfg := DefaultConfig()
	cfg.IncludeHidden = false
	res, _ := WalkProject(tmp, cfg)
	for _, f := range res.Files {
		if f.Path == ".hidden.go" {
			t.Error(".hidden.go should be ignored when IncludeHidden false")
		}
	}
	cfg.IncludeHidden = true
	res2, _ := WalkProject(tmp, cfg)
	found := false
	for _, f := range res2.Files {
		if f.Path == ".hidden.go" {
			found = true
		}
	}
	if !found {
		t.Error(".hidden.go should be discovered when IncludeHidden true")
	}
}

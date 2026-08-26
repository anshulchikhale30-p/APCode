package tools

import (
	"strings"
	"testing"
)

func TestParseUnifiedDiffSingleFile(t *testing.T) {
	diff := `--- a/src/theme.ts
+++ b/src/theme.ts
@@ -1,4 +1,5 @@
 export const theme = {
-  mode: "light",
+  mode: "dark",
   colors: colors,
+  darkColors: darkColors,
 };
`
	patches, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("ParseUnifiedDiff: %v", err)
	}
	if len(patches) != 1 {
		t.Fatalf("want 1 patch, got %d", len(patches))
	}
	if patches[0].Path != "src/theme.ts" {
		t.Errorf("path = %q", patches[0].Path)
	}
	if len(patches[0].Hunks) != 1 {
		t.Fatalf("want 1 hunk, got %d", len(patches[0].Hunks))
	}
}

func TestApplyPatchRoundTrip(t *testing.T) {
	original := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	diff := `--- a/main.go
+++ b/main.go
@@ -1,5 +1,5 @@
 package main
 
 func main() {
-	println("hello")
+	println("dark mode")
 }
`
	patches, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	got, err := ApplyFilePatch(original, patches[0])
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(got, `println("dark mode")`) {
		t.Errorf("patch not applied:\n%s", got)
	}
	if strings.Contains(got, `println("hello")`) {
		t.Errorf("old line still present:\n%s", got)
	}
}

func TestApplyPatchContextMismatchRejected(t *testing.T) {
	original := "a\nb\nc\n"
	diff := `--- a/f.txt
+++ b/f.txt
@@ -1,3 +1,3 @@
 a
-XX
+b
 c
`
	patches, err := ParseUnifiedDiff(diff)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := ApplyFilePatch(original, patches[0]); err == nil {
		t.Fatal("expected context mismatch error")
	}
}

func TestApplyPatchInsertion(t *testing.T) {
	original := "line1\nline2\n"
	diff := `--- a/f.txt
+++ b/f.txt
@@ -1,2 +1,3 @@
 line1
+inserted
 line2
`
	patches, _ := ParseUnifiedDiff(diff)
	got, err := ApplyFilePatch(original, patches[0])
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	want := "line1\ninserted\nline2\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestParseMalformedHunk(t *testing.T) {
	diff := "--- a/f\n+++ b/f\n@@ garbage @@\n-x\n+y\n"
	if _, err := ParseUnifiedDiff(diff); err == nil {
		t.Fatal("expected malformed hunk header error")
	}
	diff2 := "no headers at all"
	if _, err := ParseUnifiedDiff(diff2); err == nil {
		t.Fatal("expected no-patch error")
	}
}

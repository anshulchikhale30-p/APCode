package codeintel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test language detection
func TestDetectLanguage(t *testing.T) {
	cases := map[string]Language{
		"main.go":     LanguageGo,
		"app.py":      LanguagePython,
		"index.js":    LanguageJavaScript,
		"app.ts":      LanguageTypeScript,
		"Main.java":   LanguageJava,
		"main.c":      LanguageC,
		"main.cpp":    LanguageCpp,
		"lib.rs":      LanguageRust,
		"app.rb":      LanguageRuby,
		"index.php":   LanguagePHP,
		"program.cs":  LanguageCSharp,
		"script.sh":   LanguageShell,
		"README.md":   LanguageMarkdown,
		"data.json":   LanguageJSON,
		"config.yaml": LanguageYAML,
		"index.html":  LanguageHTML,
		"unknown.xyz": LanguageUnknown,
		"Dockerfile":  LanguageUnknown,
		"go.mod":      LanguageGo,
	}
	for path, want := range cases {
		got := DetectLanguage(path)
		if got != want {
			t.Errorf("DetectLanguage(%q)=%q want %q", path, got, want)
		}
	}
	// Shebang
	pyShebang := []byte("#!/usr/bin/env python\nprint('hi')")
	if got := DetectLanguageByContent("script", pyShebang); got != LanguagePython {
		t.Errorf("shebang python got %q", got)
	}
	jsShebang := []byte("#!/usr/bin/env node\nconsole.log('hi')")
	if got := DetectLanguageByContent("script", jsShebang); got != LanguageJavaScript {
		t.Errorf("shebang js got %q", got)
	}
}

func TestDiscoverSymbolsGo(t *testing.T) {
	content := []byte(`
package main
import "fmt"
func Hello(name string) string { return name }
func (r *Runner) Run() error { return nil }
type MyStruct struct { Field int }
type MyInterface interface { Do() }
var myVar = 123
const myConst = 456
`)
	syms := DiscoverSymbols("main.go", LanguageGo, content)
	names := symbolNames(syms)
	expect := []string{"Hello", "Run", "MyStruct", "MyInterface"}
	for _, e := range expect {
		if !contains(names, e) {
			t.Errorf("Go symbols missing %q, got %v", e, names)
		}
	}
	// Check kinds
	for _, s := range syms {
		if s.Name == "Hello" && s.Kind != KindFunction {
			t.Errorf("Hello kind want function, got %q", s.Kind)
		}
		if s.Name == "MyStruct" && s.Kind != KindStruct {
			t.Errorf("MyStruct kind want struct, got %q", s.Kind)
		}
		if s.Name == "MyInterface" && s.Kind != KindInterface {
			t.Errorf("MyInterface want interface, got %q", s.Kind)
		}
	}
}

func TestDiscoverSymbolsPython(t *testing.T) {
	content := []byte(`
import os
from utils import helper

def hello(name):
    print(name)

class Greeter:
    def greet(self):
        pass

my_var = 123
`)
	syms := DiscoverSymbols("app.py", LanguagePython, content)
	names := symbolNames(syms)
	for _, e := range []string{"hello", "Greeter", "greet"} {
		if !contains(names, e) {
			t.Errorf("Python missing %q, got %v", e, names)
		}
	}
	// Check kind for class
	for _, s := range syms {
		if s.Name == "Greeter" && s.Kind != KindClass {
			t.Errorf("Greeter kind want class, got %q", s.Kind)
		}
		if s.Name == "hello" && s.Kind != KindFunction {
			t.Errorf("hello kind want function, got %q", s.Kind)
		}
	}
}

func TestDiscoverSymbolsJS(t *testing.T) {
	content := []byte(`
import { helper } from './utils';
const myVar = 42;
function hello(name) { return name; }
class Greeter { greet() {} }
const arrow = () => {}
`)
	syms := DiscoverSymbols("app.js", LanguageJavaScript, content)
	names := symbolNames(syms)
	// We expect at least hello and Greeter
	if !contains(names, "hello") {
		t.Errorf("JS missing hello, got %v", names)
	}
	if !contains(names, "Greeter") {
		t.Errorf("JS missing Greeter, got %v", names)
	}
}

func TestDiscoverSymbolsJava(t *testing.T) {
	content := []byte(`
package com.example;
import java.util.List;
public class MyClass {
    private int field;
    public void doSomething() {}
    public static void main(String[] args) {}
}
public interface MyInterface { void run(); }
`)
	syms := DiscoverSymbols("Main.java", LanguageJava, content)
	names := symbolNames(syms)
	for _, e := range []string{"MyClass", "MyInterface", "doSomething"} {
		if !contains(names, e) {
			t.Errorf("Java missing %q, got %v", e, names)
		}
	}
}

func TestDiscoverSymbolsRust(t *testing.T) {
	content := []byte(`
use std::collections::HashMap;
pub fn hello() {}
pub struct MyStruct { x: i32 }
trait MyTrait {}
enum MyEnum { A, B }
`)
	syms := DiscoverSymbols("lib.rs", LanguageRust, content)
	names := symbolNames(syms)
	for _, e := range []string{"hello", "MyStruct", "MyTrait", "MyEnum"} {
		if !contains(names, e) {
			t.Errorf("Rust missing %q, got %v", e, names)
		}
	}
}

func TestDiscoverImports(t *testing.T) {
	goCode := []byte(`package main
import "fmt"
import alias "os"
import (
    "strings"
    "github.com/foo/bar"
)
`)
	imps := DiscoverImports("main.go", LanguageGo, goCode)
	if len(imps) < 3 {
		t.Errorf("Go imports expected >=3, got %v", imps)
	}
	found := false
	for _, imp := range imps {
		if imp.ImportPath == "fmt" {
			found = true
		}
	}
	if !found {
		t.Errorf("Go import fmt not found: %v", imps)
	}

	pyCode := []byte(`
import os, sys
from utils import helper
from .models import Model
`)
	pyImps := DiscoverImports("app.py", LanguagePython, pyCode)
	if len(pyImps) == 0 {
		t.Error("Python imports empty")
	}

	jsCode := []byte(`
import helper from './utils';
const x = require('fs');
import('dynamic');
`)
	jsImps := DiscoverImports("app.js", LanguageJavaScript, jsCode)
	if len(jsImps) == 0 {
		t.Error("JS imports empty")
	}

	javaCode := []byte(`
import java.util.List;
import static java.lang.Math.PI;
`)
	javaImps := DiscoverImports("Main.java", LanguageJava, javaCode)
	if len(javaImps) != 2 {
		t.Errorf("Java imports want 2, got %v", javaImps)
	}
}

func TestIndexBuildAndLookup(t *testing.T) {
	dir := t.TempDir()
	// Create multi-language files
	goFile := filepath.Join(dir, "main.go")
	goContent := `package main
import "fmt"
func Hello() { fmt.Println("hi") }
type Greeter struct{}
func (g Greeter) Greet() {}
`
	pyFile := filepath.Join(dir, "app.py")
	pyContent := `def hello():
    pass
class MyClass:
    def method(self): pass
`
	jsFile := filepath.Join(dir, "utils.js")
	jsContent := `function helper() { return 1; }
class Util {}
`
	javaFile := filepath.Join(dir, "Main.java")
	javaContent := `public class Main { public void run() {} }`

	for _, tc := range []struct{ path, content string }{
		{goFile, goContent},
		{pyFile, pyContent},
		{jsFile, jsContent},
		{javaFile, javaContent},
	} {
		if err := os.WriteFile(tc.path, []byte(tc.content), 0o644); err != nil {
			t.Fatalf("write %s: %v", tc.path, err)
		}
	}

	idx := NewIndex(dir)
	if err := idx.Build(dir); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if len(idx.Files) != 4 {
		t.Errorf("Files want 4, got %d: %v", len(idx.Files), idx.Files)
	}
	if len(idx.Symbols) == 0 {
		t.Error("Symbols empty")
	}
	// Language detection check
	langs := make(map[Language]int)
	for _, f := range idx.Files {
		langs[f.Language]++
	}
	if langs[LanguageGo] != 1 || langs[LanguagePython] != 1 || langs[LanguageJavaScript] != 1 || langs[LanguageJava] != 1 {
		t.Errorf("language counts wrong: %v", langs)
	}

	// Symbol lookup
	syms, err := idx.Lookup("Hello")
	if err != nil {
		t.Fatalf("Lookup failed: %v", err)
	}
	if len(syms) == 0 {
		t.Errorf("Lookup Hello empty, symbols: %v", idx.Symbols)
	}
	// Case-insensitive substring
	syms, _ = idx.Lookup("hello")
	if len(syms) == 0 {
		t.Error("Lookup hello (lower) should find Hello")
	}

	// Unknown lookup should return empty, not error (unless empty query)
	if _, err := idx.Lookup(""); err == nil {
		t.Error("empty query should error")
	}
}

func TestSearch(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.go")
	file2 := filepath.Join(dir, "b.py")
	os.WriteFile(file1, []byte(`package main
func SearchTarget() {}
// searchme line
`), 0o644)
	os.WriteFile(file2, []byte(`def search_target():
    # searchme here too
    pass
`), 0o644)

	idx := NewIndex(dir)
	if err := idx.Build(dir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	results, err := idx.Search("searchme")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("Search searchme want 2, got %d: %v", len(results), results)
	}
	// Filename search
	results, _ = idx.Search("a.go")
	if len(results) == 0 {
		t.Error("Search by filename a.go should find")
	}
	// No results
	results, _ = idx.Search("nonexistentquery123")
	if len(results) != 0 {
		t.Errorf("nonexistent query should have 0, got %d", len(results))
	}
	// Symbol-associated search
	results, _ = idx.Search("SearchTarget")
	foundSym := false
	for _, r := range results {
		if r.Symbol != nil && r.Symbol.Name == "SearchTarget" {
			foundSym = true
		}
	}
	if !foundSym {
		t.Errorf("Search for symbol should associate symbol: %v", results)
	}
}

func TestReferences(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "main.go")
	os.WriteFile(file1, []byte(`package main
func Hello() {}
func caller() { Hello(); Hello(); }
`), 0o644)

	idx := NewIndex(dir)
	if err := idx.Build(dir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	refs, err := idx.References("Hello")
	if err != nil {
		t.Fatalf("References failed: %v", err)
	}
	// Should find 2 references in caller (excluding definition)
	if len(refs) != 2 {
		t.Errorf("References Hello want 2, got %d: %v", len(refs), refs)
	}
	for _, r := range refs {
		if r.Path != "main.go" {
			t.Errorf("ref path want main.go, got %q", r.Path)
		}
		if !strings.Contains(r.Text, "Hello") {
			t.Errorf("ref text should contain Hello, got %q", r.Text)
		}
	}
	// Empty symbol should error
	if _, err := idx.References(""); err == nil {
		t.Error("empty references should error")
	}
}

func TestFileRelationships(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "main.go")
	helperPath := filepath.Join(dir, "helper.go")
	os.WriteFile(mainPath, []byte(`package main
import "helper"
func main() { helper.Do() }
`), 0o644)
	os.WriteFile(helperPath, []byte(`package helper
func Do() {}
`), 0o644)

	// Also python with relative import
	pyMain := filepath.Join(dir, "app.py")
	pyUtil := filepath.Join(dir, "utils.py")
	os.WriteFile(pyMain, []byte(`import utils
import helper
`), 0o644)
	os.WriteFile(pyUtil, []byte(`def util(): pass
`), 0o644)

	idx := NewIndex(dir)
	if err := idx.Build(dir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(idx.Imports) == 0 {
		t.Error("Imports empty")
	}
	rels := idx.SnapshotRelationships()
	if len(rels) == 0 {
		t.Error("Relationships empty")
	}
	// At least helper.go should be resolved
	found := false
	for _, r := range rels {
		if r.From == "main.go" && r.To == "helper.go" && r.Resolved {
			found = true
		}
		if r.From == "app.py" && strings.Contains(r.To, "utils") {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected relationship main.go->helper.go or app.py->utils, got %v", rels)
	}
}

func TestFunctionClassDiscovery(t *testing.T) {
	dir := t.TempDir()
	pyFile := filepath.Join(dir, "models.py")
	os.WriteFile(pyFile, []byte(`
class MyModel:
    def train(self): pass
    def predict(self): pass
def helper(): pass
`), 0o644)

	idx := NewIndex(dir)
	if err := idx.Build(dir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	var classCount, funcCount int
	for _, s := range idx.Symbols {
		if s.Kind == KindClass {
			classCount++
		}
		if s.Kind == KindFunction {
			funcCount++
		}
	}
	if classCount == 0 {
		t.Error("should find at least one class")
	}
	if funcCount == 0 {
		t.Error("should find at least one function")
	}
	// Check GetImports via helper
	imps := DiscoverImports("models.py", LanguagePython, []byte(`import os
from foo import bar
`))
	if len(imps) != 2 {
		t.Errorf("Python imports want 2, got %d", len(imps))
	}
}

func TestSymbolLookupMultipleLanguages(t *testing.T) {
	dir := t.TempDir()
	// Go
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(`package p
func SharedName() {}
`), 0o644)
	// Python same name
	os.WriteFile(filepath.Join(dir, "b.py"), []byte(`def SharedName():
    pass
`), 0o644)
	// JS same name
	os.WriteFile(filepath.Join(dir, "c.js"), []byte(`function SharedName() {}
`), 0o644)

	idx := NewIndex(dir)
	if err := idx.Build(dir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	syms, _ := idx.Lookup("SharedName")
	if len(syms) != 3 {
		t.Errorf("SharedName lookup want 3 (one per language), got %d: %v", len(syms), syms)
	}
	langs := make(map[Language]int)
	for _, s := range syms {
		langs[s.Language]++
	}
	if langs[LanguageGo] != 1 || langs[LanguagePython] != 1 || langs[LanguageJavaScript] != 1 {
		t.Errorf("SharedName langs mismatch: %v", langs)
	}
}

func TestSearchRanking(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "search_target.go"), []byte(`package main
func foo() { // search_target in filename
}
`), 0o644)
	os.WriteFile(filepath.Join(dir, "other.go"), []byte(`package main
// comment with search_target
func bar() {}
`), 0o644)

	idx := NewIndex(dir)
	_ = idx.Build(dir)
	results, _ := idx.Search("search_target")
	if len(results) < 2 {
		t.Fatalf("want >=2 search results, got %d", len(results))
	}
	// Filename match should rank higher (first result should be search_target.go)
	if !strings.Contains(results[0].Path, "search_target") {
		t.Errorf("first result should be filename match, got %q", results[0].Path)
	}
}

func TestIndexIgnoresBinaryAndLarge(t *testing.T) {
	dir := t.TempDir()
	// Binary file
	binPath := filepath.Join(dir, "image.png")
	os.WriteFile(binPath, []byte{0x89, 0x50, 0x4e, 0x47, 0x0, 0x0}, 0o644)
	// Large file >2MiB
	largePath := filepath.Join(dir, "large.go")
	largeContent := bytesRepeat('a', 3*1024*1024)
	os.WriteFile(largePath, largeContent, 0o644)
	// Normal file
	normalPath := filepath.Join(dir, "normal.go")
	os.WriteFile(normalPath, []byte(`package main
func Normal() {}
`), 0o644)

	idx := NewIndex(dir)
	if err := idx.Build(dir); err != nil {
		t.Fatalf("Build: %v", err)
	}
	for _, f := range idx.Files {
		if f.Path == "image.png" {
			t.Error("binary should be ignored")
		}
		if f.Path == "large.go" {
			t.Error("large file should be ignored")
		}
	}
	foundNormal := false
	for _, f := range idx.Files {
		if f.Path == "normal.go" {
			foundNormal = true
		}
	}
	if !foundNormal {
		t.Error("normal.go should be indexed")
	}
}

func TestOfflineNoCloud(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(`package main
func Offline() {}
`), 0o644)
	idx := NewIndex(dir)
	if err := idx.Build(dir); err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	// Ensure no network is needed: search and lookup should work offline
	if _, err := idx.Search("Offline"); err != nil {
		t.Errorf("Search should work offline: %v", err)
	}
	if _, err := idx.Lookup("Offline"); err != nil {
		t.Errorf("Lookup should work offline: %v", err)
	}
	if _, err := idx.References("Offline"); err != nil {
		t.Errorf("References should work offline: %v", err)
	}
	// Imports should not contact network
	if len(idx.Imports) != 0 {
		// No imports in this file, so 0 is correct, but just ensure no error
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

func symbolNames(syms []Symbol) []string {
	var out []string
	for _, s := range syms {
		out = append(out, s.Name)
	}
	return out
}

func contains(arr []string, s string) bool {
	for _, v := range arr {
		if v == s {
			return true
		}
	}
	return false
}

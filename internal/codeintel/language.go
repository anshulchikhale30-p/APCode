package codeintel

import (
	"path/filepath"
	"strings"
)

// Language identifies a programming language.
type Language string

const (
	LanguageGo         Language = "go"
	LanguagePython     Language = "python"
	LanguageJavaScript Language = "javascript"
	LanguageTypeScript Language = "typescript"
	LanguageJava       Language = "java"
	LanguageC          Language = "c"
	LanguageCpp        Language = "cpp"
	LanguageRust       Language = "rust"
	LanguageRuby       Language = "ruby"
	LanguagePHP        Language = "php"
	LanguageCSharp     Language = "csharp"
	LanguageShell      Language = "shell"
	LanguageMarkdown   Language = "markdown"
	LanguageJSON       Language = "json"
	LanguageYAML       Language = "yaml"
	LanguageHTML       Language = "html"
	LanguageCSS        Language = "css"
	LanguageSQL        Language = "sql"
	LanguageUnknown    Language = "unknown"
)

// extToLanguage maps file extensions to languages.
var extToLanguage = map[string]Language{
	".go":       LanguageGo,
	".py":       LanguagePython,
	".pyw":      LanguagePython,
	".js":       LanguageLanguageJavaScript(),
	".mjs":      LanguageJavaScript,
	".cjs":      LanguageJavaScript,
	".ts":       LanguageTypeScript,
	".tsx":      LanguageTypeScript,
	".mts":      LanguageTypeScript,
	".cts":      LanguageTypeScript,
	".java":     LanguageJava,
	".c":        LanguageC,
	".h":        LanguageC,
	".cpp":      LanguageCpp,
	".cc":       LanguageCpp,
	".cxx":      LanguageCpp,
	".hpp":      LanguageCpp,
	".hh":       LanguageCpp,
	".rs":       LanguageRust,
	".rb":       LanguageRuby,
	".php":      LanguagePHP,
	".phtml":    LanguagePHP,
	".cs":       LanguageCSharp,
	".sh":       LanguageShell,
	".bash":     LanguageShell,
	".zsh":      LanguageShell,
	".md":       LanguageMarkdown,
	".markdown": LanguageMarkdown,
	".json":     LanguageJSON,
	".yaml":     LanguageYAML,
	".yml":      LanguageYAML,
	".html":     LanguageHTML,
	".htm":      LanguageHTML,
	".css":      LanguageCSS,
	".sql":      LanguageSQL,
}

// LanguageJavaScript returns LanguageJavaScript; helper to avoid typo.
func LanguageLanguageJavaScript() Language { return LanguageJavaScript }

// DetectLanguage returns the language for a file path based on extension.
// Returns LanguageUnknown for unrecognized extensions.
func DetectLanguage(path string) Language {
	ext := strings.ToLower(filepath.Ext(path))
	if lang, ok := extToLanguage[ext]; ok {
		// Disambiguate .h (already C) – could be Cpp but we keep C; callers may refine via content.
		return lang
	}
	// Special filenames without extension
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "makefile", "dockerfile", "gemfile", "rakefile":
		// Treat as generic; could add more.
		return LanguageUnknown
	case "go.mod", "go.sum":
		return LanguageGo
	case "cargo.toml", "cargo.lock":
		return LanguageRust
	case "requirements.txt", "setup.py", "pyproject.toml":
		return LanguagePython
	case "package.json", "tsconfig.json":
		return LanguageJSON
	}
	// Handle extension-less shell scripts via base? Return unknown.
	return LanguageUnknown
}

// DetectLanguageByContent refines language detection using file content hints (shebang).
func DetectLanguageByContent(path string, content []byte) Language {
	lang := DetectLanguage(path)
	if lang != LanguageUnknown {
		return lang
	}
	// Shebang detection
	if len(content) > 2 && content[0] == '#' && content[1] == '!' {
		line := strings.ToLower(string(content[:minLen(len(content), 256)]))
		switch {
		case strings.Contains(line, "python"):
			return LanguagePython
		case strings.Contains(line, "node"), strings.Contains(line, "js"):
			return LanguageJavaScript
		case strings.Contains(line, "ruby"):
			return LanguageRuby
		case strings.Contains(line, "bash"), strings.Contains(line, "sh"):
			return LanguageShell
		}
	}
	return lang
}

func minLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// IsSupported returns true if the language is considered a code language (not markdown/json etc).
func IsSupported(lang Language) bool {
	switch lang {
	case LanguageGo, LanguagePython, LanguageJavaScript, LanguageTypeScript, LanguageJava, LanguageC, LanguageCpp, LanguageRust, LanguageRuby, LanguagePHP, LanguageCSharp, LanguageShell:
		return true
	default:
		return false
	}
}

// AllLanguages returns list of supported languages.
func AllLanguages() []Language {
	return []Language{LanguageGo, LanguagePython, LanguageJavaScript, LanguageTypeScript, LanguageJava, LanguageC, LanguageCpp, LanguageRust, LanguageRuby, LanguagePHP, LanguageCSharp, LanguageShell}
}

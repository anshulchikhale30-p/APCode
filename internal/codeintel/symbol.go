package codeintel

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
)

// SymbolKind classifies a symbol.
type SymbolKind string

const (
	KindFunction  SymbolKind = "function"
	KindClass     SymbolKind = "class"
	KindStruct    SymbolKind = "struct"
	KindInterface SymbolKind = "interface"
	KindMethod    SymbolKind = "method"
	KindVariable  SymbolKind = "variable"
	KindConstant  SymbolKind = "constant"
	KindField     SymbolKind = "field"
	KindImport    SymbolKind = "import"
	KindPackage   SymbolKind = "package"
	KindEnum      SymbolKind = "enum"
	KindType      SymbolKind = "type"
)

// Symbol is a named entity found in source code.
type Symbol struct {
	Name     string     `json:"name"`
	Kind     SymbolKind `json:"kind"`
	Path     string     `json:"path"`
	Line     int        `json:"line"`
	Column   int        `json:"column"`
	Language Language   `json:"language"`
	// Signature or line content for preview.
	Signature string `json:"signature,omitempty"`
	// Parent for methods (e.g., class/struct name)
	Parent string `json:"parent,omitempty"`
}

// languagePatterns holds regexes for symbol discovery per language.
type langPattern struct {
	kind        SymbolKind
	regex       *regexp.Regexp
	group       int // capture group for name
	parentGroup int // optional
}

var (
	goPatterns = []langPattern{
		{KindFunction, regexp.MustCompile(`(?m)^\s*func\s+(?:\(\s*\w+\s+[^)]+\)\s+)?(\w+)\s*\(`), 1, -1},
		{KindMethod, regexp.MustCompile(`(?m)^\s*func\s+\(\s*\w+\s+[^)]+\)\s+(\w+)\s*\(`), 1, -1},
		{KindStruct, regexp.MustCompile(`(?m)^\s*type\s+(\w+)\s+struct\b`), 1, -1},
		{KindInterface, regexp.MustCompile(`(?m)^\s*type\s+(\w+)\s+interface\b`), 1, -1},
		{KindType, regexp.MustCompile(`(?m)^\s*type\s+(\w+)\s+[^\s]+\s*$`), 1, -1},
		{KindConstant, regexp.MustCompile(`(?m)^\s*const\s+(\w+)\b`), 1, -1},
		{KindVariable, regexp.MustCompile(`(?m)^\s*var\s+(\w+)\b`), 1, -1},
	}
	pythonPatterns = []langPattern{
		{KindFunction, regexp.MustCompile(`(?m)^\s*def\s+(\w+)\s*\(`), 1, -1},
		{KindClass, regexp.MustCompile(`(?m)^\s*class\s+(\w+)\s*[\(:]`), 1, -1},
		{KindVariable, regexp.MustCompile(`(?m)^\s*(\w+)\s*=\s*[^=\n]+$`), 1, -1},
	}
	jsPatterns = []langPattern{
		{KindFunction, regexp.MustCompile(`(?m)(?:function\s+(\w+)\s*\(|(\w+)\s*:\s*function\s*\(|const\s+(\w+)\s*=\s*(?:async\s*)?function\b)`), 1, -1},
		{KindClass, regexp.MustCompile(`(?m)^\s*class\s+(\w+)\b`), 1, -1},
		{KindFunction, regexp.MustCompile(`(?m)^\s*(?:async\s+)?(\w+)\s*\([^)]*\)\s*\{\s*$`), 0, -1}, // generic
		{KindVariable, regexp.MustCompile(`(?m)^\s*(?:const|let|var)\s+(\w+)\s*=\s*[^;]+`), 1, -1},
	}
	javaPatterns = []langPattern{
		{KindClass, regexp.MustCompile(`(?m)^\s*(?:public\s+|private\s+|protected\s+)?(?:abstract\s+)?class\s+(\w+)\b`), 1, -1},
		{KindInterface, regexp.MustCompile(`(?m)^\s*(?:public\s+)?interface\s+(\w+)\b`), 1, -1},
		{KindEnum, regexp.MustCompile(`(?m)^\s*(?:public\s+)?enum\s+(\w+)\b`), 1, -1},
		{KindMethod, regexp.MustCompile(`(?m)^\s*(?:public|private|protected)?\s*(?:static\s+)?(?:\w+\s+)+(\w+)\s*\([^)]*\)\s*(?:throws[^{]+)?\{`), 1, -1},
		{KindFunction, regexp.MustCompile(`(?m)^\s*(?:public|private|protected)?\s*(?:static\s+)?\w+\s+(\w+)\s*\(`), 1, -1},
	}
	cPatterns = []langPattern{
		{KindFunction, regexp.MustCompile(`(?m)^\s*(?:\w+\s+)+(\w+)\s*\([^;]*\)\s*\{`), 1, -1},
		{KindStruct, regexp.MustCompile(`(?m)^\s*struct\s+(\w+)\b`), 1, -1},
		{KindType, regexp.MustCompile(`(?m)^\s*typedef\s+struct[^;]*\}\s+(\w+)\s*;`), 1, -1},
	}
	rustPatterns = []langPattern{
		{KindFunction, regexp.MustCompile(`(?m)^\s*(?:pub\s+)?fn\s+(\w+)\s*\(`), 1, -1},
		{KindStruct, regexp.MustCompile(`(?m)^\s*(?:pub\s+)?struct\s+(\w+)\b`), 1, -1},
		{KindInterface, regexp.MustCompile(`(?m)^\s*(?:pub\s+)?trait\s+(\w+)\b`), 1, -1},
		{KindEnum, regexp.MustCompile(`(?m)^\s*(?:pub\s+)?enum\s+(\w+)\b`), 1, -1},
	}
	rubyPatterns = []langPattern{
		{KindFunction, regexp.MustCompile(`(?m)^\s*def\s+(\w+)\b`), 1, -1},
		{KindClass, regexp.MustCompile(`(?m)^\s*class\s+(\w+)\b`), 1, -1},
		{KindConstant, regexp.MustCompile(`(?m)^\s*module\s+(\w+)\b`), 1, -1},
	}
	phpPatterns = []langPattern{
		{KindFunction, regexp.MustCompile(`(?m)^\s*function\s+(\w+)\s*\(`), 1, -1},
		{KindClass, regexp.MustCompile(`(?m)^\s*class\s+(\w+)\b`), 1, -1},
		{KindInterface, regexp.MustCompile(`(?m)^\s*interface\s+(\w+)\b`), 1, -1},
	}
	csharpPatterns = []langPattern{
		{KindClass, regexp.MustCompile(`(?m)^\s*(?:public|private|protected|internal)?\s*(?:abstract|sealed)?\s*class\s+(\w+)\b`), 1, -1},
		{KindInterface, regexp.MustCompile(`(?m)^\s*(?:public|private)?\s*interface\s+(\w+)\b`), 1, -1},
		{KindFunction, regexp.MustCompile(`(?m)^\s*(?:public|private|protected|internal)?\s*(?:static\s+)?(?:\w+\s+)+(\w+)\s*\(`), 1, -1},
	}
)

// patternsFor returns patterns for a language.
func patternsFor(lang Language) []langPattern {
	switch lang {
	case LanguageGo:
		return goPatterns
	case LanguagePython:
		return pythonPatterns
	case LanguageJavaScript, LanguageTypeScript:
		return jsPatterns
	case LanguageJava:
		return javaPatterns
	case LanguageC, LanguageCpp:
		return cPatterns
	case LanguageRust:
		return rustPatterns
	case LanguageRuby:
		return rubyPatterns
	case LanguagePHP:
		return phpPatterns
	case LanguageCSharp:
		return csharpPatterns
	default:
		return nil
	}
}

// DiscoverSymbols finds symbols in content for a given language and file path.
// Lightweight regex-based; no LLM needed.
func DiscoverSymbols(path string, lang Language, content []byte) []Symbol {
	patterns := patternsFor(lang)
	if len(patterns) == 0 {
		// Generic fallback: look for function-like definitions
		return discoverGeneric(path, lang, content)
	}
	var symbols []Symbol
	lines := bytes.Split(content, []byte{'\n'})
	// Deduplicate by line+name
	seen := make(map[string]bool)
	for _, pat := range patterns {
		matches := pat.regex.FindAllStringSubmatch(string(content), -1)
		// We need line numbers; easiest is to scan lines and apply per line
		// But for now, use FindAllSubmatchIndex to get position then compute line.
		indices := pat.regex.FindAllStringSubmatchIndex(string(content), -1)
		for _, idx := range indices {
			// idx contains pairs for entire match and groups
			// Extract name from groups
			var name string
			var col int
			// Determine which group actually matched (for jsPatterns with alternations)
			// Find first non-empty group among group..n
			if pat.group < len(idx)/2*2 {
				// Map group index to string index pair: group*2, group*2+1
				start := idx[pat.group*2]
				end := idx[pat.group*2+1]
				if start >= 0 && end >= 0 {
					name = string(content[start:end])
					col = start - bytes.LastIndex(content[:start], []byte{'\n'}) - 1
				}
			}
			// For patterns with multiple alternatives where group -1 or special handling
			if name == "" {
				// Try all groups
				for g := 1; g < len(idx)/2; g++ {
					s := idx[g*2]
					e := idx[g*2+1]
					if s >= 0 && e >= 0 {
						candidate := strings.TrimSpace(string(content[s:e]))
						if candidate != "" {
							name = candidate
							col = s - bytes.LastIndex(content[:s], []byte{'\n'}) - 1
							break
						}
					}
				}
			}
			if name == "" {
				continue
			}
			// Skip common false positives
			if isKeyword(name) {
				continue
			}
			// Compute line number
			pos := idx[0]
			line := bytes.Count(content[:pos], []byte{'\n'}) + 1
			// Get signature line
			sig := ""
			if line-1 < len(lines) {
				sig = strings.TrimSpace(string(lines[line-1]))
				if len(sig) > 120 {
					sig = sig[:120]
				}
			}
			key := name + ":" + langStr(pat.kind) + ":" + string(rune(line))
			_ = key
			dupKey := name + "|" + string(pat.kind) + "|" + string(rune(line))
			if seen[dupKey] {
				continue
			}
			// More robust dedup
			dedupKey := name + "\x00" + string(pat.kind) + "\x00" + itoa(line)
			if seen[dedupKey] {
				continue
			}
			seen[dedupKey] = true

			// Determine parent for methods (simplified: look back for class)
			parent := ""
			if pat.kind == KindMethod || pat.kind == KindFunction {
				parent = findParentClass(content, pos)
			}
			if col < 0 {
				col = 0
			}
			symbols = append(symbols, Symbol{
				Name:      name,
				Kind:      pat.kind,
				Path:      path,
				Line:      line,
				Column:    col + 1,
				Language:  lang,
				Signature: sig,
				Parent:    parent,
			})
		}
		_ = matches
	}
	if len(symbols) == 0 {
		// Fallback generic
		gen := discoverGeneric(path, lang, content)
		symbols = append(symbols, gen...)
	}
	return symbols
}

func discoverGeneric(path string, lang Language, content []byte) []Symbol {
	// Very lightweight: find words before '(' that look like functions
	re := regexp.MustCompile(`(?m)^\s*(?:func|def|function|class)\s+(\w+)`)
	var out []Symbol
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		m := re.FindStringSubmatch(line)
		if len(m) >= 2 {
			name := m[1]
			if isKeyword(name) {
				continue
			}
			kind := KindFunction
			if strings.Contains(line, "class") {
				kind = KindClass
			}
			out = append(out, Symbol{
				Name:      name,
				Kind:      kind,
				Path:      path,
				Line:      lineNo,
				Column:    strings.Index(line, name) + 1,
				Language:  lang,
				Signature: strings.TrimSpace(line),
			})
		}
	}
	return out
}

func isKeyword(s string) bool {
	switch s {
	case "if", "for", "while", "else", "return", "import", "package", "var", "const", "type", "func", "def", "class", "function", "let":
		return true
	}
	return false
}

func langStr(k SymbolKind) string { return string(k) }

func itoa(n int) string {
	// simple int to string without fmt
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 10)
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func findParentClass(content []byte, pos int) string {
	// Look backward up to 1000 bytes for class/struct definition
	start := pos - 1000
	if start < 0 {
		start = 0
	}
	snip := string(content[start:pos])
	// Find last class occurrence
	re := regexp.MustCompile(`(?m)class\s+(\w+)`)
	matches := re.FindAllStringSubmatch(snip, -1)
	if len(matches) > 0 {
		return matches[len(matches)-1][1]
	}
	re2 := regexp.MustCompile(`(?m)type\s+(\w+)\s+struct`)
	m2 := re2.FindAllStringSubmatch(snip, -1)
	if len(m2) > 0 {
		return m2[len(m2)-1][1]
	}
	return ""
}

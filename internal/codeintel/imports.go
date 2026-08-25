package codeintel

import (
	"bufio"
	"bytes"
	"regexp"
	"strings"
)

// ImportInfo represents a single import/dependency.
type ImportInfo struct {
	Path       string `json:"path"`        // file containing import
	ImportPath string `json:"import_path"` // imported package/module
	Line       int    `json:"line"`
	Alias      string `json:"alias,omitempty"`
	Raw        string `json:"raw,omitempty"`
}

// DiscoverImports extracts import statements for a file.
func DiscoverImports(path string, lang Language, content []byte) []ImportInfo {
	var out []ImportInfo
	switch lang {
	case LanguageGo:
		out = append(out, discoverGoImports(path, content)...)
	case LanguagePython:
		out = append(out, discoverPythonImports(path, content)...)
	case LanguageJavaScript, LanguageTypeScript:
		out = append(out, discoverJSImports(path, content)...)
	case LanguageJava:
		out = append(out, discoverJavaImports(path, content)...)
	case LanguageC, LanguageCpp:
		out = append(out, discoverCImports(path, content)...)
	case LanguageRust:
		out = append(out, discoverRustImports(path, content)...)
	case LanguageRuby:
		out = append(out, discoverRubyImports(path, content)...)
	case LanguagePHP:
		out = append(out, discoverPHPImports(path, content)...)
	default:
		// Generic: look for import-like lines
		out = append(out, discoverGenericImports(path, content)...)
	}
	return out
}

func discoverGoImports(path string, content []byte) []ImportInfo {
	var out []ImportInfo
	// Single line: import "fmt" or import alias "fmt"
	reSingle := regexp.MustCompile(`(?m)^\s*import\s+(?:(\w+)\s+)?"([^"]+)"`)
	// Multi-line block: import ( ... )
	reBlock := regexp.MustCompile(`(?s)import\s*\(\s*([^)]+)\s*\)`)
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, `import "`) || strings.Contains(trim, `import `) {
			m := reSingle.FindStringSubmatch(line)
			if len(m) >= 3 {
				alias := m[1]
				imp := m[2]
				if imp == "" {
					imp = m[1]
					alias = ""
				}
				if imp != "" {
					out = append(out, ImportInfo{Path: path, ImportPath: imp, Line: lineNo, Alias: alias, Raw: strings.TrimSpace(line)})
				}
				continue
			}
			// Inside block: line with "path"
			m2 := regexp.MustCompile(`"([^"]+)"`).FindStringSubmatch(line)
			if len(m2) >= 2 && strings.Contains(line, `"`) {
				// Only if within import block context – we approximate by seeing if file contains import (
				// Simpler: just collect if line contains quoted import
				if bytes.Contains(content, []byte("import (")) {
					out = append(out, ImportInfo{Path: path, ImportPath: m2[1], Line: lineNo, Raw: strings.TrimSpace(line)})
				}
			}
		}
	}
	// Also handle block via regex for completeness (dedupe)
	if m := reBlock.FindSubmatch(content); len(m) >= 2 {
		block := string(m[1])
		reQuoted := regexp.MustCompile(`"([^"]+)"`)
		for _, qm := range reQuoted.FindAllStringSubmatch(block, -1) {
			imp := qm[1]
			// Find line number of this import in original content
			line := findLineForImport(content, imp)
			// Avoid duplicate
			exists := false
			for _, e := range out {
				if e.ImportPath == imp {
					exists = true
					break
				}
			}
			if !exists {
				out = append(out, ImportInfo{Path: path, ImportPath: imp, Line: line, Raw: `"` + imp + `"`})
			}
		}
	}
	return out
}

func discoverPythonImports(path string, content []byte) []ImportInfo {
	var out []ImportInfo
	reImport := regexp.MustCompile(`(?m)^\s*import\s+([^\n#]+)`)
	reFrom := regexp.MustCompile(`(?m)^\s*from\s+(\S+)\s+import\s+([^\n#]+)`)
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if m := reFrom.FindStringSubmatch(line); len(m) >= 3 {
			out = append(out, ImportInfo{Path: path, ImportPath: strings.TrimSpace(m[1]), Line: lineNo, Raw: strings.TrimSpace(line)})
			continue
		}
		if m := reImport.FindStringSubmatch(line); len(m) >= 2 {
			parts := strings.Split(m[1], ",")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				// Handle "import x as y"
				p = strings.Split(p, " as ")[0]
				p = strings.TrimSpace(p)
				if p != "" {
					out = append(out, ImportInfo{Path: path, ImportPath: p, Line: lineNo, Raw: strings.TrimSpace(line)})
				}
			}
		}
	}
	return out
}

func discoverJSImports(path string, content []byte) []ImportInfo {
	var out []ImportInfo
	reES := regexp.MustCompile(`(?m)^\s*import\s+.*from\s+['"]([^'"]+)['"]`)
	reES2 := regexp.MustCompile(`(?m)^\s*import\s+['"]([^'"]+)['"]`)
	reRequire := regexp.MustCompile(`require\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	reDynamic := regexp.MustCompile(`import\s*\(\s*['"]([^'"]+)['"]\s*\)`)
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if m := reES.FindStringSubmatch(line); len(m) >= 2 {
			out = append(out, ImportInfo{Path: path, ImportPath: m[1], Line: lineNo, Raw: strings.TrimSpace(line)})
			continue
		}
		if m := reES2.FindStringSubmatch(line); len(m) >= 2 {
			out = append(out, ImportInfo{Path: path, ImportPath: m[1], Line: lineNo, Raw: strings.TrimSpace(line)})
			continue
		}
		if matches := reRequire.FindAllStringSubmatch(line, -1); len(matches) > 0 {
			for _, mm := range matches {
				out = append(out, ImportInfo{Path: path, ImportPath: mm[1], Line: lineNo, Raw: strings.TrimSpace(line)})
			}
			continue
		}
		if m := reDynamic.FindStringSubmatch(line); len(m) >= 2 {
			out = append(out, ImportInfo{Path: path, ImportPath: m[1], Line: lineNo, Raw: strings.TrimSpace(line)})
		}
	}
	return out
}

func discoverJavaImports(path string, content []byte) []ImportInfo {
	var out []ImportInfo
	re := regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([a-zA-Z0-9_.*]+)\s*;`)
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if m := re.FindStringSubmatch(line); len(m) >= 2 {
			out = append(out, ImportInfo{Path: path, ImportPath: strings.TrimSpace(m[1]), Line: lineNo, Raw: strings.TrimSpace(line)})
		}
	}
	return out
}

func discoverCImports(path string, content []byte) []ImportInfo {
	var out []ImportInfo
	re := regexp.MustCompile(`(?m)^\s*#\s*include\s+[<"]([^>"]+)[>"]`)
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if m := re.FindStringSubmatch(line); len(m) >= 2 {
			out = append(out, ImportInfo{Path: path, ImportPath: m[1], Line: lineNo, Raw: strings.TrimSpace(line)})
		}
	}
	return out
}

func discoverRustImports(path string, content []byte) []ImportInfo {
	var out []ImportInfo
	reUse := regexp.MustCompile(`(?m)^\s*use\s+([a-zA-Z0-9_:*]+)\s*;`)
	reMod := regexp.MustCompile(`(?m)^\s*mod\s+(\w+)\s*;`)
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if m := reUse.FindStringSubmatch(line); len(m) >= 2 {
			out = append(out, ImportInfo{Path: path, ImportPath: m[1], Line: lineNo, Raw: strings.TrimSpace(line)})
		} else if m := reMod.FindStringSubmatch(line); len(m) >= 2 {
			out = append(out, ImportInfo{Path: path, ImportPath: m[1], Line: lineNo, Raw: strings.TrimSpace(line)})
		}
	}
	return out
}

func discoverRubyImports(path string, content []byte) []ImportInfo {
	var out []ImportInfo
	re := regexp.MustCompile(`(?m)^\s*require\s+['"]([^'"]+)['"]`)
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if m := re.FindStringSubmatch(line); len(m) >= 2 {
			out = append(out, ImportInfo{Path: path, ImportPath: m[1], Line: lineNo, Raw: strings.TrimSpace(line)})
		}
	}
	return out
}

func discoverPHPImports(path string, content []byte) []ImportInfo {
	var out []ImportInfo
	reUse := regexp.MustCompile(`(?m)^\s*use\s+([a-zA-Z0-9_\\]+)\s*;`)
	reRequire := regexp.MustCompile(`(?m)^\s*(?:require|include)(?:_once)?\s*\(?\s*['"]([^'"]+)['"]`)
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if m := reUse.FindStringSubmatch(line); len(m) >= 2 {
			out = append(out, ImportInfo{Path: path, ImportPath: m[1], Line: lineNo, Raw: strings.TrimSpace(line)})
		} else if m := reRequire.FindStringSubmatch(line); len(m) >= 2 {
			out = append(out, ImportInfo{Path: path, ImportPath: m[1], Line: lineNo, Raw: strings.TrimSpace(line)})
		}
	}
	return out
}

func discoverGenericImports(path string, content []byte) []ImportInfo {
	var out []ImportInfo
	re := regexp.MustCompile(`(?m)^\s*import\s+([^\n;]+)`)
	scanner := bufio.NewScanner(bytes.NewReader(content))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if m := re.FindStringSubmatch(line); len(m) >= 2 {
			out = append(out, ImportInfo{Path: path, ImportPath: strings.TrimSpace(m[1]), Line: lineNo, Raw: strings.TrimSpace(line)})
		}
	}
	return out
}

func findLineForImport(content []byte, imp string) int {
	lines := bytes.Split(content, []byte{'\n'})
	for i, l := range lines {
		if bytes.Contains(l, []byte(imp)) {
			return i + 1
		}
	}
	return 1
}

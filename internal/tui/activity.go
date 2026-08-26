package tui

import (
	"fmt"
	"strings"
)

// ActivityKind is the semantic state of one agent-activity line.
type ActivityKind int

const (
	ActivityWorking ActivityKind = iota // ◐ in progress
	ActivitySuccess                     // ✓ done
	ActivityWarning                     // ⚠ attention
	ActivityError                       // ✗ failure
	ActivityAction                      // → action taken
)

// ActivityLine renders one semantic activity line, e.g. "  ✓ Found 24 files".
func ActivityLine(kind ActivityKind, msg string) string {
	switch kind {
	case ActivitySuccess:
		return "  " + Success(GlyphSuccess+" "+msg)
	case ActivityWarning:
		return "  " + Warning(GlyphWarning+" "+msg)
	case ActivityError:
		return "  " + Error(GlyphError+" "+msg)
	case ActivityAction:
		return "  " + Secondary(GlyphAction+" "+msg)
	default:
		return "  " + Accent(GlyphWorking+" "+msg)
	}
}

// FileChange renders a modified-file marker: "✎ src/auth/session.py".
func FileChange(path string) string {
	return "  " + Warning(GlyphEdit) + " " + Info(path)
}

// DiffLine styles a single unified-diff line: additions green, deletions
// red, hunk headers muted. This is the seam where a full diff renderer can
// later be added without touching callers.
func DiffLine(line string) string {
	switch {
	case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
		return Success(line)
	case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
		return Error(line)
	case strings.HasPrefix(line, "@@"):
		return Accent(line)
	default:
		return Muted(line)
	}
}

// RenderDiff styles a whole unified-diff text block.
func RenderDiff(diff string) string {
	lines := strings.Split(strings.TrimRight(diff, "\n"), "\n")
	for i, l := range lines {
		lines[i] = DiffLine(l)
	}
	return strings.Join(lines, "\n")
}

// RuleWidth returns the divider width for a terminal width.
func RuleWidth(termWidth int) int {
	if termWidth <= 0 {
		termWidth = defaultTermWidth
	}
	return ClampWidth(termWidth-6, 24, 62)
}

// Divider renders a horizontal rule spanning the given columns.
func Divider(width int) string {
	if width < 4 {
		width = 4
	}
	return Muted(strings.Repeat("─", width))
}

// ResponseBlock renders an assistant response visually separated from tool
// activity: divider rules above and below, text indented and word-wrapped.
func ResponseBlock(response string, termWidth int) string {
	rw := RuleWidth(termWidth)
	var b strings.Builder
	b.WriteString(Divider(rw))
	b.WriteByte('\n')
	for _, line := range WrapText(strings.TrimSpace(response), rw-2) {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString(Divider(rw))
	return b.String()
}

// WrapText greedily word-wraps s to at most width visible columns. Words
// longer than width are not broken. Existing newlines are preserved.
func WrapText(s string, width int) []string {
	if width < 10 {
		width = 10
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		cur := ""
		for _, w := range words {
			switch {
			case cur == "":
				cur = w
			case visibleWidth(cur)+1+visibleWidth(w) <= width:
				cur += " " + w
			default:
				out = append(out, cur)
				cur = w
			}
		}
		out = append(out, cur)
	}
	return out
}

// ToolSummary renders the compact per-tool activity line used while the
// agent works: "→ read_file path=main.go".
func ToolSummary(name string, input map[string]string) string {
	keys := make([]string, 0, len(input))
	for k := range input {
		keys = append(keys, k)
	}
	sortStrings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := input[k]
		if len(v) > 40 {
			v = v[:37] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	arg := strings.Join(parts, " ")
	line := name
	if arg != "" {
		line += " " + arg
	}
	return "  " + Secondary(GlyphAction+" "+line)
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

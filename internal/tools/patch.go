package tools

import (
	"fmt"
	"strings"
)

// PatchLine is a single line of a unified diff hunk.
type PatchLine struct {
	Type byte   // ' ', '+', '-', or '\'
	Text string // content without the leading type character
}

// Hunk is one @@-delimited section of a unified diff.
type Hunk struct {
	OldStart int
	OldCount int
	NewStart int
	NewCount int
	Lines    []PatchLine
}

// FilePatch is a parsed patch for a single file.
type FilePatch struct {
	Path  string // target path from +++ line (a/ or b/ prefix stripped)
	Hunks []Hunk
}

// ParseUnifiedDiff parses a unified diff (as produced by git diff) into file
// patches. It supports multiple files per diff, \ No newline markers, and
// rejects malformed hunks.
func ParseUnifiedDiff(diff string) ([]FilePatch, error) {
	var patches []FilePatch
	var cur *FilePatch
	lines := strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "--- "):
			// old path; wait for +++
		case strings.HasPrefix(line, "+++ "):
			path := extractPatchPath(line[4:])
			if path == "" {
				return nil, NewToolError(CodeInvalidInput, "apply_patch: empty target path in +++ line", nil)
			}
			patches = append(patches, FilePatch{Path: path})
			cur = &patches[len(patches)-1]
		case strings.HasPrefix(line, "@@"):
			if cur == nil {
				return nil, NewToolError(CodeInvalidInput, "apply_patch: hunk found before file header", nil)
			}
			h, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			// Collect hunk body.
			i++
			for i < len(lines) {
				l := lines[i]
				if l == "" && i == len(lines)-1 {
					break
				}
				if strings.HasPrefix(l, "@@") || strings.HasPrefix(l, "--- ") || strings.HasPrefix(l, "+++ ") || strings.HasPrefix(l, "diff ") {
					i--
					break
				}
				if l == "" {
					// Empty context line (space stripped by some tools).
					h.Lines = append(h.Lines, PatchLine{Type: ' ', Text: ""})
					i++
					continue
				}
				t := l[0]
				if t != ' ' && t != '+' && t != '-' && t != '\\' {
					return nil, NewToolError(CodeInvalidInput, fmt.Sprintf("apply_patch: unexpected line in hunk: %q", truncatePreview(l, 60)), nil)
				}
				h.Lines = append(h.Lines, PatchLine{Type: t, Text: l[1:]})
				i++
			}
			if err := validateHunk(h); err != nil {
				return nil, err
			}
			cur.Hunks = append(cur.Hunks, *h)
		default:
			// diff --git, index, mode lines etc. are ignored.
		}
	}
	if len(patches) == 0 {
		return nil, NewToolError(CodeInvalidInput, "apply_patch: no file patches found in diff", nil)
	}
	for _, p := range patches {
		if len(p.Hunks) == 0 {
			return nil, NewToolError(CodeInvalidInput, fmt.Sprintf("apply_patch: patch for %q contains no hunks", p.Path), nil)
		}
	}
	return patches, nil
}

// ApplyFilePatch applies a parsed FilePatch to original content, verifying
// that context lines match. It returns the patched content.
func ApplyFilePatch(original string, fp FilePatch) (string, error) {
	eol := "\n"
	if strings.Count(original, "\r\n") > strings.Count(original, "\n")/2 {
		eol = "\r\n"
	}
	norm := func(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }
	src := strings.Split(norm(original), "\n")

	out := make([]string, 0, len(src)+64)
	pos := 0 // 0-based index into src of next unconsumed line

	ensureEOL := func(s string) string {
		if eol == "\n" {
			return s
		}
		return s
	}
	_ = ensureEOL

	for _, h := range fp.Hunks {
		start := h.OldStart - 1 // to 0-based
		if start < 0 {
			start = pos
		}
		// Locate hunk: try expected position first, then scan forward.
		appliedAt := -1
		for at := start; at <= len(src); at++ {
			if matchesContext(src, at, h) {
				appliedAt = at
				break
			}
		}
		if appliedAt == -1 {
			// Also scan backward slightly for drifted offsets.
			for at := start - 1; at >= 0; at-- {
				if matchesContext(src, at, h) {
					appliedAt = at
					break
				}
			}
		}
		if appliedAt == -1 {
			return "", NewToolError(CodeInvalidInput,
				fmt.Sprintf("apply_patch: context does not match file for hunk @@ -%d,%d +%d,%d @@ (file may have changed since the patch was generated)", h.OldStart, h.OldCount, h.NewStart, h.NewCount), nil)
		}
		// Copy unchanged lines up to the hunk.
		out = append(out, src[pos:appliedAt]...)
		pos = appliedAt
		// Emit new lines from hunk.
		for _, pl := range h.Lines {
			switch pl.Type {
			case ' ':
				out = append(out, pl.Text)
				pos++
			case '-':
				pos++
			case '+':
				out = append(out, pl.Text)
			case '\\':
				// "\ No newline at end of file" marker: nothing structural to do.
			}
		}
	}
	out = append(out, src[pos:]...)
	result := strings.Join(out, "\n")
	if eol == "\r\n" && strings.Contains(result, "\n") && !strings.Contains(result, "\r\n") {
		result = strings.ReplaceAll(result, "\n", "\r\n")
	}
	return result, nil
}

// matchesContext reports whether hunk h's old-side lines match src starting
// at index at (0-based).
func matchesContext(src []string, at int, h Hunk) bool {
	i := at
	for _, pl := range h.Lines {
		switch pl.Type {
		case ' ':
			if i >= len(src) || src[i] != pl.Text {
				return false
			}
			i++
		case '-':
			if i >= len(src) || src[i] != pl.Text {
				return false
			}
			i++
		case '+':
			// new-only line: no source consumption
		case '\\':
			// ignore marker
		}
	}
	return true
}

// parseHunkHeader parses "@@ -l,c +l,c @@".
func parseHunkHeader(line string) (*Hunk, error) {
	var h Hunk
	rest := strings.TrimSpace(strings.TrimPrefix(line, "@@"))
	end := strings.Index(rest, "@@")
	if end == -1 {
		return nil, NewToolError(CodeInvalidInput, fmt.Sprintf("apply_patch: malformed hunk header %q", truncatePreview(line, 40)), nil)
	}
	ranges := strings.Fields(rest[:end])
	parseOne := func(s string) (int, int, bool) {
		s = strings.TrimPrefix(s, "-")
		s = strings.TrimPrefix(s, "+")
		start, count := 1, 1
		if i := strings.Index(s, ","); i != -1 {
			if n, err := fmt.Sscanf(s[:i]+","+s[i+1:], "%d,%d", &start, &count); err != nil || n != 2 {
				return 0, 0, false
			}
		} else {
			if n, err := fmt.Sscanf(s, "%d", &start); err != nil || n != 1 {
				return 0, 0, false
			}
		}
		return start, count, true
	}
	ok := len(ranges) >= 2
	if ok {
		var v1, v2 bool
		h.OldStart, h.OldCount, v1 = parseOne(ranges[0])
		h.NewStart, h.NewCount, v2 = parseOne(ranges[1])
		ok = v1 && v2
	}
	if !ok {
		return nil, NewToolError(CodeInvalidInput, fmt.Sprintf("apply_patch: malformed hunk header %q", truncatePreview(line, 40)), nil)
	}
	return &h, nil
}

// validateHunk checks declared counts against actual line composition.
func validateHunk(h *Hunk) error {
	oldN, newN := 0, 0
	for _, pl := range h.Lines {
		switch pl.Type {
		case ' ':
			oldN++
			newN++
		case '-':
			oldN++
		case '+':
			newN++
		}
	}
	// Some generators omit counts; only enforce when explicitly non-default.
	if (h.OldCount != 1 || oldN != 1) && h.OldCount != 0 && oldN != h.OldCount {
		return NewToolError(CodeInvalidInput, fmt.Sprintf("apply_patch: hunk header declares -%d,%d but body has %d old lines", h.OldStart, h.OldCount, oldN), nil)
	}
	if (h.NewCount != 1 || newN != 1) && h.NewCount != 0 && newN != h.NewCount {
		return NewToolError(CodeInvalidInput, fmt.Sprintf("apply_patch: hunk header declares +%d,%d but body has %d new lines", h.NewStart, h.NewCount, newN), nil)
	}
	return nil
}

// extractPatchPath strips a/ b/ prefixes and quotes from a diff path.
func extractPatchPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "\"")
	p = strings.TrimSuffix(p, "\t")
	for _, prefix := range []string{"a/", "b/"} {
		if strings.HasPrefix(p, prefix) {
			p = p[len(prefix):]
			break
		}
	}
	return strings.TrimSpace(p)
}

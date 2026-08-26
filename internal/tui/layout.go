package tui

import (
	"strings"
	"unicode/utf8"
)

// ansiEscape is used to strip ANSI sequences when measuring visible width.
const ansiEscape = "\x1b"

// stripANSI removes all ANSI escape sequences from s so its visible width
// can be measured.
func stripANSI(s string) string {
	if !strings.Contains(s, ansiEscape) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	inSeq := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inSeq = true
		case inSeq && (r == 'm' || r == 'K' || r == 'H' || r == 'J'):
			inSeq = false
		case !inSeq:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// visibleWidth returns the number of display columns of s, ignoring ANSI
// styling.
func visibleWidth(s string) int {
	return utf8.RuneCountInString(stripANSI(s))
}

// Header renders a consistent section header:
//
//	◆ Title ──────────────────────────────
func Header(title string) string {
	const target = 42
	dashes := target - utf8.RuneCountInString(title)
	if dashes < 4 {
		dashes = 4
	}
	return Success("◆ ") + Bold(title) + " " + Muted(strings.Repeat("─", dashes))
}

// Rule renders a muted horizontal divider line.
func Rule() string {
	return Muted(strings.Repeat("─", 46))
}

// Box renders lines inside a rounded border with an optional embedded title,
// like:
//
//	╭─ System ─────────────────────╮
//	│ Operating system : testos    │
//	╰──────────────────────────────╯
//
// ANSI styling in lines is preserved but not counted toward width. Every
// rendered row is exactly inner+2 columns wide.
func Box(title string, lines []string) string {
	const minInner = 32
	maxLine := 0
	for _, l := range lines {
		if w := visibleWidth(l); w > maxLine {
			maxLine = w
		}
	}
	// inner counts all columns between the left and right corner glyphs.
	inner := maxLine + 2 // one space of padding on each side
	if inner < minInner {
		inner = minInner
	}
	titleText := ""
	if title != "" {
		titleText = "─ " + title + " "
		if tw := utf8.RuneCountInString(titleText); tw > inner {
			inner = tw
		}
	}

	var b strings.Builder
	b.WriteString("╭")
	b.WriteString(titleText)
	b.WriteString(Muted(strings.Repeat("─", inner-utf8.RuneCountInString(titleText))))
	b.WriteString("╮")
	b.WriteByte('\n')

	for _, l := range lines {
		b.WriteString(Muted("│"))
		b.WriteString(" ")
		b.WriteString(l)
		b.WriteString(strings.Repeat(" ", inner-2-visibleWidth(l)))
		b.WriteString(" ")
		b.WriteString(Muted("│"))
		b.WriteByte('\n')
	}

	b.WriteString("╰")
	b.WriteString(strings.Repeat("─", inner))
	b.WriteString("╯")
	return b.String()
}

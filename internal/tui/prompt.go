package tui

import (
	"strings"
)

// Input box geometry. The interactive REPL prints the top border and the
// prompt prefix, lets the terminal echo the user's typing inline (so all
// normal editing keys keep working), and closes the border after Enter.

// BoxWidth returns the input box width for a given terminal width,
// clamped so the box always fits.
func BoxWidth(termWidth int) int {
	if termWidth <= 0 {
		termWidth = defaultTermWidth
	}
	w := termWidth - 4
	return ClampWidth(w, 24, 76)
}

// InputBoxTop renders the top border of the input box.
func InputBoxTop(width int) string {
	if width < 6 {
		width = 6
	}
	return Border("╭" + strings.Repeat("─", width-2) + "╮")
}

// InputBoxPrefix renders the left border and prompt mark. The cursor sits
// immediately after it, ready for typed input.
func InputBoxPrefix() string {
	return Border("│") + "  " + Accent("›") + " "
}

// InputBoxBottom renders the bottom border of the input box.
func InputBoxBottom(width int) string {
	if width < 6 {
		width = 6
	}
	return Border("╰" + strings.Repeat("─", width-2) + "╯")
}

// InputBox renders a closed preview of the input box with the given text
// inside. Used by tests, help screens, and non-interactive rendering.
func InputBox(width int, text string) string {
	var b strings.Builder
	b.WriteString(InputBoxTop(width))
	b.WriteByte('\n')
	for _, line := range strings.Split(text, "\n") {
		line = truncateVisible(line, width-6)
		pad := width - 6 - visibleWidth(line)
		if pad < 0 {
			pad = 0
		}
		b.WriteString(Border("│") + "  " + Accent("› ") + line + strings.Repeat(" ", pad) + Border("│"))
		b.WriteByte('\n')
	}
	b.WriteString(InputBoxBottom(width))
	return b.String()
}

package tui

import (
	"strings"
)

// compactLogo is the small APCode mark used on narrow terminals.
const compactLogo = ` █▀▀ █▀▀█ █▀▀▄ █▀▀ █▀▀ █▀▀▄
 █   █  █ █  █ █▀▀ ▀▀█ █▀▀▄
 ▀▀▀ ▀▀▀▀ ▀  ▀ ▀▀▀ ▀▀▀ ▀▀▀ `

// WelcomeOptions describes everything the welcome screen needs. All fields
// are produced from real state by the caller; the renderer never guesses.
type WelcomeOptions struct {
	Version     string
	Commands    []Command
	ProjectLine string // e.g. "Go · 42 files · Git: main"; may be empty
	Width       int    // terminal width; <=0 means default (80)
}

// WelcomeScreen renders the full startup screen: logo, version, project
// line, and the command menu. It never exceeds the terminal width and
// automatically switches to a compact layout below 80 columns.
func WelcomeScreen(o WelcomeOptions) string {
	width := o.Width
	if width <= 0 {
		width = defaultTermWidth
	}
	mode := LayoutForWidth(width)
	var b strings.Builder

	logo := Primary(logo)
	if mode == LayoutCompact {
		logo = Primary(compactLogo)
	}
	for _, line := range strings.Split(logo, "\n") {
		b.WriteString(center(line, width))
		b.WriteByte('\n')
	}
	b.WriteString(center(Muted("v"+o.Version), width))
	b.WriteByte('\n')

	if o.ProjectLine != "" {
		b.WriteByte('\n')
		b.WriteString("  ")
		b.WriteString(Muted(o.ProjectLine))
		b.WriteByte('\n')
	}

	if len(o.Commands) > 0 {
		b.WriteByte('\n')
		b.WriteString(CommandMenu(o.Commands, width))
	}

	return b.String()
}

// center pads s so it appears horizontally centered within width columns,
// ignoring ANSI styling. Oversized content is returned unchanged.
func center(s string, width int) string {
	w := visibleWidth(s)
	if w >= width {
		return s
	}
	left := (width - w) / 2
	return strings.Repeat(" ", left) + s
}

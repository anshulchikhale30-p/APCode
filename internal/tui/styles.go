package tui

// Semantic glyphs shared by every APCode screen. They always render as
// plain characters so no-color mode stays fully usable.
const (
	GlyphWorking = "◐" // task in progress
	GlyphSuccess = "✓" // completed successfully
	GlyphWarning = "⚠" // warning / attention needed
	GlyphError   = "✗" // failure
	GlyphAction  = "→" // action taken (tool executed, hint)
	GlyphEdit    = "✎" // file modified
)

// Additional ANSI codes for the APCode palette. All styling funnels through
// style(), so --no-color and NO_COLOR=1 strip them automatically.
const (
	ansiAccent = "\x1b[95m" // bright magenta: APCode accent
	ansiBorder = "\x1b[34m" // blue: box borders and rules
	ansiInfo   = "\x1b[97m" // bright white: emphasized values
)

// Accent styles text with the APCode accent color.
func Accent(s string) string { return style(ansiAccent, s) }

// Border styles text used for box borders and divider rules.
func Border(s string) string { return style(ansiBorder, s) }

// Info styles emphasized plain values (model names, paths).
func Info(s string) string { return style(ansiInfo, s) }

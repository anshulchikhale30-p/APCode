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

// Additional ANSI codes — reuse the narrowed palette from color.go
// (no new hard-coded colors here per style guide).
const (
	ansiAccent      = ansiBlue    // was bright magenta; now blue #4A9EFF for interactive/active
	ansiBorder      = ansiBlue    // focused input box ring — blue (spec: focus indicator)
	ansiBorderMuted = ansiDimGray // unfocused/subtle borders — dim-gray
	ansiInfo        = ansiWhite   // emphasized values — white
)

// Accent styles text with the accent color (now blue).
func Accent(s string) string { return style(ansiAccent, s) }

// Border styles text used for focused box borders (input box — blue).
func Border(s string) string { return style(ansiBorder, s) }

// BorderMuted styles subtle borders/rules in dim-gray.
func BorderMuted(s string) string { return style(ansiBorderMuted, s) }

// Info styles emphasized plain values (model names, paths) in white.
func Info(s string) string { return style(ansiInfo, s) }

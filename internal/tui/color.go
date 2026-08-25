package tui

import (
	"os"
	"sync/atomic"
)

// ANSI SGR escape sequences used by the APCode palette.
const (
	ansiReset     = "\x1b[0m"
	ansiPrimary   = "\x1b[96m" // bright cyan
	ansiSecondary = "\x1b[36m" // cyan
	ansiSuccess   = "\x1b[32m" // green
	ansiWarning   = "\x1b[33m" // yellow
	ansiError     = "\x1b[31m" // red
	ansiMuted     = "\x1b[90m" // bright black (gray)
)

// colorsEnabled reports whether styled output should include ANSI escape
// sequences. It is initialized once via automatic detection and can be
// overridden programmatically (e.g. by a --no-color flag).
var colorsEnabled atomic.Bool

func init() {
	enableANSI()
	colorsEnabled.Store(detectColorSupport())
}

// detectColorSupport returns true when it is safe to emit ANSI colors:
// stdout is attached to a character device (terminal), the terminal is not
// "dumb", and the user has not set NO_COLOR. Setting CLICOLOR_FORCE to a
// non-empty value other than "0" forces colors on.
func detectColorSupport() bool {
	if f := os.Getenv("CLICOLOR_FORCE"); f != "" && f != "0" {
		return true
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ColorsEnabled reports whether terminal colors are currently enabled.
func ColorsEnabled() bool {
	return colorsEnabled.Load()
}

// SetColorsEnabled explicitly turns colored output on or off, overriding
// automatic detection.
func SetColorsEnabled(enabled bool) {
	colorsEnabled.Store(enabled)
}

// style wraps s in the given ANSI code when colors are enabled. Plain text
// is returned unchanged otherwise, so APCode remains fully readable on
// terminals without color support.
func style(code, s string) string {
	if !colorsEnabled.Load() || s == "" {
		return s
	}
	return code + s + ansiReset
}

// Primary styles text with the primary brand color.
func Primary(s string) string { return style(ansiPrimary, s) }

// Secondary styles text with the secondary color.
func Secondary(s string) string { return style(ansiSecondary, s) }

// Success styles text with the success color.
func Success(s string) string { return style(ansiSuccess, s) }

// Warning styles text with the warning color.
func Warning(s string) string { return style(ansiWarning, s) }

// Error styles text with the error color.
func Error(s string) string { return style(ansiError, s) }

// Muted styles text in a subdued color for labels and auxiliary output.
func Muted(s string) string { return style(ansiMuted, s) }

// Reset terminates any active styling.
func Reset() string {
	if !colorsEnabled.Load() {
		return ""
	}
	return ansiReset
}

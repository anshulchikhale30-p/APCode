package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
)

// ANSI SGR escape sequences — narrowed OpenCode-inspired palette.
//
// Roles (used consistently everywhere on the welcome screen):
//   - Background: terminal default (near-black, unchanged)
//   - Text: white (primary) → mid-gray (secondary) → dim-gray (muted) — no other neutrals
//   - Interactive/active: blue  #4A9EFF — active-mode label + input focus ring
//   - Callout: amber #E3A23D — tip line + one highlighted status value
//
// Extra magenta/pink/cyan removed; Success/Warning/Red kept only for non-welcome
// screens (/status, tool output) and never on the welcome screen.
const (
	ansiReset     = "\x1b[0m"
	ansiBold      = "\x1b[1m"
	ansiUnderline = "\x1b[4m"

	// Narrow palette — truecolor for precise hues.
	ansiWhite   = "\x1b[38;2;255;255;255m" // primary text — white
	ansiMidGray = "\x1b[38;2;160;160;160m" // secondary — #A0A0A0 mid-gray
	ansiDimGray = "\x1b[38;2;115;115;115m" // muted — #737373 dim-gray
	ansiBlue    = "\x1b[38;2;74;158;255m"  // interactive — #4A9EFF blue
	ansiAmber   = "\x1b[38;2;227;162;61m"  // callout — #E3A23D amber

	// Semantic colors for non-welcome screens (kept but not used on welcome).
	ansiSuccess = "\x1b[32m" // green
	ansiWarning = "\x1b[33m" // yellow (welcome uses amber instead)
	ansiError   = "\x1b[31m" // red

	// Legacy aliases — remapped to the narrowed palette so existing callers
	// (Primary/Secondary/Muted) automatically follow the new hierarchy.
	ansiPrimary   = ansiWhite   // was bright cyan; now white
	ansiSecondary = ansiMidGray // was cyan; now mid-gray
	ansiMuted     = ansiDimGray // was bright black; now dim-gray

	// Combined style for bold amber (Tip label).
	ansiAmberBold = "\x1b[1;38;2;227;162;61m"
)

// Dynamic terminal background — user-configurable via SetBackgroundColor.
// Stored as hex string "#RRGGBB" and computed truecolor escape "\x1b[48;2;R;G;Bm".
// When set, style() prepends the background escape so every viewport/sidebar/input
// automatically inherits it (main viewport, sidebars, input boxes).
// Default is an opencode-like near-black fill so `go run ./cmd/apcode` shows
// a visible background without any user config. Darker than pure black to
// match opencode's viewport — user can pick any darker or custom color via
// `/background` (hex or preset name).
const defaultBackgroundHex = "#0A0A0A"

// BackgroundPresets are curated dark presets so users can pick a darker
// canvas without guessing hex. All are near-black but distinct.
var backgroundPresets = map[string]string{
	"default":  "#0A0A0A", // darker default — almost black (10,10,10)
	"dark":     "#121212", // charcoal (18,18,18)
	"darker":   "#080A12", // midnight blue-black (8,10,18)
	"midnight": "#050507", // deepest (5,5,7)
	"charcoal": "#1A1A1A", // soft charcoal (26,26,26)
	"opencode": "#1A1B26", // original opencode navy (26,27,38)
	"ink":      "#0D1117", // github ink (13,17,23)
	"obsidian": "#0B0D14", // obsidian (11,13,20)
}

var (
	backgroundHex    atomic.Value // string
	backgroundEscape atomic.Value // string "\x1b[48;2;R;G;Bm" or ""
)

// colorsEnabled reports whether styled output should include ANSI escape
// sequences. It is initialized once via automatic detection and can be
// overridden programmatically (e.g. by a --no-color flag).
var colorsEnabled atomic.Bool

func init() {
	enableANSI()
	colorsEnabled.Store(detectColorSupport())
	// Initialize default opencode-like background so the TUI shows a fill even
	// without user customization. This is applied via style() to every pane.
	_ = SetBackgroundColor(defaultBackgroundHex)
	// Try to restore user-chosen background from config (persists across runs).
	// This makes `apcode` remember the user's dark choice without re-typing
	// `/background` each time. Failures are silent — default remains.
	if home, err := os.UserHomeDir(); err == nil {
		cfgPath := filepath.Join(home, ".apcode", "config.json")
		if data, err := os.ReadFile(cfgPath); err == nil {
			var cfg map[string]interface{}
			if json.Unmarshal(data, &cfg) == nil {
				if bg, ok := cfg["background"].(string); ok && bg != "" {
					_ = SetBackgroundColor(bg)
				}
			}
		}
	}
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
// terminals without color support. When a custom background is set via
// SetBackgroundColor, the background escape is prepended so every styled
// component (main viewport, sidebars, input boxes) shares the same fill.
func style(code, s string) string {
	if !colorsEnabled.Load() || s == "" {
		return s
	}
	bg := GetBackgroundEscape()
	if bg != "" {
		return bg + code + s + ansiReset
	}
	return code + s + ansiReset
}

// SetBackgroundColor parses a hex color (e.g. "#1A1B26", "1a1b26", "#abc") or a
// preset name (dark/darker/midnight/charcoal/opencode/ink/obsidian) and stores
// the 48;2 escape for all panes. Pass "default" for the darker #0A0A0A fill;
// "none"/"transparent" clears to terminal default.
func SetBackgroundColor(hex string) error {
	hex = strings.TrimSpace(hex)
	if hex == "" {
		hex = defaultBackgroundHex
	}
	lower := strings.ToLower(hex)
	if preset, ok := backgroundPresets[lower]; ok {
		hex = preset
	} else if strings.EqualFold(hex, "default") {
		hex = defaultBackgroundHex
	}
	if strings.EqualFold(hex, "none") || strings.EqualFold(hex, "transparent") {
		backgroundHex.Store("")
		backgroundEscape.Store("")
		return nil
	}
	hex = strings.TrimPrefix(hex, "#")
	// support 3-digit shorthand #abc -> #aabbcc
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return fmt.Errorf("invalid background color %q: expected #RRGGBB", hex)
	}
	r, err1 := strconv.ParseUint(hex[0:2], 16, 8)
	g, err2 := strconv.ParseUint(hex[2:4], 16, 8)
	b, err3 := strconv.ParseUint(hex[4:6], 16, 8)
	if err1 != nil || err2 != nil || err3 != nil {
		return fmt.Errorf("invalid background color %q: not hex", hex)
	}
	esc := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
	backgroundHex.Store("#" + strings.ToUpper(hex))
	backgroundEscape.Store(esc)
	return nil
}

// GetBackgroundColor returns the current custom background hex (e.g. "#1A1B26")
// or "" when using terminal default.
func GetBackgroundColor() string {
	if v := backgroundHex.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetBackgroundEscape returns the computed 48;2 escape for the current
// background, or "" when default/transparent.
func GetBackgroundEscape() string {
	if v := backgroundEscape.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// BackgroundPresets returns a copy of the curated dark presets.
func BackgroundPresets() map[string]string {
	out := make(map[string]string, len(backgroundPresets))
	for k, v := range backgroundPresets {
		out[k] = v
	}
	return out
}

// BackgroundPresetNames returns sorted preset names for help display.
func BackgroundPresetNames() []string {
	names := make([]string, 0, len(backgroundPresets))
	for k := range backgroundPresets {
		names = append(names, k)
	}
	// simple sort
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	return names
}

// ResetBackgroundColor restores the opencode-like default fill.
func ResetBackgroundColor() {
	_ = SetBackgroundColor(defaultBackgroundHex)
}

// ClearBackgroundColor clears any background to terminal transparent.
func ClearBackgroundColor() {
	backgroundHex.Store("")
	backgroundEscape.Store("")
}

// SaveBackgroundToConfig persists the current background choice to
// ~/.apcode/config.json so `apcode` restores it on next launch.
// It creates the config dir if needed and preserves other fields.
func SaveBackgroundToConfig(hex string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(home, ".apcode", "config.json")
	cfgDir := filepath.Dir(cfgPath)
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return err
	}
	var cfg map[string]interface{}
	if data, err := os.ReadFile(cfgPath); err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}
	if hex == "" {
		delete(cfg, "background")
	} else {
		cfg["background"] = hex
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cfgPath, data, 0o644)
}

// Background wraps s with the current background color escape when enabled
// and one is set. Useful for raw strings that are not passed through style().
func Background(s string) string {
	if !colorsEnabled.Load() || s == "" {
		return s
	}
	bg := GetBackgroundEscape()
	if bg == "" {
		return s
	}
	return bg + s + ansiReset
}

// ApplyBackground is an alias for Background — kept for theme-manager call sites
// that explicitly mention background application.
func ApplyBackground(s string) string { return Background(s) }

// BackgroundFill pads line to width and wraps the entire line with the
// background fill so the full viewport (including outer padding and gutters)
// shows the opencode-like fill. Used by welcome and dashboard to ensure
// every row — main viewport, sidebars, input boxes — shares the same background.
func BackgroundFill(line string, width int) string {
	if !ColorsEnabled() {
		return line
	}
	bg := GetBackgroundEscape()
	if bg == "" {
		return line
	}
	vw := visibleWidth(line)
	if vw < width {
		line += strings.Repeat(" ", width-vw)
	} else if vw > width {
		line = truncateVisible(line, width)
	}
	return bg + line + ansiReset
}

// WriteTerminalBackground emits a whole-terminal background sequence so
// `apcode` changes the entire terminal canvas, not just the printed rows.
// It writes an OSC 11 background change (for emulators that support it) plus
// a 48;2 fill that covers the viewport. Call once at startup when the REPL
// is interactive; the theme manager then keeps every pane (main viewport,
// sidebars, input boxes) in sync via style().
func WriteTerminalBackground(w io.Writer) {
	if !ColorsEnabled() || w == nil {
		return
	}
	bgHex := GetBackgroundColor()
	if bgHex == "" {
		bgHex = defaultBackgroundHex
	}
	// OSC 11 — set terminal emulator background (supported by many terms)
	// Format: ESC ] 11 ; #RRGGBB BEL  (BEL = \x07)
	fmt.Fprintf(w, "\x1b]11;%s\x07", bgHex)
	// Also set the 48;2 background for subsequent cells
	if esc := GetBackgroundEscape(); esc != "" {
		fmt.Fprint(w, esc)
	}
}

// ResetTerminalBackground restores the terminal background to default.
func ResetTerminalBackground(w io.Writer) {
	if w == nil {
		return
	}
	// OSC 111 — reset background to default
	fmt.Fprint(w, "\x1b]111\x07")
	fmt.Fprint(w, ansiReset)
}

// Primary styles text with the primary brand color (white in narrowed palette).
func Primary(s string) string { return style(ansiPrimary, s) }

// Secondary styles text with the secondary color (mid-gray).
func Secondary(s string) string { return style(ansiSecondary, s) }

// Success styles text with the success color (kept for non-welcome screens).
func Success(s string) string { return style(ansiSuccess, s) }

// Warning styles text with the warning color (kept for non-welcome screens; welcome uses Amber).
func Warning(s string) string { return style(ansiWarning, s) }

// Error styles text with the error color.
func Error(s string) string { return style(ansiError, s) }

// Muted styles text in a subdued color for labels and auxiliary output (dim-gray).
func Muted(s string) string { return style(ansiMuted, s) }

// Blue styles interactive/active elements (mode label, input focus ring — #4A9EFF).
func Blue(s string) string { return style(ansiBlue, s) }

// Amber styles callouts (tip dot/label, highlighted setting — #E3A23D).
func Amber(s string) string { return style(ansiAmber, s) }

// AmberBold styles tip label "Tip" as bold amber.
func AmberBold(s string) string { return style(ansiAmberBold, s) }

// White is an alias for Primary (white) — used for woven logo and emphasized values.
func White(s string) string { return style(ansiWhite, s) }

// Bold styles text in bold.
func Bold(s string) string { return style(ansiBold, s) }

// Underline styles text underlined.
func Underline(s string) string { return style(ansiUnderline, s) }

// Reset terminates any active styling.
func Reset() string {
	if !colorsEnabled.Load() {
		return ""
	}
	return ansiReset
}

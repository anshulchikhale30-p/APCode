package tui

import (
	"fmt"
	"strings"
)

// SessionMetadata holds the right-sidebar session info per spec:
// active session ID, context token usage, percentage, cost, LSP status.
// All fields are produced from real session state; renderer never guesses.
type SessionMetadata struct {
	SessionID   string // e.g. "a1b2c3"
	TokensUsed  int    // e.g. 12345
	TokensTotal int    // e8. 128000 (0 = unknown)
	CostUSD     float64
	LSPReady    bool // true => "ready", false => "offline"
}

// TodoState tracks a single checklist entry.
type TodoState int

const (
	TodoPending    TodoState = iota // [ ] pending
	TodoInProgress                  // [•] in-progress
	TodoCompleted                   // [✓] completed
)

// TodoItem is one entry in the live agent plan checklist.
type TodoItem struct {
	Title string
	State TodoState
}

// DashboardOptions bundles everything the active-session two-column view needs.
// Width/Height come from tea.WindowSizeMsg (or TerminalSize) for responsive
// without wrapping bugs.
type DashboardOptions struct {
	Width  int // total viewport width
	Height int // total viewport height (0 = auto)

	// Left pane: main execution stream, tool outputs, LLM streaming
	LeftContent string // raw multi-line stream

	// Right sidebar metadata
	Meta  SessionMetadata
	Todos []TodoItem

	// Bottom prompt chips
	Mode      string // blue chip, e.g. "native"
	ModelName string // white chip
	Provider  string // dim chip, e.g. "ollama"
	Highlight string // amber chip

	// Pinned footer
	Workspace string
	GitBranch string
	Version   string
}

// RenderDashboard renders the full active-session view:
//
//	┌──────────────────────┬─────────────────────┐
//	│ Left pane            │ Right sidebar       │
//	│ (stream)             │ (session meta +     │
//	│                      │  todo widget)       │
//	├──────────────────────┴─────────────────────┤
//	│ Bordered prompt (chips)                    │
//	│ esc interrupt    ctrl+p commands           │
//	│ ~/dir:branch              v0.1.5           │
//	└────────────────────────────────────────────┘
//
// It is responsive via NewDashboardLayout / tea.WindowSizeMsg and applies the
// dynamic background from color.go to every pane (main viewport, sidebars, input boxes).
func RenderDashboard(o DashboardOptions) string {
	width := o.Width
	if width <= 0 {
		width = defaultTermWidth
	}
	height := o.Height
	if height <= 0 {
		height = defaultTermHeight
	}
	layout := NewDashboardLayout(width, height)
	// Reserve bottom area: input box (4 lines) + hints (1) + status bar (1) + gaps (2) = ~8
	const bottomReserve = 8
	contentHeight := height - bottomReserve
	if contentHeight < 6 {
		contentHeight = 6
	}
	if contentHeight > height {
		contentHeight = height
	}

	var mainContent string
	if layout.ShowSidebar && layout.RightWidth >= 20 {
		// Two-column path: left stream + bordered right sidebar
		rightBox := RenderSidebar(layout.RightWidth, contentHeight, o.Meta, o.Todos)
		leftLines := prepareLeftLines(o.LeftContent, layout.LeftWidth, contentHeight)
		rightLines := strings.Split(rightBox, "\n")
		mainContent = mergeColumns(leftLines, rightLines, layout.LeftWidth, layout.RightWidth, layout.Gutter, contentHeight)
	} else {
		// Single-column fallback on compact/narrow: stacked, sidebar collapsed to header line
		leftLines := prepareLeftLines(o.LeftContent, width, contentHeight)
		// On narrow, still show a compact sidebar header as a single muted line above stream
		header := ""
		if o.Meta.SessionID != "" {
			header = Muted(fmt.Sprintf("session %s · %s", o.Meta.SessionID, formatTokens(o.Meta)))
		}
		if header != "" {
			// prepend header then stream
			var b strings.Builder
			b.WriteString(truncateVisible(header, width))
			b.WriteByte('\n')
			b.WriteString(Muted(strings.Repeat("─", minInt(width, 40))))
			b.WriteByte('\n')
			b.WriteString(strings.Join(leftLines, "\n"))
			mainContent = b.String()
		} else {
			mainContent = strings.Join(leftLines, "\n")
		}
		// ensure background applied to main content block
		if GetBackgroundEscape() != "" {
			lines := strings.Split(mainContent, "\n")
			for i, l := range lines {
				lines[i] = Background(l)
			}
			mainContent = strings.Join(lines, "\n")
		}
	}

	// Bottom prompt + footer pinned to viewport bottom
	bottom := RenderBottomPrompt(width, o.Mode, o.ModelName, o.Provider, o.Highlight)
	hints := RenderBottomHints(width)
	status := StatusBar(width, o.Workspace, o.GitBranch, o.Version)

	var b strings.Builder
	b.WriteString(mainContent)
	b.WriteByte('\n')
	b.WriteString(bottom)
	b.WriteByte('\n')
	b.WriteString(centerHints(hints, width))
	b.WriteByte('\n')
	b.WriteString(status)
	b.WriteByte('\n')
	raw := b.String()
	// Full-viewport background fill like opencode — every row padded to width
	// with the dynamic background so main viewport, sidebars and input boxes
	// all share the same fill without gaps.
	if GetBackgroundEscape() != "" && ColorsEnabled() {
		lines := strings.Split(raw, "\n")
		for i, l := range lines {
			if i == len(lines)-1 && l == "" {
				continue
			}
			lines[i] = BackgroundFill(l, width)
		}
		raw = strings.Join(lines, "\n")
	}
	return raw
}

// RenderSidebar builds the bordered right sidebar for the given width/height.
// It shows session metadata (ID, tokens, %, cost, LSP) and the live Todo widget.
func RenderSidebar(width, height int, meta SessionMetadata, todos []TodoItem) string {
	if width < 20 {
		width = 20
	}
	inner := width - 2
	if inner < 10 {
		inner = 10
	}
	// Reserve at least 4 lines for box borders + title
	availLines := height
	if availLines < 4 {
		availLines = 4
	}
	var b strings.Builder
	// Top border with title
	titleText := "─ Session ─"
	if tw := len(titleText); tw > inner {
		inner = tw
		width = inner + 2
	}
	b.WriteString(Border("╭"))
	b.WriteString(titleText)
	b.WriteString(BorderMuted(strings.Repeat("─", inner-len(titleText))))
	b.WriteString(Border("╮"))
	b.WriteByte('\n')

	// Helper to emit one inner line, truncated/padded, with left/right border
	// Every pane (main viewport, sidebars, input boxes) shares the dynamic background.
	emit := func(content string) {
		w := visibleWidth(content)
		if w > inner {
			content = truncateVisible(content, inner)
			w = visibleWidth(content)
		}
		pad := inner - w
		if pad < 0 {
			pad = 0
		}
		padStr := strings.Repeat(" ", pad)
		if GetBackgroundEscape() != "" {
			padStr = Background(padStr)
			// filler lines that are all spaces need background too
			if strings.TrimSpace(stripANSI(content)) == "" && content != "" {
				content = Background(content)
			}
		}
		// Borders are styled; inner content already styled via callers (style() includes bg)
		b.WriteString(BorderMuted("│"))
		b.WriteString(content)
		b.WriteString(padStr)
		b.WriteString(BorderMuted("│"))
		b.WriteByte('\n')
	}

	// Session ID
	if meta.SessionID != "" {
		emit(White("  id ") + Muted(meta.SessionID))
	} else {
		emit(Muted("  session —"))
	}
	// Tokens + percent
	emit(Muted("  tokens ") + White(formatTokens(meta)) + Muted(formatPercent(meta)))
	// Cost
	emit(Muted("  cost ") + White(formatCost(meta.CostUSD)))
	// LSP status
	lspLabel := "  lsp "
	if meta.LSPReady {
		emit(Muted(lspLabel) + Success("ready"))
	} else {
		emit(Muted(lspLabel) + Muted("offline"))
	}
	// Divider
	emit(BorderMuted(strings.Repeat("─", inner)))
	// Todo widget header
	emit(Amber("  Todos"))

	// Todo list — live checklist
	if len(todos) == 0 {
		emit(Muted("  (no tasks)"))
	} else {
		// Fit as many as height allows; reserve 2 lines for bottom border
		maxTodos := availLines - 10 // borders + meta lines
		if maxTodos < 2 {
			maxTodos = 2
		}
		if maxTodos > len(todos) {
			maxTodos = len(todos)
		}
		for i := 0; i < maxTodos; i++ {
			emit(renderTodoLine(todos[i], inner-2))
		}
		if len(todos) > maxTodos {
			emit(Muted(fmt.Sprintf("  +%d more", len(todos)-maxTodos)))
		}
	}
	// Fill remaining height with empty padded lines so sidebar is exactly height tall
	// and background fill covers the whole pane without wrapping gaps.
	usedLines := 1 + // top border
		5 + // meta lines (id, tokens, cost, lsp, divider)
		1 + // Todos header
		maxInt(1, minInt(len(todos), maxTodosForHeight(availLines))) + // todos
		1 // placeholder for empty filler count calc; we compute actual below
	// More accurate: count lines written so far via string split
	actualLines := strings.Count(b.String(), "\n")
	need := availLines - actualLines - 1 // -1 for bottom border
	for i := 0; i < need; i++ {
		emit(strings.Repeat(" ", inner))
		usedLines++ // not used but keeps var referenced
		_ = usedLines
	}

	b.WriteString(BorderMuted("╰"))
	b.WriteString(strings.Repeat("─", inner))
	b.WriteString(BorderMuted("╯"))
	return b.String()
}

func maxTodosForHeight(height int) int {
	n := height - 10
	if n < 2 {
		return 2
	}
	return n
}

func renderTodoLine(item TodoItem, width int) string {
	var glyph, title string
	switch item.State {
	case TodoCompleted:
		glyph = Success("[✓]")
		title = Muted(item.Title)
	case TodoInProgress:
		glyph = Blue("[•]")
		title = White(item.Title)
	default:
		glyph = Muted("[ ]")
		title = Muted(item.Title)
	}
	line := "  " + glyph + " " + title
	if visibleWidth(line) > width+4 { // account for leading spaces
		line = truncateVisible(line, width+4)
	}
	return line
}

func formatTokens(m SessionMetadata) string {
	if m.TokensTotal == 0 {
		return fmt.Sprintf("%d tokens", m.TokensUsed)
	}
	return fmt.Sprintf("%s / %s", formatK(m.TokensUsed), formatK(m.TokensTotal))
}

func formatPercent(m SessionMetadata) string {
	if m.TokensTotal == 0 {
		return ""
	}
	p := float64(m.TokensUsed) / float64(m.TokensTotal) * 100
	return fmt.Sprintf(" (%.0f%%)", p)
}

func formatK(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func formatCost(usd float64) string {
	if usd == 0 {
		return "$0.00"
	}
	return fmt.Sprintf("$%.2f", usd)
}

func prepareLeftLines(content string, width, height int) []string {
	if content == "" {
		content = Muted("  (no output — stream will appear here)")
	}
	lines := strings.Split(content, "\n")
	// Wrap long lines to avoid horizontal overflow (no wrapping bugs)
	var wrapped []string
	for _, l := range lines {
		// Word-wrap to width, preserving at most height lines
		for _, wl := range WrapText(l, width) {
			wrapped = append(wrapped, wl)
			if len(wrapped) >= height {
				break
			}
		}
		if len(wrapped) >= height {
			break
		}
	}
	// Pad to height with empty lines for stable column merge
	for len(wrapped) < height {
		wrapped = append(wrapped, "")
	}
	// Truncate/pad each line to exactly width for column merge without jitter
	for i, l := range wrapped {
		if visibleWidth(l) > width {
			wrapped[i] = truncateVisible(l, width)
		}
		// Pad with spaces so background fill covers full pane width
		pad := width - visibleWidth(wrapped[i])
		if pad > 0 {
			wrapped[i] += strings.Repeat(" ", pad)
		}
		// Apply background as needed via Background for raw pane content
		if GetBackgroundEscape() != "" {
			wrapped[i] = Background(wrapped[i])
		}
	}
	return wrapped[:height]
}

func mergeColumns(left, right []string, leftWidth, rightWidth, gutter, height int) string {
	// right param is the Inner width? Actually left,right slices are already sized to panes.
	// This merges them with a gutter and ensures no wrapping.
	var b strings.Builder
	for i := 0; i < height; i++ {
		l := ""
		if i < len(left) {
			l = left[i]
		} else {
			l = strings.Repeat(" ", leftWidth)
			if GetBackgroundEscape() != "" {
				l = Background(l)
			}
		}
		r := ""
		if i < len(right) {
			r = right[i]
		} else {
			r = strings.Repeat(" ", rightWidth)
		}
		// left is already padded to leftWidth and has background,
		// right is the raw sidebar line which already includes its own borders and background via style()
		b.WriteString(l)
		b.WriteString(strings.Repeat(" ", gutter))
		b.WriteString(r)
		if i < height-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// RenderBottomPrompt is the unified single-line bordered input prompt with
// active model/provider chips per spec (e.g. "ollama • Model Name • mode").
// When width is too narrow it truncates chips rather than wrapping.
// Handles tea.WindowSizeMsg resizing by recomputing BoxWidth from the current
// width on every call — no wrapping bugs.
func RenderBottomPrompt(width int, mode, modelName, provider, highlight string) string {
	// For the dashboard, render a clean single-line bordered box (placeholder row)
	// plus a pinned chip line below that shows provider/model/mode.
	// The welcome screen's two-row InputBoxTwoRow is kept for the welcome view;
	// the active session's bottom prompt uses a single inner row plus an outer
	// chip row to satisfy the "single-line bordered input prompt with chips" spec.
	boxW := BoxWidth(width)
	inner := boxW - 4
	if inner < 10 {
		inner = 10
	}
	// Single inner placeholder row
	row1Plain := "› " + placeholderAsk + " " + placeholderExample
	row1Styled := Blue("›") + " " + Muted(placeholderAsk) + " " + Muted(placeholderExample)
	if visibleWidth(row1Plain) > inner {
		avail := inner - 2
		if avail < 4 {
			avail = 4
		}
		ph := truncateVisible(Muted(placeholderAsk+" "+placeholderExample), avail)
		row1Styled = Blue("›") + " " + ph
		row1Plain = "› " + stripANSI(ph)
	}
	pad1 := inner - visibleWidth(row1Plain)
	if pad1 < 0 {
		pad1 = 0
	}
	var box strings.Builder
	box.WriteString(InputBoxTop(boxW))
	box.WriteByte('\n')
	box.WriteString(Border("│") + " " + row1Styled + strings.Repeat(" ", pad1) + " " + Border("│"))
	box.WriteByte('\n')
	box.WriteString(InputBoxBottom(boxW))
	// Center the bordered box
	lines := strings.Split(box.String(), "\n")
	var b strings.Builder
	for _, line := range lines {
		if line == "" {
			continue
		}
		left := (width - visibleWidth(line)) / 2
		if left < 0 {
			left = 0
		}
		b.WriteString(strings.Repeat(" ", left))
		b.WriteString(line)
		b.WriteByte('\n')
	}
	// Chip line — e.g. "ollama • Model Name • mode" with correct palette
	// Provider (dim) • Model (white) • Mode (blue) • Highlight (amber)
	chips := renderBottomChips(provider, modelName, mode, highlight)
	if visibleWidth(chips) > width {
		chips = truncateVisible(chips, width)
	}
	// Center chips line below the box, using same centering as the box
	leftChips := (width - visibleWidth(chips)) / 2
	if leftChips < 0 {
		leftChips = 0
	}
	b.WriteString(strings.Repeat(" ", leftChips))
	b.WriteString(chips)
	return strings.TrimRight(b.String(), "\n")
}

// renderBottomChips builds the provider • model • mode chip line per spec
// (all segments joined by " • " with palette: provider dim, model white, mode blue, highlight amber).
func renderBottomChips(provider, modelName, mode, highlight string) string {
	var segs []string
	if provider != "" {
		segs = append(segs, Muted(provider))
	}
	if modelName != "" {
		segs = append(segs, White(modelName))
	} else {
		if len(segs) == 0 && mode == "" && highlight == "" {
			segs = append(segs, White("no model installed"))
		} else if len(segs) == 0 && mode != "" {
			// will add mode later; still need placeholder for model?
		}
	}
	if mode != "" {
		segs = append(segs, Blue(mode))
	}
	if highlight != "" {
		segs = append(segs, AmberBold(highlight))
	}
	if len(segs) == 0 {
		segs = append(segs, Blue("native"), White("no model installed"))
	} else if len(segs) == 1 && provider != "" && modelName == "" && mode == "" {
		segs = append(segs, White("no model installed"))
	} else if len(segs) == 1 && mode != "" && modelName == "" && provider == "" {
		segs = append(segs, White("no model installed"))
	}
	if len(segs) == 1 {
		return segs[0]
	}
	sep := Muted(" • ")
	return segs[0] + sep + joinWithSep(segs[1:], sep)
}

// RenderBottomHints pins the keybinding hints cleanly to the bottom viewport.
func RenderBottomHints(width int) string {
	// Per spec: `esc interrupt`, `ctrl+p commands` — keys brighter than descs
	pairs := []struct{ key, desc string }{
		{key: "esc", desc: "interrupt"},
		{key: "ctrl+p", desc: "commands"},
	}
	var b strings.Builder
	for i, p := range pairs {
		if i > 0 {
			b.WriteString(strings.Repeat(" ", 6))
		}
		b.WriteString(White(p.key))
		b.WriteString(" ")
		b.WriteString(Secondary(p.desc))
	}
	line := b.String()
	if visibleWidth(line) > width {
		return truncateVisible(line, width)
	}
	return line
}

// RenderTodoList renders a standalone live Todo checklist widget for
// the sidebar or /todos command. Supports completed ([✓]), in-progress ([•]), pending ([ ]).
func RenderTodoList(todos []TodoItem, width int) string {
	if width <= 0 {
		width = defaultTermWidth
	}
	var b strings.Builder
	b.WriteString(Amber("Todos"))
	b.WriteByte('\n')
	b.WriteString(BorderMuted(strings.Repeat("─", minInt(width, 28))))
	b.WriteByte('\n')
	if len(todos) == 0 {
		b.WriteString(Muted("  (no tasks)"))
		b.WriteByte('\n')
		return b.String()
	}
	for _, it := range todos {
		line := renderTodoLine(it, width-4)
		// Ensure line never exceeds width (no wrapping bugs on resize)
		if visibleWidth(line) > width {
			line = truncateVisible(line, width)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

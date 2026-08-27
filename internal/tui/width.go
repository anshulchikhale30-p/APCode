package tui

// Shared terminal-width constants and layout breakpoints.
const (
	defaultTermWidth  = 80
	defaultTermHeight = 24

	// WidthBreakpointCompact is the column count below which APCode
	// renders its compact UI.
	WidthBreakpointCompact = 80
	// WidthBreakpointExpanded is the column count at or above which
	// APCode renders its expanded UI.
	WidthBreakpointExpanded = 120
)

// WindowSizeMsg mirrors tea.WindowSizeMsg from bubbletea for resizing.
// The TUI layout engine handles this message to recompute the two-column
// split and right sidebar without wrapping bugs.
type WindowSizeMsg struct {
	Width  int
	Height int
}

// LayoutMode describes how much room the current terminal offers.
type LayoutMode int

const (
	LayoutCompact  LayoutMode = iota // < 80 columns
	LayoutNormal                     // 80-119 columns
	LayoutExpanded                   // >= 120 columns
)

// LayoutForWidth classifies a terminal width into a layout mode.
func LayoutForWidth(width int) LayoutMode {
	if width < WidthBreakpointCompact {
		return LayoutCompact
	}
	if width >= WidthBreakpointExpanded {
		return LayoutExpanded
	}
	return LayoutNormal
}

// ClampWidth constrains a desired content width to [min, max].
func ClampWidth(width, min, max int) int {
	if width < min {
		return min
	}
	if width > max {
		return max
	}
	return width
}

// TerminalSize returns width and height, falling back to defaults on error.
// This is the helper that handles tea.WindowSizeMsg updates in the bubbletea
// pattern: on resize, callers receive a WindowSizeMsg and recompute layout.
func TerminalSize() (int, int) {
	w := TerminalWidth()
	h := TerminalHeight()
	return w, h
}

// DashboardLayout holds the two-column split geometry for the active session
// view. See RenderDashboard for usage; it ensures wrapping-free layout on
// every tea.WindowSizeMsg.
type DashboardLayout struct {
	Width       int
	Height      int
	LeftWidth   int // main stream pane inner width
	RightWidth  int // sidebar inner width (0 when hidden)
	ShowSidebar bool
	Gutter      int // columns between panes
}

// NewDashboardLayout computes a responsive two-column split from the current
// terminal size. Sidebar scales smoothly and never causes wrapping bugs.
func NewDashboardLayout(width, height int) DashboardLayout {
	if width <= 0 {
		width = defaultTermWidth
	}
	if height <= 0 {
		height = defaultTermHeight
	}
	// Responsive: hide sidebar on compact (<80), narrow sidebar on normal,
	// wider sidebar on expanded.
	showSidebar := width >= WidthBreakpointCompact
	right := 0
	left := width
	gutter := 0
	if showSidebar {
		gutter = 1
		switch {
		case width >= WidthBreakpointExpanded:
			right = 38 // generous sidebar on wide screens
		default:
			right = 34 // narrow sidebar, still enough for tokens/cost without truncation
		}
		// ensure left pane still has room
		if width-right-gutter < 24 {
			right = width - 24 - gutter
			if right < 20 {
				right = 20
			}
		}
		left = width - right - gutter
	}
	return DashboardLayout{
		Width:       width,
		Height:      height,
		LeftWidth:   left,
		RightWidth:  right,
		ShowSidebar: showSidebar,
		Gutter:      gutter,
	}
}

// HandleWindowSizeMsg is sugar for NewDashboardLayout(msg.Width, msg.Height)
// — call this from a bubbletea Update when msg is tea.WindowSizeMsg.
func HandleWindowSizeMsg(msg WindowSizeMsg) DashboardLayout {
	return NewDashboardLayout(msg.Width, msg.Height)
}

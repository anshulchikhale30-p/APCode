package tui

// Shared terminal-width constants and layout breakpoints.
const (
	defaultTermWidth = 80

	// WidthBreakpointCompact is the column count below which APCode
	// renders its compact UI.
	WidthBreakpointCompact = 80
	// WidthBreakpointExpanded is the column count at or above which
	// APCode renders its expanded UI.
	WidthBreakpointExpanded = 120
)

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

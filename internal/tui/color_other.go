//go:build !windows

package tui

// enableANSI is a no-op on platforms where terminals handle ANSI escape
// sequences natively.
func enableANSI() {}

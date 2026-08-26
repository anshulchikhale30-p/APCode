//go:build !windows && !linux && !darwin

package tui

import "os"

const defaultTermWidth = 80

// TerminalWidth cannot be detected on this platform; return the safe default.
func TerminalWidth() int { return defaultTermWidth }

// IsTerminalWriter reports whether w is attached to a character device.
func IsTerminalWriter(w interface{ Write([]byte) (int, error) }) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

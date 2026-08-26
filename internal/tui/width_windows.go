//go:build windows

package tui

import (
	"os"
	"unsafe"
)

type coord struct{ X, Y int16 }

type smallRect struct {
	Left, Top, Right, Bottom int16
}

type consoleScreenBufferInfo struct {
	Size              coord
	CursorPosition    coord
	Attributes        uint16
	Window            smallRect
	MaximumWindowSize coord
}

var (
	procGetStdHandle           = kernel32.NewProc("GetStdHandle")
	procGetConsoleScreenBuffer = kernel32.NewProc("GetConsoleScreenBufferInfo")
)

const stdOutputHandle = ^uintptr(0) - 9 // STD_OUTPUT_HANDLE

// TerminalWidth returns the current terminal width in columns. It falls
// back to 80 when the size cannot be determined (piped output, unsupported
// console, etc.) so callers never need to handle a failure case.
func TerminalWidth() int {
	h, _, _ := procGetStdHandle.Call(stdOutputHandle)
	var info consoleScreenBufferInfo
	r, _, _ := procGetConsoleScreenBuffer.Call(h, uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return defaultTermWidth
	}
	w := int(info.Window.Right-info.Window.Left) + 1
	if w <= 0 {
		return defaultTermWidth
	}
	return w
}

// IsTerminalWriter reports whether w is attached to a character device
// (a real terminal), which gates animation and cursor tricks.
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

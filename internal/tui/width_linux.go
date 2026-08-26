//go:build linux

package tui

import (
	"os"
	"syscall"
	"unsafe"
)

const tiocgwinsz = 0x5414

type winsize struct {
	rows, cols, xpixel, ypixel uint16
}

// TerminalWidth returns the current terminal width in columns, or 80 when
// it cannot be determined.
func TerminalWidth() int {
	ws := &winsize{}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(), uintptr(tiocgwinsz), uintptr(unsafe.Pointer(ws)))
	if errno != 0 || ws.cols == 0 {
		return defaultTermWidth
	}
	return int(ws.cols)
}

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

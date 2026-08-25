//go:build windows

package tui

import (
	"os"
	"syscall"
	"unsafe"
)

// enableVirtualTerminalProcessing is the console mode flag that enables
// ANSI escape sequence handling in the Windows console host.
const enableVirtualTerminalProcessing = 0x0004

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
)

// enableANSI best-effort enables virtual terminal processing so ANSI
// escape sequences render correctly on Windows (PowerShell, cmd, and
// Windows Terminal). It silently does nothing if the console API is
// unavailable or the calls fail; APCode stays fully usable without it.
func enableANSI() {
	var mode uint32
	handle := syscall.Handle(os.Stdout.Fd())
	r1, _, _ := procGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	if r1 == 0 {
		return
	}
	procSetConsoleMode.Call(uintptr(handle), uintptr(mode|enableVirtualTerminalProcessing))
}

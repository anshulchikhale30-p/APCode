//go:build darwin

package hardware

import "syscall"

// physicalCores reads hw.physicalcpu via sysctl. ok is false when the
// sysctl is unavailable; the count is never fabricated.
func physicalCores() (int, bool) {
	n, err := syscall.SysctlUint32("hw.physicalcpu")
	if err != nil || n == 0 {
		return 0, false
	}
	return int(n), true
}

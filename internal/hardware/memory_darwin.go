//go:build darwin

package hardware

import (
	"fmt"
	"strconv"
	"syscall"
)

// readMemory reports total physical RAM via the hw.memsize sysctl.
// Available RAM is derived from free+inactive pages when those sysctls
// are exposed by the kernel; otherwise it stays unknown.
func readMemory() (MemoryInfo, error) {
	totalStr, err := syscall.Sysctl("hw.memsize")
	if err != nil {
		return MemoryInfo{}, fmt.Errorf("hw.memsize unavailable: %w", err)
	}
	total, err := strconv.ParseUint(totalStr, 10, 64)
	if err != nil || total == 0 {
		return MemoryInfo{}, fmt.Errorf("hw.memsize unreadable: %q", totalStr)
	}

	info := MemoryInfo{TotalRAMBytes: total}

	const defaultPageSize = 4096
	pageSize := uint64(defaultPageSize)
	if v, err := syscall.SysctlUint32("hw.pagesize"); err == nil && v > 0 {
		pageSize = uint64(v)
	}

	free, freeErr := syscall.SysctlUint32("vm.page_free_count")
	inactive, inactiveErr := syscall.SysctlUint32("vm.page_inactive_count")
	if freeErr == nil && inactiveErr == nil {
		info.AvailableRAMBytes = (uint64(free) + uint64(inactive)) * pageSize
		info.AvailableRAMKnown = true
	}

	return info, nil
}

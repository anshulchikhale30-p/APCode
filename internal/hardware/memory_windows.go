//go:build windows

package hardware

import (
	"fmt"
	"unsafe"
)

var procGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")

// memoryStatusEx mirrors MEMORYSTATUSEX.
type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// readMemory reports total and available physical RAM via
// GlobalMemoryStatusEx.
func readMemory() (MemoryInfo, error) {
	if procGlobalMemoryStatusEx.Find() != nil {
		return MemoryInfo{}, fmt.Errorf("GlobalMemoryStatusEx unavailable")
	}

	var status memoryStatusEx
	status.Length = uint32(unsafe.Sizeof(status))
	ret, _, _ := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if ret == 0 {
		return MemoryInfo{}, fmt.Errorf("GlobalMemoryStatusEx failed")
	}
	if status.TotalPhys == 0 {
		return MemoryInfo{}, fmt.Errorf("total RAM reported as zero")
	}

	return MemoryInfo{
		TotalRAMBytes:     status.TotalPhys,
		AvailableRAMBytes: status.AvailPhys,
		AvailableRAMKnown: true,
	}, nil
}

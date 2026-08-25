//go:build windows

package hardware

import (
	"syscall"
	"unsafe"
)

// relationProcessorCore identifies a physical core entry in
// SYSTEM_LOGICAL_PROCESSOR_INFORMATION (LOGICAL_PROCESSOR_RELATIONSHIP).
const relationProcessorCore = 3

var (
	kernel32                           = syscall.NewLazyDLL("kernel32.dll")
	procGetLogicalProcessorInformation = kernel32.NewProc("GetLogicalProcessorInformation")
)

// systemLogicalProcessorInformation mirrors
// SYSTEM_LOGICAL_PROCESSOR_INFORMATION. The union is represented as its
// raw 16-byte payload because only Relationship is needed here.
type systemLogicalProcessorInformation struct {
	ProcessorMask uintptr
	Relationship  uint32
	_             uint32
	Payload       [2]uint64
}

// physicalCores counts physical CPU cores via
// GetLogicalProcessorInformation. ok is false when the Windows API is
// unavailable or fails; the count is never fabricated.
func physicalCores() (n int, ok bool) {
	if procGetLogicalProcessorInformation.Find() != nil {
		return 0, false
	}

	var needed uint32
	ret, _, _ := procGetLogicalProcessorInformation.Call(0, uintptr(unsafe.Pointer(&needed)))
	if ret != 0 || needed == 0 {
		return 0, false
	}

	buf := make([]byte, needed)
	ret, _, _ = procGetLogicalProcessorInformation.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&needed)),
	)
	if ret == 0 || needed < uint32(unsafe.Sizeof(systemLogicalProcessorInformation{})) {
		return 0, false
	}

	count := needed / uint32(unsafe.Sizeof(systemLogicalProcessorInformation{}))
	entries := unsafe.Slice((*systemLogicalProcessorInformation)(unsafe.Pointer(&buf[0])), count)
	for i := range entries {
		if entries[i].Relationship == relationProcessorCore && entries[i].ProcessorMask != 0 {
			n++
		}
	}
	return n, n > 0
}

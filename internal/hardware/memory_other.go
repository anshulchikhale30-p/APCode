//go:build !windows && !linux && !darwin

package hardware

import "fmt"

// readMemory has no implementation on this platform. The caller treats
// this as "unknown" rather than fabricating a value.
func readMemory() (MemoryInfo, error) {
	return MemoryInfo{}, fmt.Errorf("memory detection not supported on this platform")
}

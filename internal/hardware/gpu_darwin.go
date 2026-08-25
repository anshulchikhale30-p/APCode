//go:build darwin

package hardware

import "fmt"

// detectGPU has no reliable implementation on macOS without IOKit or
// system_profiler parsing. The caller treats this as "unknown" rather
// than fabricating a value.
func detectGPU() (GPUInfo, error) {
	return GPUInfo{}, fmt.Errorf("GPU detection not implemented on darwin")
}

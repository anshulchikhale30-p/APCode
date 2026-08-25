//go:build !windows && !linux && !darwin

package hardware

import "fmt"

// detectGPU has no implementation on this platform. The caller treats
// this as "unknown" rather than fabricating a value.
func detectGPU() (GPUInfo, error) {
	return GPUInfo{}, fmt.Errorf("GPU detection not supported on this platform")
}

// Package hardware profiles the machine APCode runs on.
//
// The profiler answers "what hardware does this computer have?" and
// nothing more: model recommendation is a separate future subsystem.
//
// Architecture:
//
//	Hardware detection (platform-specific probes)
//	        ↓
//	HardwareProfile (this package's data structures)
//	        ↓
//	Hardware tier (ClassifyTier, resource-based rules only)
//
// Detection must NEVER prevent APCode from starting. Optional
// information that cannot be detected (physical cores, available RAM,
// GPU) is represented as unknown and recorded in DetectionErrors.
package hardware

import (
	"fmt"
	"runtime"
)

// Platform probes, declared as variables so tests can substitute fakes
// without depending on the developer's actual machine.
var (
	probePhysicalCores = physicalCores // func() (int, bool)
	probeMemory        = readMemory    // func() (MemoryInfo, error)
	probeGPU           = detectGPU     // func() (GPUInfo, error)
)

// Detect collects a HardwareProfile for the current machine.
//
// Detect always returns a usable profile; the returned error is always
// nil in practice because every optional detection degrades to an
// "unknown" value instead of failing. DetectionErrors records which
// optional probes were unavailable.
func Detect() (HardwareProfile, error) {
	p := HardwareProfile{
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		LogicalCPUs: runtime.NumCPU(),
	}

	if p.LogicalCPUs <= 0 {
		p.DetectionErrors = append(p.DetectionErrors,
			fmt.Sprintf("logical CPU count unavailable (got %d)", p.LogicalCPUs))
	}

	if n, ok := probePhysicalCores(); ok && n > 0 {
		p.PhysicalCores = n
		p.PhysicalCoresKnown = true
	} else {
		p.DetectionErrors = append(p.DetectionErrors,
			"physical cores: not reliably detectable on this platform")
	}

	mem, err := probeMemory()
	if err != nil || mem.TotalRAMBytes == 0 {
		reason := "total RAM unavailable"
		if err != nil {
			reason = err.Error()
		}
		p.DetectionErrors = append(p.DetectionErrors, "RAM: "+reason)
	} else {
		p.TotalRAMBytes = mem.TotalRAMBytes
		p.AvailableRAMBytes = mem.AvailableRAMBytes
		p.AvailableRAMKnown = mem.AvailableRAMKnown
		if mem.AvailableRAMBytes == 0 && mem.AvailableRAMKnown {
			p.DetectionErrors = append(p.DetectionErrors, "RAM: available memory reported as zero")
		}
	}

	gpu, err := probeGPU()
	if err != nil {
		p.GPU = UnknownGPU()
		p.DetectionErrors = append(p.DetectionErrors, "GPU: "+err.Error())
	} else {
		p.GPU = gpu
	}

	return p, nil
}

package hardware

// HardwareProfile describes the machine APCode runs on. It answers only
// "what hardware does this computer have?" — it deliberately carries no
// model recommendations or AI capability information.
//
// All byte counts are explicit units (e.g. TotalRAMBytes) so that
// formatting decisions belong to presentation layers such as the TUI.
type HardwareProfile struct {
	// Operating system name (e.g. "windows", "linux", "darwin").
	OS string

	// CPU architecture (e.g. "amd64", "arm64").
	Arch string

	// LogicalCPUs is the number of logical CPUs (hardware threads).
	LogicalCPUs int

	// PhysicalCores is the number of physical CPU cores.
	// PhysicalCoresKnown reports whether that number could be detected
	// reliably; it is never fabricated.
	PhysicalCores      int
	PhysicalCoresKnown bool

	// System memory in explicit units.
	TotalRAMBytes uint64

	// AvailableRAMBytes is currently free memory, when the platform
	// exposes it. AvailableRAMKnown reports whether it was detected.
	AvailableRAMBytes uint64
	AvailableRAMKnown bool

	// GPU information, when reliably detectable.
	GPU GPUInfo

	// DetectionErrors records optional detections that failed. The
	// profile remains valid and usable; errors are informational.
	DetectionErrors []string
}

// GPUInfo describes a single graphics adapter.
//
// Known reports whether a GPU was actually detected. When detection is
// unavailable or fails, UnknownGPU is used instead — vendor, name, and
// VRAM are never guessed.
type GPUInfo struct {
	Vendor string

	Name string

	// VRAMBytes is the adapter memory size when reported by the OS.
	VRAMBytes uint64
	VRAMKnown bool

	// Known reports whether any GPU information was detected at all.
	Known bool
}

// UnknownGPU returns the canonical representation for "no GPU detected".
func UnknownGPU() GPUInfo {
	return GPUInfo{}
}

// MemoryInfo describes system RAM in explicit units.
type MemoryInfo struct {
	TotalRAMBytes uint64

	AvailableRAMBytes uint64
	AvailableRAMKnown bool
}

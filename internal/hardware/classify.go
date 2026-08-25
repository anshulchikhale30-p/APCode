package hardware

// Tier is a preliminary classification of the machine's raw resources.
//
// It is NOT an AI capability score. It does not predict model
// performance and must never be presented as one. The rules below use
// only detected hardware resources (CPU and RAM) so that they can be
// replaced later by benchmark-based scoring without touching callers.
type Tier string

const (
	// TierLow marks resource-constrained machines.
	TierLow Tier = "LOW"

	// TierBalanced marks mainstream machines.
	TierBalanced Tier = "BALANCED"

	// TierHigh marks well-equipped machines.
	TierHigh Tier = "HIGH"

	// TierUnknown means there was not enough reliable information to
	// classify the machine.
	TierUnknown Tier = "UNKNOWN"
)

const (
	gib = 1 << 30
)

// ClassifyTier applies the hardware tier rules to a profile.
//
// Rules (resource-based only):
//
//	UNKNOWN : logical CPU count or total RAM could not be detected
//	LOW     : at most 4 logical CPUs, OR less than 8 GiB total RAM
//	HIGH    : at least 12 logical CPUs AND at least 16 GiB total RAM
//	BALANCED: everything else
//
// GPU information is deliberately ignored: vendor/VRAM presence says
// nothing about accelerators without dedicated capability checks.
func ClassifyTier(p HardwareProfile) Tier {
	if p.LogicalCPUs <= 0 || p.TotalRAMBytes == 0 {
		return TierUnknown
	}
	switch {
	case p.LogicalCPUs <= 4 || p.TotalRAMBytes < 8*gib:
		return TierLow
	case p.LogicalCPUs >= 12 && p.TotalRAMBytes >= 16*gib:
		return TierHigh
	default:
		return TierBalanced
	}
}

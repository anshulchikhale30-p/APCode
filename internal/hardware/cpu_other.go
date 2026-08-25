//go:build !windows && !linux && !darwin

package hardware

// physicalCores has no reliable implementation on this platform. The
// caller treats this as "unknown" rather than fabricating a value.
func physicalCores() (int, bool) {
	return 0, false
}

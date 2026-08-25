//go:build linux

package hardware

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// readMemory parses /proc/meminfo. MemTotal is always present;
// MemAvailable exists on kernels 3.14+, so availability stays unknown
// when the field is missing.
func readMemory() (MemoryInfo, error) {
	b, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return MemoryInfo{}, fmt.Errorf("cannot read /proc/meminfo: %w", err)
	}

	var info MemoryInfo
	fields := make(map[string]uint64)
	for _, line := range strings.Split(string(b), "\n") {
		name, value, ok := parseMeminfoLine(line)
		if ok {
			fields[name] = value
		}
	}

	total, ok := fields["MemTotal"]
	if !ok || total == 0 {
		return MemoryInfo{}, fmt.Errorf("MemTotal missing from /proc/meminfo")
	}
	info.TotalRAMBytes = total * 1024 // meminfo reports kibibytes

	if avail, ok := fields["MemAvailable"]; ok {
		info.AvailableRAMBytes = avail * 1024
		info.AvailableRAMKnown = true
	}

	return info, nil
}

// parseMeminfoLine splits lines like "MemTotal:       16384256 kB".
func parseMeminfoLine(line string) (name string, kbValue uint64, ok bool) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", 0, false
	}
	name = strings.TrimSuffix(parts[0], ":")
	v, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return "", 0, false
	}
	return name, v, true
}

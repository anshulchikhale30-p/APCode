//go:build linux

package hardware

import (
	"os"
	"path/filepath"
	"strings"
)

// physicalCores counts unique (physical package, core) pairs from the
// sysfs CPU topology. ok is false when the topology files are missing
// or unreadable; the count is never fabricated.
func physicalCores() (int, bool) {
	cpuDirs, err := filepath.Glob("/sys/devices/system/cpu/cpu[0-9]*/topology")
	if err != nil || len(cpuDirs) == 0 {
		return 0, false
	}

	seen := make(map[string]bool)
	for _, dir := range cpuDirs {
		pkg, ok := readSysfsTrim(dir, "physical_package_id")
		if !ok {
			continue
		}
		core, ok := readSysfsTrim(dir, "core_id")
		if !ok {
			continue
		}
		seen[pkg+":"+core] = true
	}

	if len(seen) == 0 {
		return 0, false
	}
	return len(seen), true
}

func readSysfsTrim(dir, name string) (string, bool) {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return "", false
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", false
	}
	return s, true
}

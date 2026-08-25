//go:build linux

package hardware

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// detectGPU scans /sys/class/drm for GPU devices and reads vendor,
// device name, and VRAM from sysfs.
func detectGPU() (GPUInfo, error) {
	cardDirs, err := filepath.Glob("/sys/class/drm/card[0-9]*")
	if err != nil || len(cardDirs) == 0 {
		return GPUInfo{}, fmt.Errorf("no DRM cards found in /sys/class/drm")
	}

	for _, cardDir := range cardDirs {
		if gpu, ok := probeCard(cardDir); ok {
			return gpu, nil
		}
	}

	return GPUInfo{}, fmt.Errorf("no GPU with detectable info found")
}

func probeCard(cardDir string) (GPUInfo, bool) {
	// Read vendor ID from device/of_node or use pci id
	vendor := readGPUVendor(cardDir)
	name := readGPUName(cardDir)
	vramBytes, vramKnown := readGPUVRAM(cardDir)

	if vendor == "" && name == "" {
		return GPUInfo{}, false
	}

	return GPUInfo{
		Vendor:    vendor,
		Name:      name,
		VRAMBytes: vramBytes,
		VRAMKnown: vramKnown,
		Known:     true,
	}, true
}

func readGPUVendor(cardDir string) string {
	// Try PCI vendor ID first
	deviceDir := filepath.Join(cardDir, "device")
	vendorPath := filepath.Join(deviceDir, "vendor")
	if b, err := os.ReadFile(vendorPath); err == nil {
		v := strings.TrimSpace(string(b))
		if v != "" {
			// Convert hex vendor ID to name if known
			return pciVendorName(v)
		}
	}
	// Fallback: try of_node for platform devices
	ofNode := filepath.Join(deviceDir, "of_node", "compatible")
	if b, err := os.ReadFile(ofNode); err == nil {
		return strings.TrimSpace(string(b))
	}
	return ""
}

func readGPUName(cardDir string) string {
	// Try PCI device ID
	deviceDir := filepath.Join(cardDir, "device")
	devicePath := filepath.Join(deviceDir, "device")
	if b, err := os.ReadFile(devicePath); err == nil {
		v := strings.TrimSpace(string(b))
		if v != "" {
			return "PCI Device " + v
		}
	}
	// Try to read the card label
	labelPath := filepath.Join(cardDir, "device", "uevent")
	if b, err := os.ReadFile(labelPath); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "DRIVER=") {
				return strings.TrimPrefix(line, "DRIVER=")
			}
		}
	}
	return ""
}

func readGPUVRAM(cardDir string) (uint64, bool) {
	// VRAM info is in /sys/class/drm/card*/device/mem_info_vram_total
	// or /sys/kernel/debug/dri/*/amdgpu_vram_mm etc.
	// For now, check the mem_info_vram_total file (AMD/Intel)
	memInfoPath := filepath.Join(cardDir, "device", "mem_info_vram_total")
	if b, err := os.ReadFile(memInfoPath); err == nil {
		v := strings.TrimSpace(string(b))
		if v != "" {
			if bytes, err := strconv.ParseUint(v, 10, 64); err == nil {
				return bytes, true
			}
		}
	}
	// Also check for NVIDIA (may be in different location)
	// NVIDIA often doesn't expose VRAM via sysfs easily without nvidia-smi
	return 0, false
}

func pciVendorName(hexID string) string {
	// Common GPU vendor IDs
	vendors := map[string]string{
		"0x1002": "AMD",
		"0x10de": "NVIDIA",
		"0x8086": "Intel",
		"0x13b5": "ARM (Mali)",
		"0x10ee": "Xilinx",
		"0x1013": "Cirrus Logic",
		"0x1a03": "ASPEED",
		"0x1af4": "Red Hat (VirtIO GPU)",
	}
	// Normalize: ensure 0x prefix
	if !strings.HasPrefix(hexID, "0x") {
		hexID = "0x" + hexID
	}
	if name, ok := vendors[strings.ToLower(hexID)]; ok {
		return name
	}
	return "PCI Vendor " + hexID
}

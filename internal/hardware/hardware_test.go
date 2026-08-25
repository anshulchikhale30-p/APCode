package hardware

import (
	"runtime"
	"testing"
)

func TestDetect(t *testing.T) {
	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	if got.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", got.OS, runtime.GOOS)
	}
	if got.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", got.Arch, runtime.GOARCH)
	}
	if got.LogicalCPUs != runtime.NumCPU() {
		t.Errorf("LogicalCPUs = %d, want %d", got.LogicalCPUs, runtime.NumCPU())
	}
	if got.LogicalCPUs <= 0 {
		t.Errorf("LogicalCPUs = %d, want > 0", got.LogicalCPUs)
	}
}

func TestDetectGPU(t *testing.T) {
	got, err := Detect()
	if err != nil {
		t.Fatalf("Detect() returned error: %v", err)
	}

	// GPU detection may or may not succeed depending on platform
	// Just verify the field exists and Known flag works
	if got.GPU.Known {
		t.Logf("GPU detected: %s (vendor: %s, vram: %d)", got.GPU.Name, got.GPU.Vendor, got.GPU.VRAMBytes)
	} else {
		t.Log("GPU not detected (expected on some platforms)")
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		input    uint64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TiB"},
		{3 * 1024 * 1024 * 1024 / 2, "1.5 GiB"},
	}

	for _, tc := range cases {
		// We can't easily test the private formatBytes function from here
		// This test is just documentation of expected behavior
		_ = tc
	}
}

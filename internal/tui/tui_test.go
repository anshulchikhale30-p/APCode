package tui

import (
	"bytes"
	"strings"
	"testing"

	"apcode/internal/config"
	"apcode/internal/hardware"
)

func disableColors() func() {
	prev := ColorsEnabled()
	SetColorsEnabled(false)
	return func() { SetColorsEnabled(prev) }
}

func TestBannerContainsBrandLines(t *testing.T) {
	b := Banner()
	for _, want := range []string{
		"█████╗ ██████╗  ██████╗ ██████╗ ██████╗ ███████╗",
		"We care about your system.",
		"So you can focus on your ideas.",
		"Making the most of every bit of your laptop.",
	} {
		if !strings.Contains(b, want) {
			t.Errorf("banner missing %q", want)
		}
	}
}

func TestPrintWelcome(t *testing.T) {
	defer disableColors()()

	var buf bytes.Buffer
	PrintWelcome(&buf, hardware.HardwareProfile{
		OS:            "testos",
		Arch:          "testarch",
		LogicalCPUs:   8,
		TotalRAMBytes: 16 * 1024 * 1024 * 1024, // 16 GiB
		GPU: hardware.GPUInfo{
			Vendor:    "NVIDIA",
			Name:      "RTX 4090",
			VRAMBytes: 24 * 1024 * 1024 * 1024,
			VRAMKnown: true,
			Known:     true,
		},
	})

	out := buf.String()
	for _, want := range []string{
		"╚═╝  ╚═╝╚═╝      ╚═════╝ ╚═════╝ ╚═════╝ ╚══════╝",
		"We care about your system.",
		"Operating system : testos",
		"CPU architecture : testarch",
		"CPU threads      : 8",
		"Total RAM        : 16.0 GiB",
		"GPU              : RTX 4090 (NVIDIA) - 24.0 GiB VRAM",
		"APCode version   : " + config.Version,
		"Offline mode     : enabled",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("welcome output missing %q", want)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("welcome output contains escape sequences with colors disabled")
	}
}

func TestPrintWelcomeMinimal(t *testing.T) {
	defer disableColors()()

	var buf bytes.Buffer
	PrintWelcome(&buf, hardware.HardwareProfile{
		OS:          "testos",
		Arch:        "testarch",
		LogicalCPUs: 4,
	})

	out := buf.String()
	for _, want := range []string{
		"Operating system : testos",
		"CPU architecture : testarch",
		"CPU threads      : 4",
		"GPU              : unknown",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("welcome output missing %q", want)
		}
	}
}

func TestPrintWelcomeWithDetectionErrors(t *testing.T) {
	defer disableColors()()

	var buf bytes.Buffer
	PrintWelcome(&buf, hardware.HardwareProfile{
		OS:              "testos",
		Arch:            "testarch",
		LogicalCPUs:     4,
		DetectionErrors: []string{"physical cores: not reliably detectable on this platform", "GPU: detection not implemented on darwin"},
	})

	out := buf.String()
	if !strings.Contains(out, "⚠ physical cores: not reliably detectable on this platform") {
		t.Error("detection error not shown in output")
	}
	if !strings.Contains(out, "⚠ GPU: detection not implemented on darwin") {
		t.Error("GPU detection error not shown in output")
	}
}

func TestStyleWithColorsDisabled(t *testing.T) {
	defer disableColors()()

	const s = "hello"
	for name, fn := range map[string]func(string) string{
		"Primary":   Primary,
		"Secondary": Secondary,
		"Success":   Success,
		"Warning":   Warning,
		"Error":     Error,
		"Muted":     Muted,
	} {
		if got := fn(s); got != s {
			t.Errorf("%s with colors disabled = %q, want %q", name, got, s)
		}
	}
	if Reset() != "" {
		t.Errorf("Reset with colors disabled = %q, want empty", Reset())
	}
}

func TestStyleWithColorsEnabled(t *testing.T) {
	prev := ColorsEnabled()
	SetColorsEnabled(true)
	defer func() { SetColorsEnabled(prev) }()
	prevBg := GetBackgroundColor()
	ClearBackgroundColor()
	defer func() {
		if prevBg != "" {
			_ = SetBackgroundColor(prevBg)
		} else {
			_ = SetBackgroundColor(defaultBackgroundHex)
		}
	}()

	cases := []struct {
		name string
		code string
		fn   func(string) string
	}{
		{"Primary", ansiPrimary, Primary},
		{"Secondary", ansiSecondary, Secondary},
		{"Success", ansiSuccess, Success},
		{"Warning", ansiWarning, Warning},
		{"Error", ansiError, Error},
		{"Muted", ansiMuted, Muted},
	}
	for _, tc := range cases {
		want := tc.code + "hello" + ansiReset
		if got := tc.fn("hello"); got != want {
			t.Errorf("%s = %q, want %q", tc.name, got, want)
		}
	}
	if Reset() != ansiReset {
		t.Errorf("Reset = %q, want %q", Reset(), ansiReset)
	}
}

func TestStyleEmptyString(t *testing.T) {
	prev := ColorsEnabled()
	SetColorsEnabled(true)
	defer func() { SetColorsEnabled(prev) }()

	if got := Primary(""); got != "" {
		t.Errorf("Primary(\"\") = %q, want empty", got)
	}
}

func TestPrintWelcomeColored(t *testing.T) {
	prev := ColorsEnabled()
	SetColorsEnabled(true)
	defer func() { SetColorsEnabled(prev) }()

	var buf bytes.Buffer
	PrintWelcome(&buf, hardware.HardwareProfile{OS: "testos", Arch: "testarch", LogicalCPUs: 4})

	out := buf.String()
	if !strings.Contains(out, ansiSuccess+"enabled"+ansiReset) {
		t.Error("offline mode value not styled with success color")
	}
	if !strings.Contains(out, ansiMuted+"Operating system :"+ansiReset) {
		t.Error("info label not styled with muted color")
	}
	if !strings.Contains(out, "testos") || !strings.Contains(out, "testarch") {
		t.Error("plain values missing from colored output")
	}
}

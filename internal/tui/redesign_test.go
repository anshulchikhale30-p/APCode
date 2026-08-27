package tui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"apcode/internal/config"
)

func TestWelcomeScreenFullLayout(t *testing.T) {
	defer disableColors()()

	out := WelcomeScreen(WelcomeOptions{
		Version:     config.Version,
		Commands:    DefaultMenuCommands(),
		ProjectLine: "Go · 42 files · Git: main",
		Width:       100,
		HasModel:    false,
		Workspace:   "/home/user/APCode",
		GitBranch:   "main",
	})

	// Narrow palette + OpenCode-inspired hierarchy: logo, version, repo summary
	for _, want := range []string{
		"█████╗ ██████╗",
		"v" + config.Version,
		"Go · 42 files · Git: main",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("welcome output missing %q", want)
		}
	}
	// Two-row bordered input box with placeholder (row 1) and status segments (row 2)
	for _, want := range []string{
		"╭", "╰",
		"Ask anything...",
		"Fix broken tests",
		"native",
		"no model installed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("welcome input box missing %q", want)
		}
	}
	// Centered keybind hints — keys brighter than descs, generous spacing
	for _, want := range []string{"enter", "send", "ctrl+p", "commands", "tab", "agents"} {
		if !strings.Contains(out, want) {
			t.Errorf("welcome keybind hints missing %q", want)
		}
	}
	// Tip line: amber dot + bold Tip, context-aware for no model
	for _, want := range []string{"●", "Tip", "/models"} {
		if !strings.Contains(out, want) {
			t.Errorf("welcome tip missing %q", want)
		}
	}
	// Status bar pinned bottom: dir:branch left, version right, dim gray
	if !strings.Contains(out, "v"+config.Version) {
		t.Errorf("welcome status bar missing version %q", "v"+config.Version)
	}
	// Welcome should NOT front-load the static slash-command list (moved to palette)
	for _, bad := range []string{"/help       show help", "/new        new session"} {
		if strings.Contains(out, bad) {
			t.Errorf("welcome should not contain static command list %q (moved to palette)", bad)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Error("welcome output contains escape sequences with colors disabled")
	}
}

func TestWelcomeScreenCompactLayout(t *testing.T) {
	defer disableColors()()

	full := WelcomeScreen(WelcomeOptions{Version: "1.0.0", Width: 100})
	compact := WelcomeScreen(WelcomeOptions{Version: "1.0.0", Width: 60})

	if !strings.Contains(compact, "v1.0.0") {
		t.Error("compact welcome missing version")
	}
	if strings.Contains(compact, "█████╗") {
		t.Error("compact layout must not use the full-size logo")
	}
	if strings.Contains(full, "▀▀▀ ▀▀▀▀") {
		t.Error("full layout must not use the compact logo")
	}

	for _, line := range strings.Split(compact, "\n") {
		if visibleWidth(line) > 60 {
			t.Errorf("compact welcome line exceeds width: %q", line)
		}
	}
}

func TestWelcomeScreenNeverOverflows(t *testing.T) {
	defer disableColors()()

	for _, width := range []int{40, 60, 79, 80, 100, 120, 160} {
		out := WelcomeScreen(WelcomeOptions{
			Version:     config.Version,
			Commands:    DefaultMenuCommands(),
			ProjectLine: ProjectLine("Python", 24, true, "main"),
			Width:       width,
		})
		for _, line := range strings.Split(out, "\n") {
			if visibleWidth(line) > width {
				t.Errorf("width %d: line overflows: %q (%d cols)", width, line, visibleWidth(line))
			}
		}
	}
}

func TestCommandMenuAlignment(t *testing.T) {
	defer disableColors()()

	cmds := []Command{
		{Name: "/help", Description: "show help"},
		{Name: "/newsessionlonger", Description: "longer name", Shortcut: "Ctrl+C"},
	}
	out := CommandMenu(cmds, 80)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	// Descriptions start at the same column on every line.
	d1 := strings.Index(lines[0], "show help")
	d2 := strings.Index(lines[1], "longer name")
	if d1 != d2 {
		t.Errorf("descriptions not aligned: %d vs %d", d1, d2)
	}
	if !strings.Contains(lines[1], "Ctrl+C") {
		t.Error("shortcut missing from menu entry")
	}
}

func TestFooterHints(t *testing.T) {
	defer disableColors()()

	out := FooterHints("Enter send", "Local Model", 40)
	l := visibleWidth(out)
	if l > 40 || l < 20 {
		t.Errorf("footer width %d out of range for terminal 40", l)
	}
	if !strings.Contains(out, "Enter send") || !strings.Contains(out, "Local Model") {
		t.Errorf("footer missing hints: %q", out)
	}
	// Too narrow: right hint dropped instead of overflowing.
	narrow := FooterHints("a very long left hint that is long", "right", 30)
	if strings.Contains(narrow, "right") {
		t.Error("right hint should be dropped on narrow terminals")
	}
}

func TestInputBoxRendering(t *testing.T) {
	defer disableColors()()

	box := InputBox(50, "Fix the authentication bug")
	lines := strings.Split(box, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (top/body/bottom), got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "╭") || !strings.HasSuffix(lines[0], "╮") {
		t.Errorf("bad top border: %q", lines[0])
	}
	if !strings.HasPrefix(lines[2], "╰") || !strings.HasSuffix(lines[2], "╯") {
		t.Errorf("bad bottom border: %q", lines[2])
	}
	if !strings.Contains(lines[1], "› Fix the authentication bug") {
		t.Errorf("prompt text missing: %q", lines[1])
	}
	for i, l := range lines {
		if visibleWidth(l) != 50 {
			t.Errorf("line %d width = %d, want 50", i, visibleWidth(l))
		}
	}
}

func TestInputBoxInteractiveParts(t *testing.T) {
	defer disableColors()()

	top := InputBoxTop(62)
	bottom := InputBoxBottom(62)
	prefix := InputBoxPrefix()
	if visibleWidth(top) != 62 || visibleWidth(bottom) != 62 {
		t.Errorf("border widths wrong: top=%d bottom=%d", visibleWidth(top), visibleWidth(bottom))
	}
	if !strings.Contains(prefix, "›") {
		t.Errorf("prefix missing prompt mark: %q", prefix)
	}
	if w := BoxWidth(200); w > 76 {
		t.Errorf("BoxWidth clamped to %d, want <= 76", w)
	}
	if w := BoxWidth(20); w < 24 {
		t.Errorf("BoxWidth floor violated: %d", w)
	}
}

func TestActivityLines(t *testing.T) {
	defer disableColors()()

	cases := []struct {
		kind ActivityKind
		want string
	}{
		{ActivityWorking, "◐ Analyzing project..."},
		{ActivitySuccess, "✓ Found 24 files"},
		{ActivityWarning, "⚠ low memory"},
		{ActivityError, "✗ tests failed"},
		{ActivityAction, "→ searching code"},
	}
	for _, c := range cases {
		msg := strings.TrimSpace(strings.TrimLeft(c.want, "◐✓⚠✗→ "))
		got := ActivityLine(c.kind, msg)
		if !strings.Contains(got, c.want) {
			t.Errorf("activity line for %q = %q", c.want, got)
		}
		if !strings.HasPrefix(got, "  ") {
			t.Errorf("activity line not indented: %q", got)
		}
	}
}

func TestFileChangeAndDiff(t *testing.T) {
	defer disableColors()()

	fc := FileChange("src/auth/session.py")
	if !strings.Contains(fc, "✎") || !strings.Contains(fc, "src/auth/session.py") {
		t.Errorf("file change marker missing: %q", fc)
	}

	diff := RenderDiff("+added\n-removed\n@@ -1 +1 @@\n context\n+++ b/file.go\n--- a/file.go")
	for _, want := range []string{"+added", "-removed", "@@ -1 +1 @@", " context", "+++ b/file.go", "--- a/file.go"} {
		if !strings.Contains(diff, want) {
			t.Errorf("diff missing %q", want)
		}
	}
	if strings.Contains(diff, "\x1b[") {
		t.Error("no-color diff contains escape sequences")
	}
}

func TestResponseBlock(t *testing.T) {
	defer disableColors()()

	block := ResponseBlock("The login issue has been fixed.", 80)
	if !strings.Contains(block, "The login issue has been fixed.") {
		t.Errorf("response text missing: %q", block)
	}
	if strings.Count(block, "─") < 2*RuleWidth(80)-10 {
		t.Error("expected divider rules around response")
	}
	for _, line := range strings.Split(block, "\n") {
		if visibleWidth(line) > RuleWidth(80)+2 {
			t.Errorf("response line too wide: %q", line)
		}
	}
}

func TestWrapText(t *testing.T) {
	wrapped := WrapText("one two three four five six seven", 10)
	for _, line := range wrapped {
		if visibleWidth(line) > 10 {
			t.Errorf("wrapped line too wide: %q", line)
		}
	}
	if got := WrapText("", 10); len(got) != 1 || got[0] != "" {
		t.Errorf("empty wrap = %v", got)
	}
	long := WrapText("superduperlongwordwithoutspaces", 8)
	if len(long) != 1 || long[0] != "superduperlongwordwithoutspaces" {
		t.Errorf("long word should not be broken: %v", long)
	}
}

func TestToolSummary(t *testing.T) {
	defer disableColors()()

	out := ToolSummary("read_file", map[string]string{"path": "main.go"})
	if !strings.Contains(out, "read_file") || !strings.Contains(out, "path=main.go") {
		t.Errorf("tool summary malformed: %q", out)
	}
	big := ToolSummary("write_file", map[string]string{"content": strings.Repeat("x", 100)})
	if len(big) > 120 {
		t.Errorf("tool summary not truncated: %d chars", len(big))
	}
}

func TestSpinnerLifecycleSilent(t *testing.T) {
	var buf bytes.Buffer
	sp := NewSpinner(&buf, "Working...", false)
	sp.Mode = SpinnerOff
	sp.Start()
	if sp.Active() {
		t.Error("disabled spinner should never become active")
	}
	sp.Stop() // must be safe when never started
	if buf.Len() != 0 {
		t.Errorf("disabled spinner wrote output: %q", buf.String())
	}
}

func TestSpinnerLifecycleAnimated(t *testing.T) {
	prev := ColorsEnabled()
	SetColorsEnabled(true)
	defer func() { SetColorsEnabled(prev) }()

	var buf bytes.Buffer
	sp := &Spinner{
		Out:      &buf,
		Message:  "Analyzing project...",
		Frames:   defaultFrames,
		Interval: time.Millisecond,
		Mode:     SpinnerOn,
		Ctx:      context.Background(),
	}
	sp.Start()
	if !sp.Active() {
		t.Fatal("spinner should be active after Start")
	}
	sp.Start() // idempotent
	sp.Stop()
	sp.Stop() // idempotent
	if sp.Active() {
		t.Error("spinner still active after Stop")
	}
	out := buf.String()
	if !strings.Contains(out, "Analyzing project...") {
		t.Error("spinner never rendered its message")
	}
	if strings.Contains(out, "\n") {
		t.Error("spinner corrupted the terminal with newlines")
	}
	if !strings.HasSuffix(out, "\r\r") && !strings.HasSuffix(out, "\r") {
		t.Errorf("spinner did not clear its line; tail=%q", out[max(0, len(out)-12):])
	}
}

func TestModelIndicator(t *testing.T) {
	cases := []struct{ rt, model, want string }{
		{"llama.cpp", "Phi-3 Mini Q4", "llama.cpp · Phi-3 Mini Q4"},
		{"", "Phi-3 Mini Q4", "Local · Phi-3 Mini Q4"},
		{"native", "", "native · no model installed"},
		{"", "", GlyphWarning + " No local model installed"},
	}
	for _, c := range cases {
		if got := ModelIndicator(c.rt, c.model); got != c.want {
			t.Errorf("ModelIndicator(%q,%q) = %q, want %q", c.rt, c.model, got, c.want)
		}
	}
}

func TestProjectLine(t *testing.T) {
	cases := []struct {
		lang   string
		files  int
		git    bool
		branch string
		want   string
	}{
		{"Python", 24, true, "main", "Python · 24 files · Git: main"},
		{"Go", 42, true, "", "Go · 42 files · Git: clean"},
		{"Python", 1, false, "", "Python · 1 file · Git: not initialized"},
		{"unknown", 0, false, "", "Empty directory"},
	}
	for _, c := range cases {
		if got := ProjectLine(c.lang, c.files, c.git, c.branch); got != c.want {
			t.Errorf("ProjectLine(%q,%d,%v,%q) = %q, want %q", c.lang, c.files, c.git, c.branch, got, c.want)
		}
	}
}

func TestRenderStatus(t *testing.T) {
	defer disableColors()()

	var buf bytes.Buffer
	RenderStatus(&buf, StatusData{
		Version:         "9.9.9",
		ProjectLanguage: "Go",
		ProjectFiles:    42,
		GitRepo:         true,
		GitBranch:       "main",
		GitChanges:      "3 modified",
		OS:              "Windows",
		CPU:             "12 threads",
		RAM:             "15.3 GiB",
		GPU:             "AMD Radeon",
		RuntimeName:     "Native",
		RuntimeModel:    "Phi-3 Mini Q4",
		RuntimeState:    "Ready",
		RuntimeReady:    true,
	})

	out := buf.String()
	for _, want := range []string{
		"APCode Status",
		"PROJECT", "Language", "Go", "Files", "42", "Git", "main", "Changes", "3 modified",
		"SYSTEM", "OS", "Windows", "CPU", "12 threads", "RAM", "15.3 GiB", "GPU", "AMD Radeon",
		"RUNTIME", "Runtime", "Native", "Model", "Phi-3 Mini Q4", "Status", "Ready",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q", want)
		}
	}

	buf.Reset()
	RenderStatus(&buf, StatusData{})
	out = buf.String()
	for _, want := range []string{"not initialized", "none detected", "no local model installed"} {
		if !strings.Contains(out, want) {
			t.Errorf("empty status output missing fallback %q", want)
		}
	}
}

func TestLayoutForWidth(t *testing.T) {
	cases := []struct {
		width int
		want  LayoutMode
	}{
		{40, LayoutCompact},
		{79, LayoutCompact},
		{80, LayoutNormal},
		{119, LayoutNormal},
		{120, LayoutExpanded},
		{200, LayoutExpanded},
	}
	for _, c := range cases {
		if got := LayoutForWidth(c.width); got != c.want {
			t.Errorf("LayoutForWidth(%d) = %v, want %v", c.width, got, c.want)
		}
	}
}

func TestTerminalWidthFallback(t *testing.T) {
	// In test binaries stdout may or may not be a console; either way this
	// must return a sane positive number and never panic.
	if w := TerminalWidth(); w <= 0 || w > 1000 {
		t.Errorf("TerminalWidth() = %d, want sane positive value", w)
	}
}

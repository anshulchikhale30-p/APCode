package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"apcode/internal/tui"
)

func newUIREPL(t *testing.T) (*REPL, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	repl, err := NewREPL(bytes.NewBufferString(""), out, errOut)
	if err != nil {
		t.Fatalf("NewREPL failed: %v", err)
	}
	return repl, out, errOut
}

func TestWelcomeScreenOutput(t *testing.T) {
	prev := tui.ColorsEnabled()
	tui.SetColorsEnabled(false)
	defer func() { tui.SetColorsEnabled(prev) }()

	repl, out, _ := newUIREPL(t)
	repl.printWelcome()

	o := out.String()
	// Woven logo + version + repo summary (muted hierarchy) — not slash list
	for _, want := range []string{
		"APCode",
		"Ask anything...",
		"Fix broken tests",
		"╭", "╰",
		"enter", "send",
		"ctrl+p", "commands",
		"●", "Tip",
	} {
		if !strings.Contains(o, want) {
			t.Errorf("welcome output missing %q", want)
		}
	}
	// Slash list moved to palette/help, not front-loaded on welcome
	if strings.Contains(o, "/help       show help") {
		t.Errorf("welcome should not front-load slash list (moved to palette)")
	}
}

func TestProjectLineFromState(t *testing.T) {
	repl, _, _ := newUIREPL(t)
	line := repl.projectLine()
	if line == "" {
		t.Error("project line should never be empty")
	}

	repl.IsGitRepo = true
	repl.GitBranch = "main"
	if !strings.Contains(repl.projectLine(), "Git: main") {
		t.Errorf("expected git branch in project line: %q", repl.projectLine())
	}
}

func TestModelIndicatorText(t *testing.T) {
	repl, _, _ := newUIREPL(t)

	// No model must never be reported as available.
	repl.Model = nil
	repl.Runtime = nil
	repl.RuntimeName = ""
	if got := repl.modelIndicatorText(); !strings.Contains(got, "No local model installed") && !strings.Contains(got, "no model installed") {
		t.Errorf("unexpected indicator without model: %q", got)
	}

	repl.RuntimeName = "native"
	if got := repl.modelIndicatorText(); !strings.Contains(got, "native") {
		t.Errorf("runtime name missing from indicator: %q", got)
	}
}

func TestHandleNewSession(t *testing.T) {
	prev := tui.ColorsEnabled()
	tui.SetColorsEnabled(false)
	defer func() { tui.SetColorsEnabled(prev) }()

	repl, out, _ := newUIREPL(t)
	repl.History = []Message{{Role: "user", Content: "x"}, {Role: "assistant", Content: "y"}}
	repl.lastPlan = "1. do stuff"
	repl.handleNewSession()
	if len(rHistory(repl)) != 0 {
		t.Error("history not cleared by /new")
	}
	if repl.lastPlan != "" {
		t.Error("plan not cleared by /new")
	}
	if !strings.Contains(out.String(), "New session started") {
		t.Error("missing confirmation output")
	}
}

// rHistory avoids direct field aliasing confusion in the assertion above.
func rHistory(r *REPL) []Message { return r.History }

func TestHandleSlashCommandNewAndBenchmark(t *testing.T) {
	prev := tui.ColorsEnabled()
	tui.SetColorsEnabled(false)
	defer func() { tui.SetColorsEnabled(prev) }()

	repl, _, _ := newUIREPL(t)
	if shouldExit := repl.handleSlashCommand(context.Background(), "/new"); shouldExit {
		t.Error("/new should not exit")
	}
	// /benchmark must run against the real benchmark package and terminate
	// without panicking regardless of hardware detection results.
	if shouldExit := repl.handleSlashCommand(context.Background(), "/benchmark"); shouldExit {
		t.Error("/benchmark should not exit")
	}
}

func TestStatusScreenOutput(t *testing.T) {
	prev := tui.ColorsEnabled()
	tui.SetColorsEnabled(false)
	defer func() { tui.SetColorsEnabled(prev) }()

	repl, out, _ := newUIREPL(t)
	repl.handleStatus()
	o := out.String()
	for _, want := range []string{"APCode Status", "PROJECT", "SYSTEM", "RUNTIME"} {
		if !strings.Contains(o, want) {
			t.Errorf("status screen missing %q", want)
		}
	}
}

func TestHelpListsOnlyRealCommands(t *testing.T) {
	prev := tui.ColorsEnabled()
	tui.SetColorsEnabled(false)
	defer func() { tui.SetColorsEnabled(prev) }()

	repl, out, _ := newUIREPL(t)
	repl.printHelp()
	o := out.String()
	for _, cmd := range []string{"/help", "/new", "/models", "/status", "/benchmark", "/exit"} {
		if !strings.Contains(o, cmd) {
			t.Errorf("help missing %q", cmd)
		}
	}
}

func TestRunLoopEOFShutsDownCleanly(t *testing.T) {
	prev := tui.ColorsEnabled()
	tui.SetColorsEnabled(false)
	defer func() { tui.SetColorsEnabled(prev) }()

	in := bytes.NewBufferString("")
	out := &bytes.Buffer{}
	repl, err := NewREPL(in, out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("NewREPL failed: %v", err)
	}
	if err := repl.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error on EOF: %v", err)
	}
	if !strings.Contains(out.String(), "Goodbye!") {
		t.Error("expected goodbye message")
	}
	if !strings.Contains(out.String(), "╰") {
		t.Error("input box left open after EOF")
	}
}

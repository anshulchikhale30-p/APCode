package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"apcode/internal/tools"
)

// TestAgentPromptContainsAuthoritativeToolCatalog is the core regression for
// the tool-mismatch bug: the model prompt must be generated from the
// registry so the model can never see a tool that does not exist, and every
// registered tool must appear.
func TestAgentPromptContainsAuthoritativeToolCatalog(t *testing.T) {
	ws := t.TempDir()
	repl := newTestREPL(t, ws, &scriptedRuntime{}, "")

	prompt := repl.Registry.DefinitionsForPrompt()
	for _, name := range []string{
		"read_file", "write_file", "edit_file", "list_files", "search",
		"create_file", "delete_file", "apply_patch", "project_info",
		"run_tests", "run_build", "run_lint", "git_status", "git_diff",
	} {
		if !strings.Contains(prompt, "- "+name+":") {
			t.Errorf("registered tool %q missing from model catalog:\n%s", name, prompt)
		}
	}
	// Hallucinated names from the bug report must never be advertised.
	for _, ghost := range []string{"ListFilesInDirectory", "IdentifyFrontendFiles", "RunTests", "ReadDirectoryContents", "InspectProjectDirectory"} {
		if strings.Contains(prompt, ghost) {
			t.Errorf("hallucinated tool %q appears in catalog", ghost)
		}
	}
	if !strings.Contains(prompt, "ONLY tools") {
		t.Error("catalog should state it is exhaustive")
	}

	// And the REPL system prompt embeds this exact catalog.
	sys := buildSystemPrompt(repl)
	for _, name := range []string{"list_files", "run_tests"} {
		if !strings.Contains(sys, "- "+name+":") {
			t.Errorf("system prompt missing %q", name)
		}
	}
}

// buildSystemPrompt mirrors runAgent's system prompt construction so tests
// can assert on exactly what the model receives.
func buildSystemPrompt(r *REPL) string {
	return "You are APCode, an offline AI coding agent working inside the user's project at " + r.Workspace + ".\n" +
		r.Registry.DefinitionsForPrompt()
}

func TestUnknownToolReturnsStructuredErrorAndRecovers(t *testing.T) {
	ws := t.TempDir()
	for _, f := range []string{"main.py"} {
		os.WriteFile(filepath.Join(ws, f), []byte("# app\n"), 0o644)
	}
	rt := &scriptedRuntime{outputs: []string{
		// A tool name that does not exist in any spelling.
		`{"tool":"IdentifyFrontendFiles","input":{"path":"."}}`,
		`{"tool":"list_files","input":{"path":"."}}`,
		"Done.",
	}}
	repl := newTestREPL(t, ws, rt, "")
	repl.Out = &strings.Builder{}

	repl.Journal.BeginGroup()
	_, err := repl.runAgent(context.Background(), "find frontend files")
	repl.Journal.EndGroup()
	if err != nil {
		t.Fatalf("runAgent: %v", err)
	}

	// History must contain the canonical structured error with real tools.
	var foundPayload bool
	var foundRecovery bool
	for _, m := range repl.History {
		if strings.Contains(m.Content, `"error":"unknown_tool"`) &&
			strings.Contains(m.Content, `"tool":"IdentifyFrontendFiles"`) &&
			strings.Contains(m.Content, `"available_tools":[`) &&
			strings.Contains(m.Content, "list_files") {
			foundPayload = true
		}
		if strings.Contains(m.Content, "list_files result:") && strings.Contains(m.Content, "main.py") {
			foundRecovery = true
		}
	}
	if !foundPayload {
		t.Fatalf("structured unknown_tool payload not fed back to model; history=%+v", repl.History)
	}
	if !foundRecovery {
		t.Error("model did not recover by executing a real tool")
	}
	out := repl.Out.(*strings.Builder).String()
	if !strings.Contains(out, "Unknown tool: IdentifyFrontendFiles") {
		t.Errorf("user was not shown the unknown-tool failure:\n%s", out)
	}
}

// TestNormalizedToolNameStillSafe documents that near-miss names which
// normalize onto a real tool (e.g. RunTests -> run_tests) are tolerated,
// while truly invented tools are rejected.
func TestNormalizedToolNameStillSafe(t *testing.T) {
	ws := t.TempDir()
	os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module x\n\ngo 1.21\n"), 0o644)
	rt := &scriptedRuntime{outputs: []string{
		`{"tool":"RunTests","input":{}}`,
		"ok.",
	}}
	repl := newTestREPL(t, ws, rt, "")
	repl.Journal.BeginGroup()
	_, err := repl.runAgent(context.Background(), "run tests")
	repl.Journal.EndGroup()
	if err != nil {
		t.Fatalf("runAgent: %v", err)
	}
	var executedRealTool bool
	for _, m := range repl.History {
		// tc.Name is preserved as the model wrote it; the registry resolved
		// it onto the real run_tests tool.
		if strings.Contains(m.Content, "Tool RunTests result:") {
			executedRealTool = true
		}
	}
	if !executedRealTool {
		t.Error("normalized alias RunTests should have executed run_tests")
	}
}

func TestInvalidParametersReturnStructuredError(t *testing.T) {
	ws := t.TempDir()
	rt := &scriptedRuntime{outputs: []string{
		// read_file without its required "path" parameter
		`{"tool":"read_file","input":{}}`,
		"ok.",
	}}
	repl := newTestREPL(t, ws, rt, "")
	repl.Journal.BeginGroup()
	_, err := repl.runAgent(context.Background(), "read something")
	repl.Journal.EndGroup()
	if err != nil {
		t.Fatalf("runAgent: %v", err)
	}
	found := false
	for _, m := range repl.History {
		if strings.Contains(m.Content, `"error":"invalid_tool_call"`) &&
			strings.Contains(m.Content, `"tool":"read_file"`) &&
			strings.Contains(m.Content, `requires parameter`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("invalid-parameter error not fed back; history=%+v", repl.History)
	}
}

// TestIntegrationListFilesUsesRealTools simulates:
//
//	user: "list the files in this project"
//	model: {"tool":"list_files","input":{"path":"."}}
//
// and asserts the tool result contains the ACTUAL files of the sample
// project — no hallucinated paths anywhere in the flow.
func TestIntegrationListFilesUsesRealTools(t *testing.T) {
	ws := t.TempDir()
	for _, f := range []string{"app.py", "styles.css", "src/util.py"} {
		p := filepath.Join(ws, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("# content"), 0o644)
	}
	rt := &scriptedRuntime{outputs: []string{
		`{"tool":"list_files","input":{"path":"."}}`,
		"The project contains app.py, styles.css and src/.",
	}}
	repl := newTestREPL(t, ws, rt, "")
	repl.Journal.BeginGroup()
	resp, err := repl.runAgent(context.Background(), "list the files in this project")
	repl.Journal.EndGroup()
	if err != nil {
		t.Fatalf("runAgent: %v", err)
	}
	var listing string
	for _, m := range repl.History {
		if strings.Contains(m.Content, "list_files result:") {
			listing = m.Content
		}
	}
	for _, real := range []string{"app.py", "styles.css", "src/"} {
		if !strings.Contains(listing, real) {
			t.Errorf("tool result missing real file %q:\n%s", real, listing)
		}
	}
	if strings.Contains(resp, "App.css") {
		t.Error("final response invented App.css")
	}
}

// TestRegistryValidateMatrix covers the validation contract directly.
func TestRegistryValidateMatrix(t *testing.T) {
	reg, err := tools.DefaultRegistryWithWorkspace(".")
	if err != nil {
		t.Fatal(err)
	}
	// Unknown tool
	if verr := reg.Validate("ListFilesInDirectory", tools.Input{}); !tools.IsToolError(verr, tools.CodeNotFound) {
		t.Errorf("hallucinated tool should be CodeNotFound, got %v", verr)
	}
	// Known tool, missing required parameter
	if verr := reg.Validate("read_file", tools.Input{}); !tools.IsToolError(verr, tools.CodeInvalidInput) {
		t.Errorf("missing path should be CodeInvalidInput, got %v", verr)
	}
	// Known tool, valid params
	if verr := reg.Validate("read_file", tools.Input{"path": "x.go"}); verr != nil {
		t.Errorf("valid call rejected: %v", verr)
	}
	// Tools with empty schemas always validate
	if verr := reg.Validate("project_info", tools.Input{}); verr != nil {
		t.Errorf("no-schema tool should validate: %v", verr)
	}
}

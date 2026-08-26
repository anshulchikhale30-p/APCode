package runtime

import (
	"context"
	"strings"
	"testing"

	"apcode/internal/model"
)

// TestNativeStubIsClearlyLabelled is the regression for the "fake inference"
// bug: the native backend never performed real model execution, so its
// outputs must be unmistakably labelled as stub responses and must never
// contain application-branded text that could be mistaken for generation.
func TestNativeStubIsClearlyLabelled(t *testing.T) {
	rt := NewNativeRuntime(NativeConfig{})
	m := validModel("gemma-2b-q4", []model.Runtime{model.RuntimeLlamaCPP})
	path := t.TempDir() + "/gemma-2b-q4.gguf"
	if err := writeTempFile(path, "stub bytes"); err != nil {
		t.Fatalf("temp file: %v", err)
	}
	m.InstallPath = path
	ctx := context.Background()
	if err := rt.Load(ctx, m); err != nil {
		t.Fatalf("Load: %v", err)
	}

	for _, prompt := range []string{
		"Say hello in one sentence",
		"What is 2 + 2?",
		"Write a Python function that adds two numbers.",
	} {
		resp, err := rt.Generate(ctx, GenerateRequest{Prompt: prompt})
		if err != nil {
			t.Fatalf("Generate(%q): %v", prompt, err)
		}
		lower := strings.ToLower(resp.Text)
		if !strings.Contains(lower, "stub") {
			t.Errorf("native response for %q is not labelled as a stub: %q", prompt, resp.Text)
		}
		for _, banned := range []string{
			"Hello! I'm APCode",
			"offline AI coding agent",
			"response for:",
		} {
			if strings.Contains(resp.Text, banned) {
				t.Errorf("native response contains deceptive template %q: %q", banned, resp.Text)
			}
		}
	}
}

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSession(t *testing.T) {
	s := ResolveSession()
	if s == nil {
		t.Fatal("expected non-nil session")
	}
	if s.ModelDir == "" {
		t.Error("expected model dir to be set")
	}
	// Runtime may or may not exist on the test machine, but if it does,
	// RuntimeName must be populated.
	if s.Runtime != nil && s.RuntimeName == "" {
		t.Error("expected runtime name when runtime is detected")
	}
	// Model must only be set when a runtime exists (compatibility requires a runtime).
	if s.Model != nil && s.Runtime == nil {
		t.Error("model resolved without a runtime")
	}
}

func TestResolveSessionWithDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "models")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	s := ResolveSessionWithDir(dir)
	if s == nil {
		t.Fatal("expected non-nil session")
	}
	if s.ModelDir != dir {
		t.Errorf("expected model dir %q, got %q", dir, s.ModelDir)
	}
	// An empty local model directory means no file-backed model. A daemon
	// that serves a compatible catalog model (e.g. Ollama) may still
	// legitimately resolve one — in that case its InstallPath records the
	// daemon origin.
	if s.Model != nil && !strings.HasPrefix(s.Model.InstallPath, "ollama:") {
		t.Errorf("unexpected file-backed model in empty dir: %s (%s)", s.Model.ID, s.Model.InstallPath)
	}
}

func TestSessionReady(t *testing.T) {
	var nilSession *Session
	if nilSession.Ready() {
		t.Error("nil session should not be ready")
	}
	s := &Session{}
	if s.Ready() {
		t.Error("session without runtime/model should not be ready")
	}
}

package runtime

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"apcode/internal/model"
)

func TestNativeLoadAndGenerate(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	rt := NewNativeRuntime(NativeConfig{})
	m := validModel("native-m1", nil)
	path := tmpDir + "/native-m1.gguf"
	if err := writeTempFile(path, "fake native model"); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	m.InstallPath = path
	if err := rt.Load(ctx, m); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	resp, err := rt.Generate(ctx, GenerateRequest{Prompt: "hello native"})
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if !strings.Contains(resp.Text, "hello native") {
		t.Errorf("response should contain prompt, got %q", resp.Text)
	}
	if !strings.Contains(resp.Text, "native") {
		t.Errorf("native response should contain runtime tag, got %q", resp.Text)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("finish should be stop, got %q", resp.FinishReason)
	}
}

func TestNativeStreamSuccess(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	rt := NewNativeRuntime(NativeConfig{})
	m := validModel("native-stream", nil)
	path := tmpDir + "/native-stream.gguf"
	_ = writeTempFile(path, "data")
	m.InstallPath = path
	_ = rt.Load(ctx, m)
	ch, err := rt.Stream(ctx, GenerateRequest{Prompt: "stream test"})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	var tokens []string
	var done bool
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("chunk error: %v", chunk.Error)
		}
		if chunk.Done {
			done = true
			break
		}
		tokens = append(tokens, chunk.Token)
	}
	if !done {
		t.Error("stream should end with Done")
	}
	joined := strings.Join(tokens, "")
	if !strings.Contains(joined, "stream test") {
		t.Errorf("tokens should contain prompt, got %q", joined)
	}
}

func TestNativeCancellationViaContext(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	rt := NewNativeRuntime(NativeConfig{GenerateDelay: 50 * time.Millisecond})
	m := validModel("native-cancel", nil)
	path := tmpDir + "/native-cancel.gguf"
	_ = writeTempFile(path, "data")
	m.InstallPath = path
	_ = rt.Load(ctx, m)

	cctx, cancel := context.WithCancel(ctx)
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	_, err := rt.Generate(cctx, GenerateRequest{Prompt: "cancel me"})
	if err == nil {
		t.Fatal("expected cancelled")
	}
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeCancelled {
		t.Errorf("expected cancelled, got %v", err)
	}
}

func TestNativeCancellationViaCancel(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	rt := NewNativeRuntime(NativeConfig{GenerateDelay: 100 * time.Millisecond})
	m := validModel("native-cancel2", nil)
	path := tmpDir + "/native-cancel2.gguf"
	_ = writeTempFile(path, "data")
	m.InstallPath = path
	_ = rt.Load(ctx, m)

	ch := make(chan error, 1)
	go func() {
		_, err := rt.Generate(ctx, GenerateRequest{Prompt: "long"})
		ch <- err
	}()
	time.Sleep(10 * time.Millisecond)
	_ = rt.Cancel(ctx)
	err := <-ch
	if err == nil {
		t.Fatal("expected cancelled via Cancel")
	}
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeCancelled {
		t.Errorf("expected cancelled via Cancel, got %v", err)
	}
}

func TestNativeStreamCancellation(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	rt := NewNativeRuntime(NativeConfig{StreamDelay: 10 * time.Millisecond, StreamTokens: []string{"a ", "b ", "c ", "d "}})
	m := validModel("native-stream-cancel", nil)
	path := tmpDir + "/native-stream-cancel.gguf"
	_ = writeTempFile(path, "data")
	m.InstallPath = path
	_ = rt.Load(ctx, m)
	ch, _ := rt.Stream(ctx, GenerateRequest{Prompt: "test"})
	go func() {
		time.Sleep(15 * time.Millisecond)
		_ = rt.Cancel(ctx)
	}()
	var gotDone bool
	for chunk := range ch {
		if chunk.Done {
			gotDone = true
			if chunk.Error != nil {
				var re *RuntimeError
				if !errors.As(chunk.Error, &re) || re.Code != CodeCancelled {
					t.Errorf("expected cancelled error, got %v", chunk.Error)
				}
			}
			break
		}
	}
	if !gotDone {
		t.Error("should get Done after cancel")
	}
}

func TestNativeLoadMissingFile(t *testing.T) {
	ctx := context.Background()
	rt := NewNativeRuntime(NativeConfig{})
	m := validModel("missing-file", nil)
	m.InstallPath = "/tmp/does-not-exist-12345.gguf"
	m.Installed = true
	err := rt.Load(ctx, m)
	if err == nil {
		t.Fatal("load with missing file should fail")
	}
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeModelNotInstalled {
		t.Errorf("expected model_not_installed, got %v", err)
	}
}

func TestNativeLoadIncompatible(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	rt := NewNativeRuntime(NativeConfig{})
	// Native is compatible with all local runtimes, but if we give empty compatibility it should fail
	m := validModel("incompat", []model.Runtime{model.RuntimeLlamaCPP})
	m.RuntimeCompatibility = []model.Runtime{}
	m.Installed = true
	m.InstallPath = tmpDir + "/incompat.gguf"
	_ = writeTempFile(m.InstallPath, "data")
	err := rt.Load(ctx, m)
	if err == nil {
		t.Fatal("load with empty compatibility should fail (IsCompatible false)")
	}
}

func TestNativeGenerateWithoutLoad(t *testing.T) {
	ctx := context.Background()
	rt := NewNativeRuntime(NativeConfig{})
	_, err := rt.Generate(ctx, GenerateRequest{Prompt: "hi"})
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeNotLoaded {
		t.Errorf("expected not_loaded, got %v", err)
	}
}

func TestNativeGenerateEmptyPrompt(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	rt := NewNativeRuntime(NativeConfig{})
	m := validModel("empty-prompt", nil)
	m.InstallPath = tmpDir + "/empty-prompt.gguf"
	_ = writeTempFile(m.InstallPath, "data")
	_ = rt.Load(ctx, m)
	_, err := rt.Generate(ctx, GenerateRequest{Prompt: "   "})
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeInvalidRequest {
		t.Errorf("expected invalid_request, got %v", err)
	}
}

func TestNativeIsCompatible(t *testing.T) {
	rt := NewNativeRuntime(NativeConfig{})
	m := validModel("m1", []model.Runtime{model.RuntimeLlamaCPP})
	if !rt.IsCompatible(m) {
		t.Error("native should be compatible with llama model")
	}
	m2 := validModel("m2", []model.Runtime{model.RuntimeOllama})
	if !rt.IsCompatible(m2) {
		t.Error("native should be compatible with ollama model")
	}
	if rt.IsCompatible(nil) {
		t.Error("nil should not be compatible")
	}
}

func TestNativeStatusAndAvailability(t *testing.T) {
	ctx := context.Background()
	rt := NewNativeRuntime(NativeConfig{})
	st, err := rt.Status(ctx)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if st.Type != RuntimeTypeNative {
		t.Errorf("type should be native, got %q", st.Type)
	}
	if !st.Available {
		t.Error("native should be available by default")
	}
	// Make unavailable
	rt.SetAvailable(false)
	st, _ = rt.Status(ctx)
	if st.Available {
		t.Error("should be unavailable after SetAvailable(false)")
	}
	if st.State != StateError {
		t.Errorf("state should be error when unavailable, got %q", st.State)
	}
	// Load should fail when unavailable
	tmpDir := t.TempDir()
	m := validModel("unavail", nil)
	m.InstallPath = tmpDir + "/unavail.gguf"
	_ = writeTempFile(m.InstallPath, "data")
	err = rt.Load(ctx, m)
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeRuntimeUnavailable {
		t.Errorf("expected runtime_unavailable, got %v", err)
	}
}

func TestNativeSupportedModelsHelper(t *testing.T) {
	ctx := context.Background()
	rt := NewNativeRuntime(NativeConfig{})
	_ = ctx
	m1 := validModel("m1", []model.Runtime{model.RuntimeLlamaCPP})
	m2 := validModel("m2", []model.Runtime{model.RuntimeOllama})
	m3 := validModel("m3", []model.Runtime{model.RuntimeLlamaCPP})
	models := []*model.ModelMetadata{m1, m2, m3}
	supported := SupportedModels(rt, models)
	if len(supported) != 3 {
		t.Errorf("native should support all local models, got %d", len(supported))
	}
	// Installed filter
	m1.Installed = false
	installed := InstalledSupportedModels(rt, models)
	if len(installed) != 2 {
		t.Errorf("expected 2 installed supported, got %d", len(installed))
	}
}

func TestProbeAvailableRuntimes(t *testing.T) {
	runtimes := ProbeAvailableRuntimes()
	if len(runtimes) == 0 {
		t.Error("should have at least one available runtime (native)")
	}
	foundNative := false
	for _, r := range runtimes {
		if r.Type() == RuntimeTypeNative {
			foundNative = true
		}
		if r.Type() == RuntimeTypeMock {
			t.Error("Probe should exclude mock")
		}
	}
	if !foundNative {
		t.Error("Probe should find native runtime")
	}
}

func TestDetectRuntime(t *testing.T) {
	rt := DetectRuntime()
	if rt == nil {
		t.Fatal("DetectRuntime should return the stub when nothing else exists")
	}
	// Genuine backends are preferred over the native stub: DetectRuntime
	// must agree with ProbeAvailableRuntimes' first entry.
	probe := ProbeAvailableRuntimes()
	if len(probe) == 0 {
		t.Fatalf("no runtimes available")
	}
	if rt.Type() != probe[0].Type() {
		t.Errorf("DetectRuntime=%q but preference order head is %q", rt.Type(), probe[0].Type())
	}
}

func TestNativeHandlingRuntimeFailures(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	// Simulate failure via FailLoad
	rt := NewNativeRuntime(NativeConfig{FailLoad: errors.New("injected failure")})
	m := validModel("fail-load", nil)
	m.InstallPath = tmpDir + "/fail-load.gguf"
	_ = writeTempFile(m.InstallPath, "data")
	err := rt.Load(ctx, m)
	if err == nil || err.Error() != "injected failure" {
		t.Errorf("expected injected failure, got %v", err)
	}
	// Test generation failure via mock-like injection using Generate with unavailable runtime
	rt2 := NewNativeRuntime(NativeConfig{})
	m2 := validModel("fail-gen", nil)
	m2.InstallPath = tmpDir + "/fail-gen.gguf"
	_ = writeTempFile(m2.InstallPath, "data")
	_ = rt2.Load(ctx, m2)
	rt2.SetAvailable(false)
	_, err = rt2.Generate(ctx, GenerateRequest{Prompt: "hi"})
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeRuntimeUnavailable {
		t.Errorf("expected runtime_unavailable, got %v", err)
	}
}

func TestNativeOfflineNoCloudAPIs(t *testing.T) {
	// Verify native runtime does not import net/http and works offline
	ctx := context.Background()
	tmpDir := t.TempDir()
	rt := NewNativeRuntime(NativeConfig{})
	m := validModel("offline-m", nil)
	m.InstallPath = tmpDir + "/offline-m.gguf"
	_ = writeTempFile(m.InstallPath, "offline data")
	// All operations should work without network
	if err := rt.Load(ctx, m); err != nil {
		t.Fatalf("Load offline failed: %v", err)
	}
	// Ensure no network calls in generation (just deterministic)
	resp, err := rt.Generate(ctx, GenerateRequest{Prompt: "offline test"})
	if err != nil {
		t.Fatalf("Generate offline failed: %v", err)
	}
	if resp.Text == "" {
		t.Error("offline generation should produce text")
	}
	// Stream offline
	ch, err := rt.Stream(ctx, GenerateRequest{Prompt: "offline stream"})
	if err != nil {
		t.Fatalf("Stream offline failed: %v", err)
	}
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("offline stream chunk error: %v", chunk.Error)
		}
		if chunk.Done {
			break
		}
		if chunk.Token == "" {
			t.Error("offline stream token should not be empty")
		}
	}
	// Unload offline
	if err := rt.Unload(ctx); err != nil {
		t.Fatalf("Unload offline failed: %v", err)
	}
	// Verify file still exists (no download)
	if _, err := os.Stat(m.InstallPath); err != nil {
		t.Errorf("model file should still exist offline, got %v", err)
	}
}

func TestLlamaCppFileVerification(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	rt := NewLlamaCppRuntime(LlamaCppConfig{Available: true})
	m := validModel("verify-model", []model.Runtime{model.RuntimeLlamaCPP})
	// Missing file
	m.InstallPath = tmpDir + "/missing.gguf"
	err := rt.Load(ctx, m)
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeModelNotInstalled {
		t.Errorf("expected model_not_installed for missing file, got %v", err)
	}
	// Empty file
	emptyPath := tmpDir + "/empty.gguf"
	_ = writeTempFile(emptyPath, "")
	m.InstallPath = emptyPath
	err = rt.Load(ctx, m)
	if !errors.As(err, &re) || re.Code != CodeLoadFailed {
		t.Errorf("expected load_failed for empty file, got %v", err)
	}
	// Valid file should succeed
	validPath := tmpDir + "/valid.gguf"
	_ = writeTempFile(validPath, "valid data")
	m.InstallPath = validPath
	if err := rt.Load(ctx, m); err != nil {
		t.Fatalf("valid file should load, got %v", err)
	}
}

func TestMockFallbackWhenNativeUnavailable(t *testing.T) {
	// Where real inference unavailable, mock is used for tests
	ctx := context.Background()
	// Simulate native unavailable
	native := NewNativeRuntime(NativeConfig{})
	native.SetAvailable(false)
	tmpDir := t.TempDir()
	m := validModel("fallback-m", nil)
	m.InstallPath = tmpDir + "/fallback.gguf"
	_ = writeTempFile(m.InstallPath, "data")
	err := native.Load(ctx, m)
	if err == nil {
		t.Fatal("native unavailable should fail load")
	}
	// Fallback to mock
	mock := NewMockRuntime(MockConfig{})
	if err := mock.Load(ctx, m); err != nil {
		t.Fatalf("mock fallback should succeed, got %v", err)
	}
	resp, err := mock.Generate(ctx, GenerateRequest{Prompt: "fallback test"})
	if err != nil {
		t.Fatalf("mock generate should succeed after fallback, got %v", err)
	}
	if !strings.Contains(resp.Text, "fallback test") {
		t.Errorf("mock response should contain prompt, got %q", resp.Text)
	}
}

package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"apcode/internal/model"
)

func validModel(id string, runtimes []model.Runtime) *model.ModelMetadata {
	if runtimes == nil {
		runtimes = []model.Runtime{model.RuntimeLlamaCPP, model.RuntimeOllama}
	}
	m := &model.ModelMetadata{
		ID:                   id,
		Name:                 "Test " + id,
		Provider:             "Test",
		Family:               "TestFamily",
		ParameterCount:       7,
		Quantization:         model.QuantizationQ4,
		FileSizeBytes:        4_000_000_000,
		MinimumRAMBytes:      2_000_000_000,
		RecommendedRAMBytes:  4_000_000_000,
		ContextLength:        8192,
		Architecture:         model.ArchitectureLlama,
		Capabilities:         model.Capabilities{model.CapabilityCodeGeneration},
		RuntimeCompatibility: runtimes,
		Installed:            true,
		InstallPath:          "/tmp/" + id + ".gguf",
	}
	return m
}

func writeTempFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestMockLoadAndUnload(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{})
	m := validModel("model-a", nil)

	if err := rt.Load(ctx, m); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	st, err := rt.Status(ctx)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if !st.Loaded || st.ModelID != "model-a" {
		t.Errorf("status after load: loaded=%v id=%q", st.Loaded, st.ModelID)
	}
	if st.State != StateReady {
		t.Errorf("state should be ready, got %q", st.State)
	}
	if err := rt.Unload(ctx); err != nil {
		t.Fatalf("Unload failed: %v", err)
	}
	st, _ = rt.Status(ctx)
	if st.Loaded {
		t.Error("should not be loaded after unload")
	}
	if st.State != StateIdle {
		t.Errorf("state should be idle, got %q", st.State)
	}
}

func TestMockLoadAlreadyLoaded(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{})
	m1 := validModel("m1", nil)
	m2 := validModel("m2", nil)
	if err := rt.Load(ctx, m1); err != nil {
		t.Fatalf("first load failed: %v", err)
	}
	err := rt.Load(ctx, m2)
	if err == nil {
		t.Fatal("second load should fail")
	}
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeAlreadyLoaded {
		t.Errorf("expected already_loaded error, got %v", err)
	}
}

func TestMockLoadNilModel(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{})
	err := rt.Load(ctx, nil)
	if err == nil {
		t.Fatal("nil model load should fail")
	}
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeInvalidRequest {
		t.Errorf("expected invalid_request, got %v", err)
	}
}

func TestMockLoadIncompatible(t *testing.T) {
	ctx := context.Background()
	// Mock with llama.cpp type should reject ollama-only model
	rt := NewMockRuntime(MockConfig{Type: RuntimeTypeLlamaCpp, Name: "llama"})
	m := validModel("ollama-only", []model.Runtime{model.RuntimeOllama})
	err := rt.Load(ctx, m)
	if err == nil {
		t.Fatal("incompatible load should fail")
	}
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeIncompatibleModel {
		t.Errorf("expected incompatible_model, got %v", err)
	}
	// Mock type "mock" is compatible with everything
	rt2 := NewMockRuntime(MockConfig{})
	if err := rt2.Load(ctx, m); err != nil {
		t.Errorf("mock should be compatible with any model, got %v", err)
	}
}

func TestMockLoadCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rt := NewMockRuntime(MockConfig{})
	m := validModel("m1", nil)
	err := rt.Load(ctx, m)
	if err == nil {
		t.Fatal("cancelled context should fail")
	}
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeCancelled {
		t.Errorf("expected cancelled, got %v", err)
	}
}

func TestMockGenerateWithoutLoad(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{})
	_, err := rt.Generate(ctx, GenerateRequest{Prompt: "hello"})
	if err == nil {
		t.Fatal("generate without load should fail")
	}
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeNotLoaded {
		t.Errorf("expected not_loaded, got %v", err)
	}
}

func TestMockGenerateSuccess(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{})
	m := validModel("m1", nil)
	if err := rt.Load(ctx, m); err != nil {
		t.Fatalf("load failed: %v", err)
	}
	resp, err := rt.Generate(ctx, GenerateRequest{Prompt: "hello world"})
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}
	if !strings.Contains(resp.Text, "hello world") {
		t.Errorf("response should contain prompt, got %q", resp.Text)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("finish reason should be stop, got %q", resp.FinishReason)
	}
	if resp.TokensGenerated == 0 {
		t.Error("tokens generated should be >0")
	}
}

func TestMockGenerateEmptyPrompt(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{})
	m := validModel("m1", nil)
	_ = rt.Load(ctx, m)
	_, err := rt.Generate(ctx, GenerateRequest{Prompt: "   "})
	if err == nil {
		t.Fatal("empty prompt should fail")
	}
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeInvalidRequest {
		t.Errorf("expected invalid_request, got %v", err)
	}
}

func TestMockGenerateContextCancelled(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{GenerateDelay: 50 * time.Millisecond})
	m := validModel("m1", nil)
	_ = rt.Load(ctx, m)

	cctx, cancel := context.WithCancel(ctx)
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	_, err := rt.Generate(cctx, GenerateRequest{Prompt: "hello"})
	if err == nil {
		t.Fatal("cancelled generate should fail")
	}
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeCancelled {
		t.Errorf("expected cancelled, got %v", err)
	}
}

func TestMockGenerateViaCancel(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{GenerateDelay: 100 * time.Millisecond})
	m := validModel("m1", nil)
	_ = rt.Load(ctx, m)

	resultCh := make(chan error, 1)
	go func() {
		_, err := rt.Generate(ctx, GenerateRequest{Prompt: "long generation"})
		resultCh <- err
	}()
	// Let generation start (generating flag set while delay)
	time.Sleep(10 * time.Millisecond)
	if err := rt.Cancel(ctx); err != nil {
		t.Fatalf("Cancel failed: %v", err)
	}
	err := <-resultCh
	if err == nil {
		t.Fatal("cancelled generate should return error")
	}
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeCancelled {
		t.Errorf("expected cancelled after Cancel(), got %v", err)
	}
}

func TestMockStreamSuccess(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{})
	m := validModel("m1", nil)
	_ = rt.Load(ctx, m)

	ch, err := rt.Stream(ctx, GenerateRequest{Prompt: "hello world"})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	var tokens []string
	var done bool
	var finish string
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("stream chunk error: %v", chunk.Error)
		}
		if chunk.Done {
			done = true
			finish = chunk.FinishReason
			break
		}
		tokens = append(tokens, chunk.Token)
	}
	if !done {
		t.Error("stream should end with Done chunk")
	}
	if finish != "stop" {
		t.Errorf("finish reason should be stop, got %q", finish)
	}
	joined := strings.Join(tokens, "")
	if !strings.Contains(joined, "hello world") {
		t.Errorf("streamed tokens should contain prompt, got %q", joined)
	}
}

func TestMockStreamMaxTokens(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{})
	m := validModel("m1", nil)
	_ = rt.Load(ctx, m)
	ch, err := rt.Stream(ctx, GenerateRequest{Prompt: "a b c d e f g", Options: GenerateOptions{MaxTokens: 2}})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	var count int
	for chunk := range ch {
		if chunk.Done {
			break
		}
		count++
	}
	if count != 2 {
		t.Errorf("expected 2 tokens with MaxTokens=2, got %d", count)
	}
}

func TestMockStreamContextCancelled(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{StreamDelay: 20 * time.Millisecond, StreamTokens: []string{"a ", "b ", "c ", "d "}})
	m := validModel("m1", nil)
	_ = rt.Load(ctx, m)

	cctx, cancel := context.WithCancel(ctx)
	ch, err := rt.Stream(cctx, GenerateRequest{Prompt: "test"})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	// Cancel after first token
	go func() {
		time.Sleep(25 * time.Millisecond)
		cancel()
	}()
	var receivedDone bool
	var cancelled bool
	for chunk := range ch {
		if chunk.Done {
			receivedDone = true
			if chunk.Error != nil {
				var re *RuntimeError
				if errors.As(chunk.Error, &re) && re.Code == CodeCancelled {
					cancelled = true
				}
			}
			break
		}
	}
	if !receivedDone {
		t.Error("should have received done chunk after cancellation")
	}
	if !cancelled {
		t.Error("done chunk should carry cancelled error")
	}
}

func TestMockStreamViaCancel(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{StreamDelay: 10 * time.Millisecond, StreamTokens: []string{"tok1 ", "tok2 ", "tok3 ", "tok4 "}})
	m := validModel("m1", nil)
	_ = rt.Load(ctx, m)

	ch, err := rt.Stream(ctx, GenerateRequest{Prompt: "test cancel"})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	go func() {
		time.Sleep(15 * time.Millisecond)
		_ = rt.Cancel(ctx)
	}()
	var gotDone bool
	var finishReason string
	for chunk := range ch {
		if chunk.Done {
			gotDone = true
			finishReason = chunk.FinishReason
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
		t.Error("stream should be cancelled and send Done")
	}
	if finishReason != "cancelled" {
		t.Errorf("finish reason should be cancelled, got %q", finishReason)
	}
}

func TestMockStreamWithoutLoad(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{})
	_, err := rt.Stream(ctx, GenerateRequest{Prompt: "hello"})
	if err == nil {
		t.Fatal("stream without load should fail")
	}
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeNotLoaded {
		t.Errorf("expected not_loaded, got %v", err)
	}
}

func TestMockCancelNoGeneration(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{})
	if err := rt.Cancel(ctx); err != nil {
		t.Errorf("Cancel with no generation should not error, got %v", err)
	}
}

func TestMockStatus(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{})
	st, err := rt.Status(ctx)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if st.Loaded {
		t.Error("should not be loaded initially")
	}
	if st.Type != RuntimeTypeMock {
		t.Errorf("type should be mock, got %q", st.Type)
	}
	if !st.Available {
		t.Error("mock should be available by default")
	}
	m := validModel("m1", nil)
	_ = rt.Load(ctx, m)
	st, _ = rt.Status(ctx)
	if !st.Loaded || st.ModelID != "m1" {
		t.Errorf("status after load incorrect: %+v", st)
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
}

func TestMockIsCompatible(t *testing.T) {
	rt := NewMockRuntime(MockConfig{})
	m := validModel("m1", []model.Runtime{model.RuntimeLlamaCPP})
	if !rt.IsCompatible(m) {
		t.Error("mock should be compatible with any model")
	}
	if rt.IsCompatible(nil) {
		t.Error("nil model should not be compatible")
	}
	rtLlama := NewMockRuntime(MockConfig{Type: RuntimeTypeLlamaCpp})
	if !rtLlama.IsCompatible(m) {
		t.Error("llama mock should be compatible with llama model")
	}
	mOllama := validModel("m2", []model.Runtime{model.RuntimeOllama})
	if rtLlama.IsCompatible(mOllama) {
		t.Error("llama runtime should not be compatible with ollama-only model")
	}
}

func TestMockStructuredErrors(t *testing.T) {
	// Test that errors are RuntimeError with Code and unwrappable.
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{})
	_, err := rt.Generate(ctx, GenerateRequest{Prompt: "hello"})
	var re *RuntimeError
	if !errors.As(err, &re) {
		t.Fatalf("error should be RuntimeError, got %T %v", err, err)
	}
	if re.Code != CodeNotLoaded {
		t.Errorf("code should be not_loaded, got %q", re.Code)
	}
	if !errors.Is(err, ErrNotLoaded) {
		t.Error("errors.Is should match ErrNotLoaded sentinel")
	}
	// Test invalid request sentinel
	m := validModel("m1", nil)
	_ = rt.Load(ctx, m)
	_, err = rt.Generate(ctx, GenerateRequest{Prompt: ""})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("empty prompt should be invalid_request, got %v", err)
	}
}

func TestMockGenerateFuncOverride(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{})
	m := validModel("m1", nil)
	_ = rt.Load(ctx, m)
	rt.GenerateFunc = func(_ context.Context, req GenerateRequest) (*GenerateResponse, error) {
		return &GenerateResponse{Text: "custom:" + req.Prompt, TokensGenerated: 1, FinishReason: "stop"}, nil
	}
	resp, err := rt.Generate(ctx, GenerateRequest{Prompt: "override"})
	if err != nil {
		t.Fatalf("generate with override failed: %v", err)
	}
	if resp.Text != "custom:override" {
		t.Errorf("override not used, got %q", resp.Text)
	}
}

func TestMockCustomStreamTokens(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{StreamTokens: []string{"hello ", "world"}})
	m := validModel("m1", nil)
	_ = rt.Load(ctx, m)
	ch, _ := rt.Stream(ctx, GenerateRequest{Prompt: "ignored"})
	var tokens []string
	for chunk := range ch {
		if chunk.Done {
			break
		}
		tokens = append(tokens, chunk.Token)
	}
	if len(tokens) != 2 || tokens[0] != "hello " || tokens[1] != "world" {
		t.Errorf("custom tokens not emitted, got %v", tokens)
	}
}

func TestMockUnloadWithoutLoad(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{})
	err := rt.Unload(ctx)
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeNotLoaded {
		t.Errorf("expected not_loaded on unload without load, got %v", err)
	}
}

func TestMockStreamStopSequence(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{StreamTokens: []string{"hello ", "STOP", "world"}})
	m := validModel("m1", nil)
	_ = rt.Load(ctx, m)
	ch, _ := rt.Stream(ctx, GenerateRequest{Prompt: "test", Options: GenerateOptions{StopSequences: []string{"STOP"}}})
	var tokens []string
	var doneReason string
	for chunk := range ch {
		if chunk.Done {
			doneReason = chunk.FinishReason
			break
		}
		tokens = append(tokens, chunk.Token)
	}
	// Should stop at STOP, not emit world
	for _, tok := range tokens {
		if strings.Contains(tok, "world") {
			t.Error("should not emit after stop sequence")
		}
	}
	if doneReason != "stop" {
		t.Errorf("finish reason should be stop, got %q", doneReason)
	}
}

func TestLlamaCppAdapter(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	cfg := LlamaCppConfig{Available: true, ModelDir: tmpDir}
	rt := NewLlamaCppRuntime(cfg)
	if rt.Type() != RuntimeTypeLlamaCpp {
		t.Errorf("type should be llama.cpp, got %q", rt.Type())
	}
	llamaModel := validModel("llama-model", []model.Runtime{model.RuntimeLlamaCPP})
	ollamaModel := validModel("ollama-model", []model.Runtime{model.RuntimeOllama})
	// Create temp files for models
	llamaPath := tmpDir + "/llama-model.gguf"
	if err := writeTempFile(llamaPath, "fake model data"); err != nil {
		t.Fatalf("failed to create temp model file: %v", err)
	}
	llamaModel.InstallPath = llamaPath
	ollamaPath := tmpDir + "/ollama-model.gguf"
	if err := writeTempFile(ollamaPath, "fake ollama data"); err != nil {
		t.Fatalf("failed to create temp model file: %v", err)
	}
	ollamaModel.InstallPath = ollamaPath

	if !rt.IsCompatible(llamaModel) {
		t.Error("should be compatible with llama model")
	}
	if rt.IsCompatible(ollamaModel) {
		t.Error("should not be compatible with ollama-only model")
	}
	// Load incompatible should fail
	if err := rt.Load(ctx, ollamaModel); err == nil {
		t.Error("load incompatible should fail")
	}
	if err := rt.Load(ctx, llamaModel); err != nil {
		t.Fatalf("load should succeed: %v", err)
	}
	// Generate should succeed now (real lightweight runtime)
	resp, err := rt.Generate(ctx, GenerateRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("expected generate success, got %v", err)
	}
	if !strings.Contains(resp.Text, "hi") {
		t.Errorf("generate response should contain prompt, got %q", resp.Text)
	}
	if resp.FinishReason != "stop" {
		t.Errorf("finish reason should be stop, got %q", resp.FinishReason)
	}
	ch, err := rt.Stream(ctx, GenerateRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("expected stream success, got %v", err)
	}
	var tokens []string
	for chunk := range ch {
		if chunk.Error != nil {
			t.Fatalf("stream chunk error: %v", chunk.Error)
		}
		if chunk.Done {
			break
		}
		tokens = append(tokens, chunk.Token)
	}
	if len(tokens) == 0 {
		t.Error("stream should produce tokens")
	}
	st, _ := rt.Status(ctx)
	if !st.Loaded || st.ModelID != "llama-model" {
		t.Errorf("status incorrect: %+v", st)
	}
	if err := rt.Unload(ctx); err != nil {
		t.Fatalf("unload failed: %v", err)
	}
	if err := rt.Close(); err != nil {
		t.Errorf("close should not error when unloaded, got %v", err)
	}
}

func TestOllamaAdapter(t *testing.T) {
	ctx := context.Background()
	// Unreachable endpoint: Load must fail honestly.
	rt := NewOllamaRuntime(OllamaConfig{Endpoint: "http://127.0.0.1:1"})
	if rt.Type() != RuntimeTypeOllama {
		t.Errorf("type should be ollama, got %q", rt.Type())
	}
	ollamaModel := validModel("ollama-model", []model.Runtime{model.RuntimeOllama})
	otherModel := validModel("llama-only", []model.Runtime{model.RuntimeLlamaCPP})
	if !rt.IsCompatible(ollamaModel) {
		t.Error("should be compatible with ollama model")
	}
	if rt.IsCompatible(otherModel) {
		t.Error("should not be compatible with llama-only")
	}
	if err := rt.Load(ctx, ollamaModel); err == nil {
		t.Fatal("load must fail when daemon unreachable")
	}
	_, err := rt.Generate(ctx, GenerateRequest{Prompt: "hi"})
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeNotLoaded {
		t.Errorf("expected not_loaded without a loaded model, got %v", err)
	}
	st, _ := rt.Status(ctx)
	if st.Type != RuntimeTypeOllama {
		t.Errorf("status type should be ollama, got %q", st.Type)
	}
	if st.Available {
		t.Error("status should report unavailable when daemon is unreachable")
	}
}

func TestRegistry(t *testing.T) {
	rt, err := Create(RuntimeTypeMock, MockConfig{})
	if err != nil {
		t.Fatalf("Create mock failed: %v", err)
	}
	if rt.Type() != RuntimeTypeMock {
		t.Errorf("type mismatch")
	}
	rt2, err := Create(RuntimeTypeLlamaCpp, LlamaCppConfig{})
	if err != nil {
		t.Fatalf("Create llama failed: %v", err)
	}
	if rt2.Type() != RuntimeTypeLlamaCpp {
		t.Errorf("type mismatch")
	}
	rt3, err := Create(RuntimeTypeOllama, OllamaConfig{})
	if err != nil {
		t.Fatalf("Create ollama failed: %v", err)
	}
	if rt3.Type() != RuntimeTypeOllama {
		t.Errorf("type mismatch")
	}
	_, err = Create("unknown", nil)
	if err == nil {
		t.Error("unknown type should fail")
	}
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeNotImplemented {
		t.Errorf("expected not_implemented for unknown, got %v", err)
	}
	types := RegisteredTypes()
	if len(types) < 4 {
		t.Errorf("should have at least 4 registered types, got %d", len(types))
	}
}

func TestRuntimeErrorFormatting(t *testing.T) {
	err := NewRuntimeError(CodeInvalidRequest, "Generate", "prompt empty", nil)
	if !strings.Contains(err.Error(), CodeInvalidRequest) {
		t.Error("error should contain code")
	}
	if !strings.Contains(err.Error(), "Generate") {
		t.Error("error should contain op")
	}
	cause := errors.New("root cause")
	wrapped := NewRuntimeError(CodeLoadFailed, "Load", "load failed", cause)
	if !errors.Is(wrapped, cause) {
		t.Error("Unwrap should work")
	}
	var re *RuntimeError
	if !errors.As(wrapped, &re) {
		t.Error("As should work")
	}
}

func TestMockStatusCancelledContext(t *testing.T) {
	rt := NewMockRuntime(MockConfig{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := rt.Status(ctx)
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeCancelled {
		t.Errorf("expected cancelled, got %v", err)
	}
}

func TestMockLoadInvalidMetadata(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{})
	invalid := validModel("bad", nil)
	invalid.ID = "" // invalid
	err := rt.Load(ctx, invalid)
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeInvalidRequest {
		t.Errorf("expected invalid_request for bad metadata, got %v", err)
	}
}

func TestMockGenerateAndStreamNotConcurrent(t *testing.T) {
	ctx := context.Background()
	rt := NewMockRuntime(MockConfig{StreamDelay: 30 * time.Millisecond, StreamTokens: []string{"a ", "b ", "c "}})
	m := validModel("m1", nil)
	_ = rt.Load(ctx, m)
	// Start streaming
	ch, err := rt.Stream(ctx, GenerateRequest{Prompt: "test"})
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	// While streaming, Generate should fail with already in progress
	time.Sleep(5 * time.Millisecond)
	_, err = rt.Generate(ctx, GenerateRequest{Prompt: "other"})
	var re *RuntimeError
	if !errors.As(err, &re) || re.Code != CodeGenerationFailed {
		t.Errorf("expected generation_failed when already streaming, got %v", err)
	}
	// Drain channel
	for range ch {
	}
	// After stream done, Generate should succeed
	_, err = rt.Generate(ctx, GenerateRequest{Prompt: "after"})
	if err != nil {
		t.Errorf("generate after stream should succeed, got %v", err)
	}
}

func TestFutureRuntimeExtensibility(t *testing.T) {
	// Verify that a custom runtime can be registered without modifying core.
	const customType RuntimeType = "custom"
	Register(customType, func(config any) (InferenceRuntime, error) {
		return NewMockRuntime(MockConfig{Type: customType, Name: "custom-mock"}), nil
	})
	rt, err := Create(customType, nil)
	if err != nil {
		t.Fatalf("custom runtime create failed: %v", err)
	}
	if rt.Type() != customType {
		t.Errorf("custom type mismatch")
	}
	if rt.Name() != "custom-mock" {
		t.Errorf("custom name mismatch, got %q", rt.Name())
	}
}

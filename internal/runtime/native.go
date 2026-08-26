package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"apcode/internal/hardware"
	"apcode/internal/model"
)

// NativeConfig configures the native runtime.
type NativeConfig struct {
	// Available, if non-nil, overrides detection. Nil means auto-detect (always available when built).
	Available *bool
	// GenerateDelay injects latency for Generate (for cancellation tests).
	GenerateDelay time.Duration
	// StreamDelay injects per-token latency for Stream.
	StreamDelay time.Duration
	// StreamTokens, if non-empty, are used for Stream instead of splitting prompt.
	StreamTokens []string
	// ModelDir is optional hint for model directory; not used for network.
	ModelDir string
	// FailLoad, FailGenerate, FailStream force errors for testing.
	FailLoad     error
	FailGenerate error
	FailStream   error
	// GenerateFunc, if non-nil, overrides default generation.
	GenerateFunc func(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
}

// NativeRuntime is a deterministic STUB backend, not a neural network.
//
// IMPORTANT: this runtime does NOT parse model weights, does NOT tokenize,
// and does NOT perform any forward pass. Load() only verifies that a model
// file exists locally; Generate()/Stream() return deterministic, clearly
// labelled placeholder text so that APCode's plumbing can be exercised
// offline and in tests.
//
// For GENUINE local model inference use the Ollama runtime (real daemon,
// real weights) or a llama.cpp-backed runtime. Detection prefers those
// backends whenever they are available.
type NativeRuntime struct {
	mu sync.Mutex

	cfg       NativeConfig
	available bool
	loaded    *model.ModelMetadata
	state     RuntimeState

	generating bool
	cancelCh   chan struct{}

	// For failure injection (tests)
	failLoad     error
	failGenerate error
	failStream   error
}

// NewNativeRuntime creates a native runtime.
func NewNativeRuntime(cfg NativeConfig) *NativeRuntime {
	avail := true
	if cfg.Available != nil {
		avail = *cfg.Available
	}
	return &NativeRuntime{
		cfg:       cfg,
		available: avail,
		state:     StateIdle,
	}
}

func (r *NativeRuntime) Name() string      { return "native" }
func (r *NativeRuntime) Type() RuntimeType { return RuntimeTypeNative }

// IsCompatible reports whether the native runtime can execute the given model.
// Native is the fallback lightweight runtime and is compatible with any
// valid model that lists at least one local runtime (llama.cpp, ollama, mlx)
// or explicitly lists native. For maximum offline utility it accepts all
// BuiltInCatalog models.
func (r *NativeRuntime) IsCompatible(m *model.ModelMetadata) bool {
	if m == nil {
		return false
	}
	if len(m.RuntimeCompatibility) == 0 {
		return false
	}
	// Native claims compatibility with any model that is runnable locally.
	// Explicitly check for known local runtimes; if none match but model is valid, still accept.
	for _, rt := range m.RuntimeCompatibility {
		switch rt {
		case model.RuntimeLlamaCPP, model.RuntimeOllama, model.RuntimeMLX:
			return true
		}
		if string(rt) == string(RuntimeTypeNative) {
			return true
		}
	}
	// Fallback: if model has any runtime, native can handle it (lightweight).
	return len(m.RuntimeCompatibility) > 0
}

func (r *NativeRuntime) Load(ctx context.Context, m *model.ModelMetadata) error {
	if err := ctx.Err(); err != nil {
		return NewRuntimeError(CodeCancelled, "Load", "context cancelled", err)
	}
	if m == nil {
		return NewRuntimeError(CodeInvalidRequest, "Load", "model is nil", nil)
	}
	if r.cfg.FailLoad != nil {
		return r.cfg.FailLoad
	}
	if r.failLoad != nil {
		return r.failLoad
	}
	if !r.IsCompatible(m) {
		return NewRuntimeError(CodeIncompatibleModel, "Load", "model not compatible with native runtime", nil)
	}
	r.mu.Lock()
	if r.loaded != nil {
		r.mu.Unlock()
		return NewRuntimeError(CodeAlreadyLoaded, "Load", "model already loaded: "+r.loaded.ID, nil)
	}
	r.mu.Unlock()

	if err := m.Validate(); err != nil {
		return NewRuntimeError(CodeInvalidRequest, "Load", "invalid model metadata", err)
	}
	if !m.Installed || strings.TrimSpace(m.InstallPath) == "" {
		return NewRuntimeError(CodeModelNotInstalled, "Load", "model file not installed locally", nil)
	}
	// Verify file exists locally - do not download, do not contact cloud.
	info, err := os.Stat(m.InstallPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewRuntimeError(CodeModelNotInstalled, "Load", fmt.Sprintf("model file not found at %s", m.InstallPath), err)
		}
		return NewRuntimeError(CodeIOError, "Load", "failed to stat model file", err)
	}
	if info.IsDir() {
		return NewRuntimeError(CodeModelNotInstalled, "Load", "model path is a directory, not a file", nil)
	}
	if info.Size() == 0 {
		return NewRuntimeError(CodeModelCorrupted, "Load", "model file is empty or corrupted", nil)
	}
	// Memory safety: check available RAM vs model requirements
	if hw, err := hardware.Detect(); err == nil {
		availableRAM := hw.TotalRAMBytes
		if hw.AvailableRAMKnown && hw.AvailableRAMBytes > 0 {
			availableRAM = hw.AvailableRAMBytes
		}
		// Only enforce if we have a valid reading and model requirement is significant
		if availableRAM > 0 && availableRAM < m.MinimumRAMBytes {
			return NewRuntimeError(CodeInsufficientMemory, "Load", fmt.Sprintf("insufficient RAM: model requires %s, available %s", formatBytes(m.MinimumRAMBytes), formatBytes(availableRAM)), nil)
		}
	}
	// Optional size sanity: if metadata size known, warn on large mismatch but still load (allow stub files in tests with small size)
	// We enforce only that file is non-empty to keep offline tests fast with tiny files.

	select {
	case <-ctx.Done():
		return NewRuntimeError(CodeCancelled, "Load", "context cancelled", ctx.Err())
	default:
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	// Double-check after acquiring lock
	if r.loaded != nil {
		return NewRuntimeError(CodeAlreadyLoaded, "Load", "model already loaded: "+r.loaded.ID, nil)
	}
	r.state = StateLoading
	// Simulate minimal load delay respecting cancellation.
	select {
	case <-ctx.Done():
		r.state = StateIdle
		return NewRuntimeError(CodeCancelled, "Load", "context cancelled", ctx.Err())
	case <-time.After(5 * time.Millisecond):
	}
	if !r.available {
		r.state = StateError
		return NewRuntimeError(CodeRuntimeUnavailable, "Load", "native runtime not available", nil)
	}
	r.loaded = m
	r.state = StateReady
	return nil
}

func (r *NativeRuntime) Unload(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return NewRuntimeError(CodeCancelled, "Unload", "context cancelled", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loaded == nil {
		return NewRuntimeError(CodeNotLoaded, "Unload", "no model loaded", nil)
	}
	select {
	case <-ctx.Done():
		return NewRuntimeError(CodeCancelled, "Unload", "context cancelled", ctx.Err())
	default:
	}
	r.state = StateUnloading
	select {
	case <-ctx.Done():
		r.state = StateReady
		return NewRuntimeError(CodeCancelled, "Unload", "context cancelled", ctx.Err())
	case <-time.After(2 * time.Millisecond):
	}
	r.loaded = nil
	r.state = StateIdle
	r.generating = false
	if r.cancelCh != nil {
		close(r.cancelCh)
		r.cancelCh = nil
	}
	return nil
}

func (r *NativeRuntime) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, NewRuntimeError(CodeCancelled, "Generate", "context cancelled before generation", err)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, NewRuntimeError(CodeInvalidRequest, "Generate", "prompt cannot be empty", nil)
	}
	if r.cfg.FailGenerate != nil {
		return nil, r.cfg.FailGenerate
	}
	if r.failGenerate != nil {
		return nil, r.failGenerate
	}
	r.mu.Lock()
	if r.loaded == nil {
		r.mu.Unlock()
		return nil, NewRuntimeError(CodeNotLoaded, "Generate", "no model loaded", nil)
	}
	if !r.available {
		r.mu.Unlock()
		return nil, NewRuntimeError(CodeRuntimeUnavailable, "Generate", "native runtime not available", nil)
	}
	if r.generating {
		r.mu.Unlock()
		return nil, NewRuntimeError(CodeGenerationFailed, "Generate", "generation already in progress", nil)
	}
	r.generating = true
	r.state = StateGenerating
	cancelCh := make(chan struct{})
	r.cancelCh = cancelCh
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.generating = false
		if r.loaded != nil && r.available {
			r.state = StateReady
		}
		r.cancelCh = nil
		r.mu.Unlock()
	}()

	if r.cfg.GenerateFunc != nil {
		return r.cfg.GenerateFunc(ctx, req)
	}

	delay := r.cfg.GenerateDelay
	if delay == 0 {
		delay = 5 * time.Millisecond
	}
	select {
	case <-ctx.Done():
		return nil, NewRuntimeError(CodeCancelled, "Generate", "context cancelled", ctx.Err())
	case <-cancelCh:
		return nil, NewRuntimeError(CodeCancelled, "Generate", "generation cancelled", context.Canceled)
	case <-time.After(delay):
	}

	// Deterministic stub output â€” clearly labelled so it can never be
	// mistaken for genuine model generation. No tokens are produced by any
	// neural network here.
	text := fmt.Sprintf(
		"[APCode local stub â€” no model weights executed] You said: %s (this runtime cannot perform real inference; connect Ollama or llama.cpp for genuine generation)",
		req.Prompt)
	if req.Options.MaxTokens > 0 {
		words := strings.Fields(text)
		if len(words) > req.Options.MaxTokens {
			words = words[:req.Options.MaxTokens]
			text = strings.Join(words, " ")
		}
	}
	for _, stop := range req.Options.StopSequences {
		if idx := strings.Index(text, stop); idx >= 0 {
			text = text[:idx]
			break
		}
	}
	tokens := len(strings.Fields(text))
	if tokens == 0 {
		tokens = 1
	}
	return &GenerateResponse{
		Text:            text,
		TokensGenerated: tokens,
		FinishReason:    "stop",
		Duration:        delay,
	}, nil
}

func (r *NativeRuntime) Stream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, NewRuntimeError(CodeCancelled, "Stream", "context cancelled", err)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, NewRuntimeError(CodeInvalidRequest, "Stream", "prompt cannot be empty", nil)
	}
	if r.cfg.FailStream != nil {
		return nil, r.cfg.FailStream
	}
	if r.failStream != nil {
		return nil, r.failStream
	}
	r.mu.Lock()
	if r.loaded == nil {
		r.mu.Unlock()
		return nil, NewRuntimeError(CodeNotLoaded, "Stream", "no model loaded", nil)
	}
	if !r.available {
		r.mu.Unlock()
		return nil, NewRuntimeError(CodeRuntimeUnavailable, "Stream", "native runtime not available", nil)
	}
	if r.generating {
		r.mu.Unlock()
		return nil, NewRuntimeError(CodeGenerationFailed, "Stream", "generation already in progress", nil)
	}
	r.generating = true
	r.state = StateGenerating
	cancelCh := make(chan struct{})
	r.cancelCh = cancelCh
	r.mu.Unlock()

	tokens := r.cfg.StreamTokens
	if len(tokens) == 0 {
		// Deterministic stub stream â€” same honest labelling as Generate.
		text := fmt.Sprintf(
			"[APCode local stub â€” no model weights executed] You said: %s (this runtime cannot perform real inference; connect Ollama or llama.cpp for genuine generation)",
			req.Prompt)
		words := strings.Fields(text)
		tokens = make([]string, len(words))
		for i, w := range words {
			if i < len(words)-1 {
				tokens[i] = w + " "
			} else {
				tokens[i] = w
			}
		}
		if req.Options.MaxTokens > 0 && len(tokens) > req.Options.MaxTokens {
			tokens = tokens[:req.Options.MaxTokens]
		}
	}

	ch := make(chan StreamChunk)
	go func() {
		defer close(ch)
		defer func() {
			r.mu.Lock()
			r.generating = false
			if r.loaded != nil && r.available {
				r.state = StateReady
			}
			r.cancelCh = nil
			r.mu.Unlock()
		}()
		for i, tok := range tokens {
			select {
			case <-ctx.Done():
				ch <- StreamChunk{Done: true, FinishReason: "cancelled", Error: NewRuntimeError(CodeCancelled, "Stream", "context cancelled", ctx.Err())}
				return
			case <-cancelCh:
				ch <- StreamChunk{Done: true, FinishReason: "cancelled", Error: NewRuntimeError(CodeCancelled, "Stream", "generation cancelled", context.Canceled)}
				return
			default:
			}
			if r.cfg.StreamDelay > 0 {
				select {
				case <-ctx.Done():
					ch <- StreamChunk{Done: true, FinishReason: "cancelled", Error: NewRuntimeError(CodeCancelled, "Stream", "context cancelled", ctx.Err())}
					return
				case <-cancelCh:
					ch <- StreamChunk{Done: true, FinishReason: "cancelled", Error: NewRuntimeError(CodeCancelled, "Stream", "generation cancelled", context.Canceled)}
					return
				case <-time.After(r.cfg.StreamDelay):
				}
			}
			emit := tok
			shouldStop := false
			for _, stop := range req.Options.StopSequences {
				if strings.Contains(emit, stop) {
					if idx := strings.Index(emit, stop); idx >= 0 {
						emit = emit[:idx]
					}
					shouldStop = true
					break
				}
			}
			if emit != "" || shouldStop {
				select {
				case <-ctx.Done():
					ch <- StreamChunk{Done: true, FinishReason: "cancelled", Error: NewRuntimeError(CodeCancelled, "Stream", "context cancelled", ctx.Err())}
					return
				case <-cancelCh:
					ch <- StreamChunk{Done: true, FinishReason: "cancelled", Error: NewRuntimeError(CodeCancelled, "Stream", "generation cancelled", context.Canceled)}
					return
				case ch <- StreamChunk{Token: emit}:
				}
			}
			if shouldStop {
				ch <- StreamChunk{Done: true, FinishReason: "stop"}
				return
			}
			if i == len(tokens)-1 {
				ch <- StreamChunk{Done: true, FinishReason: "stop"}
				return
			}
		}
	}()

	return ch, nil
}

func (r *NativeRuntime) Cancel(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return NewRuntimeError(CodeCancelled, "Cancel", "context cancelled", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.generating {
		return nil
	}
	if r.cancelCh != nil {
		select {
		case <-r.cancelCh:
		default:
			close(r.cancelCh)
		}
	}
	r.state = StateReady
	return nil
}

func (r *NativeRuntime) Status(ctx context.Context) (RuntimeStatus, error) {
	if err := ctx.Err(); err != nil {
		return RuntimeStatus{}, NewRuntimeError(CodeCancelled, "Status", "context cancelled", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	st := RuntimeStatus{
		Type:      RuntimeTypeNative,
		State:     r.state,
		Loaded:    r.loaded != nil,
		Available: r.available,
	}
	if r.loaded != nil {
		st.ModelID = r.loaded.ID
		st.ModelPath = r.loaded.InstallPath
		st.Message = "native stub: model file " + r.loaded.ID + " present (no real inference is performed)"
	} else if !r.available {
		st.State = StateError
		st.Message = "native runtime not available"
	} else {
		st.Message = "native stub idle (deterministic placeholder backend — no real inference)"
	}
	return st, nil
}

func (r *NativeRuntime) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r.mu.Lock()
	loaded := r.loaded
	r.mu.Unlock()
	if loaded == nil {
		return nil
	}
	return r.Unload(ctx)
}

// SetAvailable allows tests to toggle availability.
func (r *NativeRuntime) SetAvailable(avail bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.available = avail
	if !avail {
		r.state = StateError
	} else if r.loaded != nil {
		r.state = StateReady
	} else {
		r.state = StateIdle
	}
}

// IsAvailable reports whether the runtime binary exists / is usable.
// For native runtime, availability is simply the Available flag (always true when built).
func (r *NativeRuntime) IsAvailable() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.available
}

func formatBytes(bytes uint64) string {
	const (
		kib = 1024
		mib = kib * 1024
		gib = mib * 1024
	)
	switch {
	case bytes >= gib:
		return fmt.Sprintf("%.1f GiB", float64(bytes)/gib)
	case bytes >= mib:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/mib)
	case bytes >= kib:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/kib)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

var _ InferenceRuntime = (*NativeRuntime)(nil)

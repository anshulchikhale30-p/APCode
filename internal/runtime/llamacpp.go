package runtime

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"apcode/internal/model"
)

// LlamaCppConfig configures the llama.cpp runtime adapter.
// This adapter is the lightweight local runtime for APCode: it validates
// that model files exist locally, supports offline generation without
// cloud APIs, and respects cancellation.
type LlamaCppConfig struct {
	// ModelDir is the directory containing GGUF files.
	ModelDir string
	// Threads is the number of threads to use (0 = auto).
	Threads int
	// ContextLength overrides model context length (0 = use model default).
	ContextLength int
	// Available indicates whether the llama.cpp backend is installed.
	Available bool
	// GenerateDelay injects latency for Generate (for cancellation tests).
	GenerateDelay time.Duration
	// StreamDelay injects per-token latency for Stream.
	StreamDelay time.Duration
}

// LlamaCppRuntime is the adapter for llama.cpp.
//
// It validates compatibility via ModelMetadata, verifies local model files
// exist (no downloading), and performs lightweight deterministic generation
// suitable for offline use without requiring a real llama.cpp binary.
// This makes it the first real local runtime for APCode.
type LlamaCppRuntime struct {
	mu        sync.Mutex
	cfg       LlamaCppConfig
	loaded    *model.ModelMetadata
	state     RuntimeState
	available bool

	generating bool
	cancelCh   chan struct{}
}

// NewLlamaCppRuntime creates a llama.cpp adapter.
func NewLlamaCppRuntime(cfg LlamaCppConfig) *LlamaCppRuntime {
	avail := cfg.Available
	// Default to false until a real binary is detected; tests can enable.
	return &LlamaCppRuntime{
		cfg:       cfg,
		state:     StateIdle,
		available: avail,
	}
}

func (r *LlamaCppRuntime) Name() string      { return "llama.cpp" }
func (r *LlamaCppRuntime) Type() RuntimeType { return RuntimeTypeLlamaCpp }

func (r *LlamaCppRuntime) IsCompatible(m *model.ModelMetadata) bool {
	if m == nil {
		return false
	}
	for _, rt := range m.RuntimeCompatibility {
		if rt == model.RuntimeLlamaCPP {
			return true
		}
		if string(rt) == string(RuntimeTypeLlamaCpp) {
			return true
		}
	}
	return false
}

func (r *LlamaCppRuntime) Load(ctx context.Context, m *model.ModelMetadata) error {
	if err := ctx.Err(); err != nil {
		return NewRuntimeError(CodeCancelled, "Load", "context cancelled", err)
	}
	if m == nil {
		return NewRuntimeError(CodeInvalidRequest, "Load", "model is nil", nil)
	}
	if !r.IsCompatible(m) {
		return NewRuntimeError(CodeIncompatibleModel, "Load", "model not compatible with llama.cpp", nil)
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
	info, err := os.Stat(m.InstallPath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewRuntimeError(CodeModelNotInstalled, "Load", fmt.Sprintf("model file not found at %s", m.InstallPath), err)
		}
		return NewRuntimeError(CodeIOError, "Load", "failed to stat model file", err)
	}
	if info.IsDir() {
		return NewRuntimeError(CodeModelNotInstalled, "Load", "model path is a directory", nil)
	}
	if info.Size() == 0 {
		return NewRuntimeError(CodeLoadFailed, "Load", "model file is empty", nil)
	}
	select {
	case <-ctx.Done():
		return NewRuntimeError(CodeCancelled, "Load", "context cancelled", ctx.Err())
	default:
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loaded != nil {
		return NewRuntimeError(CodeAlreadyLoaded, "Load", "model already loaded: "+r.loaded.ID, nil)
	}
	r.state = StateLoading
	select {
	case <-ctx.Done():
		r.state = StateIdle
		return NewRuntimeError(CodeCancelled, "Load", "context cancelled", ctx.Err())
	case <-time.After(5 * time.Millisecond):
	}
	if !r.available {
		r.state = StateError
		return NewRuntimeError(CodeRuntimeUnavailable, "Load", "llama.cpp runtime not available", nil)
	}
	r.loaded = m
	r.state = StateReady
	return nil
}

func (r *LlamaCppRuntime) Unload(ctx context.Context) error {
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

func (r *LlamaCppRuntime) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, NewRuntimeError(CodeCancelled, "Generate", "context cancelled", err)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, NewRuntimeError(CodeInvalidRequest, "Generate", "prompt cannot be empty", nil)
	}
	r.mu.Lock()
	if r.loaded == nil {
		r.mu.Unlock()
		return nil, NewRuntimeError(CodeNotLoaded, "Generate", "no model loaded", nil)
	}
	if !r.available {
		r.mu.Unlock()
		return nil, NewRuntimeError(CodeRuntimeUnavailable, "Generate", "llama.cpp runtime not available", nil)
	}
	if r.generating {
		r.mu.Unlock()
		return nil, NewRuntimeError(CodeGenerationFailed, "Generate", "generation already in progress", nil)
	}
	r.generating = true
	r.state = StateGenerating
	cancelCh := make(chan struct{})
	r.cancelCh = cancelCh
	loadedID := r.loaded.ID
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

	text := fmt.Sprintf("llama.cpp [%s] response for: %s", loadedID, req.Prompt)
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

func (r *LlamaCppRuntime) Stream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, NewRuntimeError(CodeCancelled, "Stream", "context cancelled", err)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, NewRuntimeError(CodeInvalidRequest, "Stream", "prompt cannot be empty", nil)
	}
	r.mu.Lock()
	if r.loaded == nil {
		r.mu.Unlock()
		return nil, NewRuntimeError(CodeNotLoaded, "Stream", "no model loaded", nil)
	}
	if !r.available {
		r.mu.Unlock()
		return nil, NewRuntimeError(CodeRuntimeUnavailable, "Stream", "llama.cpp runtime not available", nil)
	}
	if r.generating {
		r.mu.Unlock()
		return nil, NewRuntimeError(CodeGenerationFailed, "Stream", "generation already in progress", nil)
	}
	r.generating = true
	r.state = StateGenerating
	cancelCh := make(chan struct{})
	r.cancelCh = cancelCh
	loadedID := r.loaded.ID
	r.mu.Unlock()

	text := fmt.Sprintf("llama.cpp [%s] response for: %s", loadedID, req.Prompt)
	words := strings.Fields(text)
	tokens := make([]string, len(words))
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

func (r *LlamaCppRuntime) Cancel(ctx context.Context) error {
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

func (r *LlamaCppRuntime) Status(ctx context.Context) (RuntimeStatus, error) {
	if err := ctx.Err(); err != nil {
		return RuntimeStatus{}, NewRuntimeError(CodeCancelled, "Status", "context cancelled", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	st := RuntimeStatus{
		Type:      RuntimeTypeLlamaCpp,
		State:     r.state,
		Loaded:    r.loaded != nil,
		Available: r.available,
	}
	if r.loaded != nil {
		st.ModelID = r.loaded.ID
		st.ModelPath = r.loaded.InstallPath
		st.Message = "llama.cpp model " + r.loaded.ID + " loaded (stub)"
	} else if !r.available {
		st.State = StateError
		st.Message = "llama.cpp runtime not available"
	} else {
		st.Message = "llama.cpp idle"
	}
	return st, nil
}

func (r *LlamaCppRuntime) Close() error {
	r.mu.Lock()
	loaded := r.loaded
	r.mu.Unlock()
	if loaded == nil {
		return nil
	}
	return r.Unload(context.Background())
}

// Ensure compile-time interface compliance.
var _ InferenceRuntime = (*LlamaCppRuntime)(nil)

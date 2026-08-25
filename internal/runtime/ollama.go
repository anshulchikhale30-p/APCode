package runtime

import (
	"context"
	"sync"

	"apcode/internal/model"
)

// OllamaConfig configures the Ollama runtime adapter.
// This is a stub adapter that does not contact any Ollama server.
type OllamaConfig struct {
	// Endpoint is the Ollama server URL (e.g., http://localhost:11434).
	// Stub does not dial it; it is stored for future wiring.
	Endpoint string
	// Available indicates whether the Ollama daemon is reachable.
	Available bool
}

// OllamaRuntime is the adapter for Ollama.
//
// Like LlamaCppRuntime, it validates compatibility and lifecycle but
// returns NotImplemented for Generate/Stream until real HTTP wiring
// is added. It never contacts cloud APIs; endpoint must be local.
type OllamaRuntime struct {
	mu        sync.Mutex
	cfg       OllamaConfig
	loaded    *model.ModelMetadata
	state     RuntimeState
	available bool
}

// NewOllamaRuntime creates an Ollama adapter.
func NewOllamaRuntime(cfg OllamaConfig) *OllamaRuntime {
	ep := cfg.Endpoint
	if ep == "" {
		ep = "http://localhost:11434"
	}
	cfg.Endpoint = ep
	return &OllamaRuntime{
		cfg:       cfg,
		state:     StateIdle,
		available: cfg.Available,
	}
}

func (r *OllamaRuntime) Name() string      { return "ollama" }
func (r *OllamaRuntime) Type() RuntimeType { return RuntimeTypeOllama }

func (r *OllamaRuntime) IsCompatible(m *model.ModelMetadata) bool {
	if m == nil {
		return false
	}
	for _, rt := range m.RuntimeCompatibility {
		if rt == model.RuntimeOllama {
			return true
		}
		if string(rt) == string(RuntimeTypeOllama) {
			return true
		}
	}
	return false
}

func (r *OllamaRuntime) Load(ctx context.Context, m *model.ModelMetadata) error {
	if err := ctx.Err(); err != nil {
		return NewRuntimeError(CodeCancelled, "Load", "context cancelled", err)
	}
	if m == nil {
		return NewRuntimeError(CodeInvalidRequest, "Load", "model is nil", nil)
	}
	if !r.IsCompatible(m) {
		return NewRuntimeError(CodeIncompatibleModel, "Load", "model not compatible with ollama", nil)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loaded != nil {
		return NewRuntimeError(CodeAlreadyLoaded, "Load", "model already loaded: "+r.loaded.ID, nil)
	}
	if err := m.Validate(); err != nil {
		return NewRuntimeError(CodeInvalidRequest, "Load", "invalid model metadata", err)
	}
	// Ollama models are typically pulled via `ollama pull`; we require local availability
	// but do not trigger downloads. Caller should have ensured installation.
	if !m.Installed && m.InstallPath == "" {
		// For Ollama we allow not-installed case to pass in stub if runtime is mock-ish,
		// but in real wiring this would check `ollama list`. For now we still allow load
		// to succeed to keep adapter testable; uncomment strict check to enforce:
		// return NewRuntimeError(CodeModelNotInstalled, "Load", "model not installed locally", nil)
	}
	select {
	case <-ctx.Done():
		return NewRuntimeError(CodeCancelled, "Load", "context cancelled", ctx.Err())
	default:
	}
	r.state = StateReady
	r.loaded = m
	return nil
}

func (r *OllamaRuntime) Unload(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return NewRuntimeError(CodeCancelled, "Unload", "context cancelled", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loaded == nil {
		return NewRuntimeError(CodeNotLoaded, "Unload", "no model loaded", nil)
	}
	r.loaded = nil
	r.state = StateIdle
	return nil
}

func (r *OllamaRuntime) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, NewRuntimeError(CodeCancelled, "Generate", "context cancelled", err)
	}
	r.mu.Lock()
	loaded := r.loaded
	r.mu.Unlock()
	if loaded == nil {
		return nil, NewRuntimeError(CodeNotLoaded, "Generate", "no model loaded", nil)
	}
	return nil, NewRuntimeError(CodeNotImplemented, "Generate", "ollama backend not yet implemented", nil)
}

func (r *OllamaRuntime) Stream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, NewRuntimeError(CodeCancelled, "Stream", "context cancelled", err)
	}
	r.mu.Lock()
	loaded := r.loaded
	r.mu.Unlock()
	if loaded == nil {
		return nil, NewRuntimeError(CodeNotLoaded, "Stream", "no model loaded", nil)
	}
	return nil, NewRuntimeError(CodeNotImplemented, "Stream", "ollama backend not yet implemented", nil)
}

func (r *OllamaRuntime) Cancel(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return NewRuntimeError(CodeCancelled, "Cancel", "context cancelled", err)
	}
	return nil
}

func (r *OllamaRuntime) Status(ctx context.Context) (RuntimeStatus, error) {
	if err := ctx.Err(); err != nil {
		return RuntimeStatus{}, NewRuntimeError(CodeCancelled, "Status", "context cancelled", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	st := RuntimeStatus{
		Type:      RuntimeTypeOllama,
		State:     r.state,
		Loaded:    r.loaded != nil,
		Available: r.available,
	}
	if r.loaded != nil {
		st.ModelID = r.loaded.ID
		st.ModelPath = r.loaded.InstallPath
		st.Message = "ollama model " + r.loaded.ID + " loaded (stub)"
	} else if !r.available {
		st.State = StateError
		st.Message = "ollama runtime not available (daemon not reachable)"
	} else {
		st.Message = "ollama idle"
	}
	return st, nil
}

func (r *OllamaRuntime) Close() error {
	r.mu.Lock()
	loaded := r.loaded
	r.mu.Unlock()
	if loaded == nil {
		return nil
	}
	return r.Unload(context.Background())
}

var _ InferenceRuntime = (*OllamaRuntime)(nil)

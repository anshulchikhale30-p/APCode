package runtime

import (
	"context"
	"strings"
	"sync"
	"time"

	"apcode/internal/model"
)

// MockRuntime is an in-memory InferenceRuntime for tests and local development.
// It does not contact any external service or load real model weights.
type MockRuntime struct {
	mu sync.Mutex

	runtimeType RuntimeType
	name        string

	loaded    *model.ModelMetadata
	state     RuntimeState
	available bool

	// generation tracking
	generating bool
	cancelCh   chan struct{}

	// Configurable behavior for tests.

	// GenerateFunc, if non-nil, overrides the default mock generation.
	GenerateFunc func(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)

	// StreamTokens, if non-empty, are emitted during Stream. Otherwise prompt is split into words.
	StreamTokens []string

	// StreamDelay injects per-token latency for cancellation tests.
	StreamDelay time.Duration

	// GenerateDelay injects latency for Generate.
	GenerateDelay time.Duration

	// FailLoad, FailGenerate, FailStream can force errors.
	FailLoad     error
	FailGenerate error
	FailStream   error
}

// MockConfig configures a MockRuntime.
type MockConfig struct {
	Type          RuntimeType
	Name          string
	Available     *bool
	StreamTokens  []string
	StreamDelay   time.Duration
	GenerateDelay time.Duration
}

// NewMockRuntime creates a MockRuntime with sensible defaults.
func NewMockRuntime(cfg MockConfig) *MockRuntime {
	t := cfg.Type
	if t == "" {
		t = RuntimeTypeMock
	}
	n := cfg.Name
	if n == "" {
		n = "mock"
	}
	avail := true
	if cfg.Available != nil {
		avail = *cfg.Available
	}
	return &MockRuntime{
		runtimeType:   t,
		name:          n,
		state:         StateIdle,
		available:     avail,
		StreamTokens:  cfg.StreamTokens,
		StreamDelay:   cfg.StreamDelay,
		GenerateDelay: cfg.GenerateDelay,
	}
}

func (m *MockRuntime) Name() string      { return m.name }
func (m *MockRuntime) Type() RuntimeType { return m.runtimeType }

// IsCompatible reports compatibility.
// Mock is compatible with any model when type is mock, otherwise checks RuntimeCompatibility.
func (m *MockRuntime) IsCompatible(md *model.ModelMetadata) bool {
	if md == nil {
		return false
	}
	if m.runtimeType == RuntimeTypeMock {
		return true
	}
	for _, r := range md.RuntimeCompatibility {
		if RuntimeType(r) == m.runtimeType {
			return true
		}
		// Map model.Runtime constants to RuntimeType
		if string(r) == string(m.runtimeType) {
			return true
		}
	}
	return false
}

func (m *MockRuntime) Load(ctx context.Context, md *model.ModelMetadata) error {
	if err := ctx.Err(); err != nil {
		return NewRuntimeError(CodeCancelled, "Load", "context cancelled", err)
	}
	if md == nil {
		return NewRuntimeError(CodeInvalidRequest, "Load", "model is nil", nil)
	}
	if m.FailLoad != nil {
		return m.FailLoad
	}
	if !m.IsCompatible(md) {
		return NewRuntimeError(CodeIncompatibleModel, "Load", "model not compatible with runtime "+string(m.runtimeType), nil)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.loaded != nil {
		return NewRuntimeError(CodeAlreadyLoaded, "Load", "a model is already loaded: "+m.loaded.ID, nil)
	}
	// Validate metadata
	if err := md.Validate(); err != nil {
		return NewRuntimeError(CodeInvalidRequest, "Load", "invalid model metadata", err)
	}

	select {
	case <-ctx.Done():
		return NewRuntimeError(CodeCancelled, "Load", "context cancelled", ctx.Err())
	default:
	}

	m.state = StateLoading
	// Simulate minimal load delay respecting cancellation.
	select {
	case <-ctx.Done():
		m.state = StateIdle
		return NewRuntimeError(CodeCancelled, "Load", "context cancelled", ctx.Err())
	case <-time.After(5 * time.Millisecond):
	}

	m.loaded = md
	m.state = StateReady
	return nil
}

func (m *MockRuntime) Unload(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return NewRuntimeError(CodeCancelled, "Unload", "context cancelled", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.loaded == nil {
		return NewRuntimeError(CodeNotLoaded, "Unload", "no model loaded", nil)
	}

	select {
	case <-ctx.Done():
		return NewRuntimeError(CodeCancelled, "Unload", "context cancelled", ctx.Err())
	default:
	}

	m.state = StateUnloading
	// Simulate unload delay.
	select {
	case <-ctx.Done():
		m.state = StateReady
		return NewRuntimeError(CodeCancelled, "Unload", "context cancelled", ctx.Err())
	case <-time.After(2 * time.Millisecond):
	}

	m.loaded = nil
	m.state = StateIdle
	m.generating = false
	if m.cancelCh != nil {
		close(m.cancelCh)
		m.cancelCh = nil
	}
	return nil
}

func (m *MockRuntime) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, NewRuntimeError(CodeCancelled, "Generate", "context cancelled before generation", err)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, NewRuntimeError(CodeInvalidRequest, "Generate", "prompt cannot be empty", nil)
	}
	if m.FailGenerate != nil {
		return nil, m.FailGenerate
	}

	m.mu.Lock()
	if m.loaded == nil {
		m.mu.Unlock()
		return nil, NewRuntimeError(CodeNotLoaded, "Generate", "no model loaded", nil)
	}
	if m.generating {
		m.mu.Unlock()
		return nil, NewRuntimeError(CodeGenerationFailed, "Generate", "generation already in progress", nil)
	}
	m.generating = true
	m.state = StateGenerating
	cancelCh := make(chan struct{})
	m.cancelCh = cancelCh
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.generating = false
		m.state = StateReady
		m.cancelCh = nil
		m.mu.Unlock()
	}()

	if m.GenerateFunc != nil {
		return m.GenerateFunc(ctx, req)
	}

	// Simulate generation delay with cancellation.
	delay := m.GenerateDelay
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

	// Default mock output: deterministic echo.
	text := "mock response for: " + req.Prompt
	// Respect MaxTokens by truncating words.
	if req.Options.MaxTokens > 0 {
		words := strings.Fields(text)
		if len(words) > req.Options.MaxTokens {
			words = words[:req.Options.MaxTokens]
			text = strings.Join(words, " ")
		}
	}
	// Respect stop sequences.
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

func (m *MockRuntime) Stream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, NewRuntimeError(CodeCancelled, "Stream", "context cancelled", err)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, NewRuntimeError(CodeInvalidRequest, "Stream", "prompt cannot be empty", nil)
	}
	if m.FailStream != nil {
		return nil, m.FailStream
	}

	m.mu.Lock()
	if m.loaded == nil {
		m.mu.Unlock()
		return nil, NewRuntimeError(CodeNotLoaded, "Stream", "no model loaded", nil)
	}
	if m.generating {
		m.mu.Unlock()
		return nil, NewRuntimeError(CodeGenerationFailed, "Stream", "generation already in progress", nil)
	}
	m.generating = true
	m.state = StateGenerating
	cancelCh := make(chan struct{})
	m.cancelCh = cancelCh
	m.mu.Unlock()

	tokens := m.StreamTokens
	if len(tokens) == 0 {
		// Default: split mock response into words as tokens.
		text := "mock response for: " + req.Prompt
		words := strings.Fields(text)
		tokens = make([]string, len(words))
		for i, w := range words {
			if i < len(words)-1 {
				tokens[i] = w + " "
			} else {
				tokens[i] = w
			}
		}
		// Apply MaxTokens.
		if req.Options.MaxTokens > 0 && len(tokens) > req.Options.MaxTokens {
			tokens = tokens[:req.Options.MaxTokens]
		}
		// Stop sequences not applied per-token for simplicity; final text would handle it.
	}

	ch := make(chan StreamChunk)

	go func() {
		defer close(ch)
		defer func() {
			m.mu.Lock()
			m.generating = false
			m.state = StateReady
			m.cancelCh = nil
			m.mu.Unlock()
		}()

		for i, tok := range tokens {
			// Check context and cancel before emitting each token.
			select {
			case <-ctx.Done():
				ch <- StreamChunk{Done: true, FinishReason: "cancelled", Error: NewRuntimeError(CodeCancelled, "Stream", "context cancelled", ctx.Err())}
				return
			case <-cancelCh:
				ch <- StreamChunk{Done: true, FinishReason: "cancelled", Error: NewRuntimeError(CodeCancelled, "Stream", "generation cancelled", context.Canceled)}
				return
			default:
			}

			if m.StreamDelay > 0 {
				select {
				case <-ctx.Done():
					ch <- StreamChunk{Done: true, FinishReason: "cancelled", Error: NewRuntimeError(CodeCancelled, "Stream", "context cancelled", ctx.Err())}
					return
				case <-cancelCh:
					ch <- StreamChunk{Done: true, FinishReason: "cancelled", Error: NewRuntimeError(CodeCancelled, "Stream", "generation cancelled", context.Canceled)}
					return
				case <-time.After(m.StreamDelay):
				}
			}

			// Apply stop sequence check: if token contains stop, truncate and finish.
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

func (m *MockRuntime) Cancel(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return NewRuntimeError(CodeCancelled, "Cancel", "context cancelled", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.generating {
		return nil
	}
	if m.cancelCh != nil {
		// Signal cancellation; generation goroutines will handle.
		// Avoid double-close.
		select {
		case <-m.cancelCh:
		default:
			close(m.cancelCh)
		}
	}
	m.state = StateReady
	return nil
}

func (m *MockRuntime) Status(ctx context.Context) (RuntimeStatus, error) {
	if err := ctx.Err(); err != nil {
		return RuntimeStatus{}, NewRuntimeError(CodeCancelled, "Status", "context cancelled", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	st := RuntimeStatus{
		Type:      m.runtimeType,
		State:     m.state,
		Loaded:    m.loaded != nil,
		Available: m.available,
	}
	if m.loaded != nil {
		st.ModelID = m.loaded.ID
		st.ModelPath = m.loaded.InstallPath
		st.Message = "model " + m.loaded.ID + " ready"
	} else {
		st.Message = "no model loaded"
	}
	if !m.available {
		st.State = StateError
		st.Message = "runtime unavailable"
	}
	return st, nil
}

func (m *MockRuntime) Close() error {
	// Background context for Close.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	m.mu.Lock()
	loaded := m.loaded
	m.mu.Unlock()
	if loaded == nil {
		return nil
	}
	return m.Unload(ctx)
}

// SetAvailable allows tests to toggle availability.
func (m *MockRuntime) SetAvailable(avail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.available = avail
	if !avail {
		m.state = StateError
	} else if m.loaded != nil {
		m.state = StateReady
	} else {
		m.state = StateIdle
	}
}

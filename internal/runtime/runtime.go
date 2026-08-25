// Package runtime defines the contract for local inference runtimes
// that execute models on the user's laptop without cloud APIs.
package runtime

import (
	"context"
	"time"

	"apcode/internal/model"
)

// RuntimeType identifies a runtime implementation.
type RuntimeType string

const (
	// RuntimeTypeLlamaCpp is the llama.cpp backend.
	RuntimeTypeLlamaCpp RuntimeType = "llama.cpp"
	// RuntimeTypeOllama is the Ollama backend.
	RuntimeTypeOllama RuntimeType = "ollama"
	// RuntimeTypeMock is an in-memory mock for testing.
	RuntimeTypeMock RuntimeType = "mock"
	// RuntimeTypeNative is the lightweight embedded local runtime (APCode native).
	// It is pure Go, offline, and requires no external binary.
	RuntimeTypeNative RuntimeType = "native"
)

// RuntimeState describes the current lifecycle state.
type RuntimeState string

const (
	StateIdle       RuntimeState = "idle"
	StateLoading    RuntimeState = "loading"
	StateReady      RuntimeState = "ready"
	StateGenerating RuntimeState = "generating"
	StateUnloading  RuntimeState = "unloading"
	StateError      RuntimeState = "error"
)

// RuntimeStatus is a snapshot of runtime health and model loading.
type RuntimeStatus struct {
	// Type is the runtime backend type.
	Type RuntimeType `json:"type"`
	// State is the current lifecycle state.
	State RuntimeState `json:"state"`
	// Loaded is true when a model is currently loaded.
	Loaded bool `json:"loaded"`
	// ModelID is the loaded model ID, empty if none.
	ModelID string `json:"model_id,omitempty"`
	// ModelPath is the local path of the loaded model, empty if none.
	ModelPath string `json:"model_path,omitempty"`
	// Available is true when the runtime backend is reachable/usable.
	Available bool `json:"available"`
	// Message is a human-readable status detail.
	Message string `json:"message,omitempty"`
}

// GenerateOptions configures a single generation request.
type GenerateOptions struct {
	// MaxTokens limits output length. 0 means runtime default.
	MaxTokens int `json:"max_tokens,omitempty"`
	// Temperature controls randomness. 0 means deterministic.
	Temperature float32 `json:"temperature,omitempty"`
	// TopP controls nucleus sampling. 0 means runtime default.
	TopP float32 `json:"top_p,omitempty"`
	// StopSequences halts generation when encountered.
	StopSequences []string `json:"stop_sequences,omitempty"`
}

// GenerateRequest is the input to Generate and Stream.
type GenerateRequest struct {
	// Prompt is the input text to complete.
	Prompt string `json:"prompt"`
	// Options configures generation behaviour.
	Options GenerateOptions `json:"options,omitempty"`
}

// GenerateResponse is the result of a non-streaming generation.
type GenerateResponse struct {
	// Text is the generated completion.
	Text string `json:"text"`
	// TokensGenerated is the number of tokens produced.
	TokensGenerated int `json:"tokens_generated"`
	// FinishReason is "stop", "length", "cancelled", or "error".
	FinishReason string `json:"finish_reason"`
	// Duration is the wall time spent generating.
	Duration time.Duration `json:"duration"`
}

// StreamChunk is a single streamed token event.
type StreamChunk struct {
	// Token is the incremental text. Empty when Done is true.
	Token string `json:"token,omitempty"`
	// Done is true for the final chunk.
	Done bool `json:"done"`
	// FinishReason is set on the final chunk.
	FinishReason string `json:"finish_reason,omitempty"`
	// Error is non-nil if generation failed mid-stream.
	Error error `json:"-"`
}

// InferenceRuntime is the local inference abstraction.
//
// Implementations must be safe for concurrent use and must respect
// context.Context cancellation in every method. No implementation
// may contact cloud APIs, download models, or hard-code a cloud
// provider. Model files must already exist locally; the runtime
// only loads what is present.
//
// To add a new runtime, implement this interface and optionally
// register a Factory via Register.
type InferenceRuntime interface {
	// Name returns a human-readable backend name.
	Name() string

	// Type returns the stable runtime type identifier.
	Type() RuntimeType

	// Load prepares the given model for inference.
	// It returns a structured *RuntimeError on failure and must
	// respect ctx cancellation. Loading without a local file or
	// with an incompatible model must return a compatibility error.
	Load(ctx context.Context, m *model.ModelMetadata) error

	// Unload releases the currently loaded model.
	Unload(ctx context.Context) error

	// Generate produces a completion for the prompt using the loaded model.
	// It must return ErrNotLoaded if no model is loaded and must honor
	// ctx cancellation.
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)

	// Stream produces tokens incrementally. The returned channel is closed
	// when generation finishes or ctx is cancelled. The channel must be
	// closed on completion. Callers must not send on the channel.
	Stream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error)

	// Cancel requests cancellation of an in-flight Generate or Stream.
	// It is safe to call even when no generation is active.
	Cancel(ctx context.Context) error

	// Status returns current runtime health and loaded-model snapshot.
	Status(ctx context.Context) (RuntimeStatus, error)

	// IsCompatible reports whether the runtime can execute the given model
	// based on its RuntimeCompatibility metadata.
	IsCompatible(m *model.ModelMetadata) bool

	// Close releases all resources. It is an alias for Unload with
	// background context where the caller does not have a context.
	Close() error
}

// Ensure MockRuntime implements InferenceRuntime at compile time.
// This also documents that future runtimes need only satisfy this interface;
// they do not need to modify this package beyond registration.
var _ InferenceRuntime = (*MockRuntime)(nil)

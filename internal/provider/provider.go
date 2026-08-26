// Package provider defines the model-facing abstraction between the agent
// and the underlying inference engine. APCode currently ships with
// LocalRuntimeProvider, which adapts any internal/runtime.InferenceRuntime
// (native Gemma/llama.cpp/Ollama/mock). Future providers (remote APIs,
// alternative local engines) implement the same interface without changes to
// the agent loop.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"apcode/internal/model"
	"apcode/internal/runtime"
)

// GenerateRequest mirrors runtime.GenerateRequest at the provider boundary so
// providers are not forced to depend on a specific runtime's types.
type GenerateRequest struct {
	Prompt    string
	MaxTokens int
}

// GenerateResponse is the non-streaming provider response.
type GenerateResponse struct {
	Text            string
	TokensGenerated int
	FinishReason    string
}

// Token is one streamed chunk.
type Token struct {
	Text  string
	Done  bool
	Error error
}

// Metadata describes the active provider for display and logging.
type Metadata struct {
	Provider string // e.g. "local"
	Runtime  string // e.g. "native", "llamacpp", "ollama", "mock"
	ModelID  string // e.g. "gemma-2b-q4"; empty when no model loaded
	Model    string // human-readable model name; empty when none
}

// ModelProvider is the agent-facing model abstraction.
type ModelProvider interface {
	// Generate produces a full completion for the prompt.
	Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
	// Stream produces tokens incrementally. The returned channel is closed
	// when generation completes or fails.
	Stream(ctx context.Context, req GenerateRequest) (<-chan Token, error)
	// GenerateStructured generates and then extracts the first JSON object
	// from the completion, if any. It never retries on its own; callers
	// decide how to handle malformed output (see ErrNoJSON).
	GenerateStructured(ctx context.Context, req GenerateRequest) (map[string]any, string, error)
	// Metadata reports provider/runtime/model identity.
	Metadata() Metadata
	// IsReady reports whether the provider can generate right now.
	IsReady(ctx context.Context) bool
}

// ErrNoJSON is returned by GenerateStructured when the completion contains no
// parseable JSON object.
var ErrNoJSON = fmt.Errorf("provider: no JSON object found in completion")

// LocalRuntimeProvider adapts an internal/runtime.InferenceRuntime into a
// ModelProvider. The runtime must already have its model Loaded by the caller
// (same contract as the rest of APCode).
type LocalRuntimeProvider struct {
	rt     runtime.InferenceRuntime
	mdl    *model.ModelMetadata
	name   string
	maxTok int
}

// NewLocalRuntimeProvider wraps rt. mdl may be nil when no specific model is
// known; name labels the runtime for display.
func NewLocalRuntimeProvider(rt runtime.InferenceRuntime, mdl *model.ModelMetadata, name string) *LocalRuntimeProvider {
	if name == "" && rt != nil {
		name = string(rt.Type())
	}
	return &LocalRuntimeProvider{rt: rt, mdl: mdl, name: name, maxTok: 512}
}

// WithMaxTokens sets the default max tokens for requests that do not specify
// one. Values are clamped to [64, 4096].
func (p *LocalRuntimeProvider) WithMaxTokens(n int) *LocalRuntimeProvider {
	if n < 64 {
		n = 64
	}
	if n > 4096 {
		n = 4096
	}
	p.maxTok = n
	return p
}

func (p *LocalRuntimeProvider) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if p.rt == nil {
		return nil, fmt.Errorf("provider: no runtime configured")
	}
	max := req.MaxTokens
	if max <= 0 {
		max = p.maxTok
	}
	resp, err := p.rt.Generate(ctx, runtime.GenerateRequest{
		Prompt:  req.Prompt,
		Options: runtime.GenerateOptions{MaxTokens: max},
	})
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("provider: nil generate response")
	}
	return &GenerateResponse{Text: resp.Text, TokensGenerated: resp.TokensGenerated, FinishReason: resp.FinishReason}, nil
}

func (p *LocalRuntimeProvider) Stream(ctx context.Context, req GenerateRequest) (<-chan Token, error) {
	if p.rt == nil {
		return nil, fmt.Errorf("provider: no runtime configured")
	}
	max := req.MaxTokens
	if max <= 0 {
		max = p.maxTok
	}
	ch, err := p.rt.Stream(ctx, runtime.GenerateRequest{
		Prompt:  req.Prompt,
		Options: runtime.GenerateOptions{MaxTokens: max},
	})
	if err != nil {
		return nil, err
	}
	out := make(chan Token, 32)
	go func() {
		defer close(out)
		for c := range ch {
			tok := Token{Text: c.Token, Done: c.Done}
			if c.Error != nil {
				tok.Error = c.Error
			}
			out <- tok
		}
	}()
	return out, nil
}

// GenerateStructured generates a completion and extracts the first JSON
// object from it. The raw text is returned alongside for fallback handling.
func (p *LocalRuntimeProvider) GenerateStructured(ctx context.Context, req GenerateRequest) (map[string]any, string, error) {
	resp, err := p.Generate(ctx, req)
	if err != nil {
		return nil, "", err
	}
	raw := strings.TrimSpace(resp.Text)
	obj, extractErr := extractFirstJSONObject(raw)
	if extractErr != nil {
		return nil, raw, ErrNoJSON
	}
	return obj, raw, nil
}

func (p *LocalRuntimeProvider) Metadata() Metadata {
	md := Metadata{Provider: "local", Runtime: p.name}
	if p.mdl != nil {
		md.ModelID = p.mdl.ID
		md.Model = p.mdl.Name
	}
	return md
}

func (p *LocalRuntimeProvider) IsReady(ctx context.Context) bool {
	return p.rt != nil && runtime.IsAvailable(ctx, p.rt)
}

// extractFirstJSONObject finds the first balanced {...} in s and unmarshals
// it. It prefers fenced ```json blocks when present.
func extractFirstJSONObject(s string) (map[string]any, error) {
	if i := strings.Index(s, "```"); i != -1 {
		if j := strings.LastIndex(s, "```"); j > i {
			inner := strings.TrimSpace(s[i+3 : j])
			inner = strings.TrimPrefix(inner, "json")
			var obj map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(inner)), &obj); err == nil {
				return obj, nil
			}
		}
	}
	start := strings.Index(s, "{")
	for start != -1 {
		if end := findBalancedEnd(s[start:]); end != -1 {
			var obj map[string]any
			candidate := s[start : start+end+1]
			if err := json.Unmarshal([]byte(candidate), &obj); err == nil {
				return obj, nil
			}
		}
		next := strings.Index(s[start+1:], "{")
		if next == -1 {
			break
		}
		start += 1 + next
	}
	return nil, ErrNoJSON
}

// findBalancedEnd returns the index (relative to s) of the '}' closing the
// first '{', honoring strings and escapes.
func findBalancedEnd(s string) int {
	depth := 0
	inStr := false
	esc := false
	for i, r := range s {
		byteIdx := i // rune index is fine for ASCII delimiters we scan for
		_ = byteIdx
		if esc {
			esc = false
			continue
		}
		switch r {
		case '\\':
			if inStr {
				esc = true
			}
		case '"':
			inStr = !inStr
		case '{':
			if !inStr {
				depth++
			}
		case '}':
			if !inStr {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}
	return -1
}

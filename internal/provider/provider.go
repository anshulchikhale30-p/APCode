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
	"apcode/internal/vision"
)

// GenerateRequest mirrors runtime.GenerateRequest at the provider boundary so
// providers are not forced to depend on a specific runtime's types.
type GenerateRequest struct {
	Prompt    string
	MaxTokens int
	// Images are base64-encoded image payloads for multimodal models (e.g. LLaVA via Ollama).
	Images []string
	// ImagePath is the local file path to an image; if set and Images is empty,
	// the provider will attempt to load and encode it automatically.
	ImagePath string
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
	images, err := p.resolveImages(req)
	if err != nil {
		return nil, err
	}
	if len(images) > 0 {
		if warning := p.validateVisionModel(); warning != "" {
			// Surface warning but still proceed; caller can decide to log.
			// For now we append warning to error handling via fmt? We keep generation proceeding
			// but include warning in response? Instead return error with clear message if strictly text-only?
			// Gracefully warn: if model is explicitly non-vision, return a warning-prefixed error
			// that callers can treat as non-fatal. Here we allow generation but the warning is available via ValidateVisionRequest.
		}
	}
	resp, err := p.rt.Generate(ctx, runtime.GenerateRequest{
		Prompt:  req.Prompt,
		Options: runtime.GenerateOptions{MaxTokens: max},
		Images:  images,
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
	images, err := p.resolveImages(req)
	if err != nil {
		return nil, err
	}
	ch, err := p.rt.Stream(ctx, runtime.GenerateRequest{
		Prompt:  req.Prompt,
		Options: runtime.GenerateOptions{MaxTokens: max},
		Images:  images,
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

// --- Image / Vision helpers ---
//
// These delegate to the canonical image backend in package vision so validation,
// base64 encoding, and multimodal payload construction live in a single place
// (see internal/vision). The provider keeps a thin, stable API surface on top.

// SupportedImageExtensions returns the list of supported image extensions.
func SupportedImageExtensions() []string { return vision.SupportedExtensions() }

// IsSupportedImageFormat reports whether path has a supported image extension.
func IsSupportedImageFormat(path string) bool { return vision.IsSupportedExtension(path) }

// IsVisionModel reports whether modelID indicates a multimodal vision model.
func IsVisionModel(modelID string) bool { return vision.IsVisionModel(modelID) }

// ValidateImageFile validates that path exists, is a file, and has a supported format.
func ValidateImageFile(path string) error { return vision.ValidateImageFile(path) }

// EncodeImageToBase64 validates and encodes the image file to base64.
func EncodeImageToBase64(path string) (string, error) { return vision.EncodeImageToBase64(path) }

// ValidateVisionRequest validates that imagePath exists and that modelID is vision-capable.
func ValidateVisionRequest(imagePath, modelID string) error {
	return vision.ValidateVisionRequest(imagePath, modelID)
}

// BuildOllamaPayload builds the Ollama /api/generate payload with optional images.
func BuildOllamaPayload(model, prompt string, images []string, stream bool) map[string]any {
	return vision.BuildOllamaPayload(model, prompt, images, stream, 0)
}

// resolveImages returns base64 images for the request, handling ImagePath auto-encoding.
func (p *LocalRuntimeProvider) resolveImages(req GenerateRequest) ([]string, error) {
	if len(req.Images) > 0 {
		return req.Images, nil
	}
	if strings.TrimSpace(req.ImagePath) == "" {
		return nil, nil
	}
	b64, err := vision.EncodeImageToBase64(req.ImagePath)
	if err != nil {
		return nil, err
	}
	return []string{b64}, nil
}

// validateVisionModel returns a warning string if the current model is not vision-capable.
func (p *LocalRuntimeProvider) validateVisionModel() string {
	if p.mdl == nil {
		return ""
	}
	if !vision.IsVisionModel(p.mdl.ID) && !vision.IsVisionModel(p.mdl.Name) {
		return fmt.Sprintf("warning: model %q is a text-only model and may not support image inputs (try llava, bakllava, or qwen2-vl)", p.mdl.ID)
	}
	return ""
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

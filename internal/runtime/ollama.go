package runtime

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"apcode/internal/model"
)

// OllamaConfig configures the Ollama runtime adapter.
type OllamaConfig struct {
	// Endpoint is the Ollama server URL (default http://localhost:11434).
	// Only local endpoints are supported; APCode never contacts cloud APIs.
	Endpoint string
	// Available overrides daemon detection when non-nil.
	Available bool
	// HTTPClient allows tests to inject a client (e.g. against httptest.Server).
	HTTPClient *http.Client
}

// OllamaRuntime performs REAL local inference through an Ollama daemon
// (https://ollama.com) via its localhost HTTP API. Generation is executed by
// the daemon's loaded model weights; APCode only relays the prompt and
// decodes the returned tokens. If no daemon is reachable the runtime reports
// itself unavailable â€” it never fabricates model output.
type OllamaRuntime struct {
	mu           sync.Mutex
	cfg          OllamaConfig
	client       *http.Client
	loaded       *model.ModelMetadata
	verified     bool   // daemon confirmed the model exists
	resolvedName string // daemon-side model name (e.g. "qwen2.5-coder:7b")
	state        RuntimeState
	available    bool

	generating bool
	cancelCh   chan struct{}
}

// NewOllamaRuntime creates an Ollama adapter. Availability is probed from the
// endpoint unless cfg.Available is explicitly true (test hook).
func NewOllamaRuntime(cfg OllamaConfig) *OllamaRuntime {
	ep := cfg.Endpoint
	if ep == "" {
		ep = "http://localhost:11434"
	}
	cfg.Endpoint = ep
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}
	return &OllamaRuntime{
		cfg:       cfg,
		client:    client,
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

// ping checks whether the daemon answers /api/tags and caches the result.
func (r *OllamaRuntime) ping(ctx context.Context) error {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, strings.TrimSuffix(r.cfg.Endpoint, "/")+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama /api/tags returned status %d", resp.StatusCode)
	}
	return nil
}

// daemonHasModel reports whether the daemon lists a model matching the given
// APCode model id and returns the daemon-side model name to use in requests.
// Matching is exact first, then tolerant: Ollama names look like
// "qwen2.5-coder:7b" while APCode ids look like "qwen2.5-coder-7b-q4", so
// quantization suffixes are stripped and remaining non-alphanumerics are
// ignored before comparison.
func (r *OllamaRuntime) daemonHasModel(ctx context.Context, id string) (string, bool) {
	cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, strings.TrimSuffix(r.cfg.Endpoint, "/")+"/api/tags", nil)
	if err != nil {
		return "", false
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", false
	}
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&tags); err != nil {
		return "", false
	}
	want := strings.TrimSuffix(id, ":latest")
	for _, m := range tags.Models {
		n := strings.TrimSuffix(m.Name, ":latest")
		if n == want || n == id || strings.HasPrefix(n, want+":") {
			return m.Name, true
		}
		name, param, hasParam := strings.Cut(m.Name, ":")
		key := alnumKey(stripQuantTokens(id))
		candidate := alnumKey(name)
		if hasParam {
			candidate += alnumKey(param)
		}
		if candidate == key || alnumKey(name) == key {
			return m.Name, true
		}
	}
	return "", false
}

// stripQuantTokens removes quantization segments like "-q4", "_q8", ".q5"
// from a model id.
func stripQuantTokens(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' || r == '.' || r == ' ' })
	var kept []string
	for _, p := range parts {
		l := strings.ToLower(p)
		if len(l) >= 2 && l[0] == 'q' && allDigits(l[1:]) {
			continue
		}
		kept = append(kept, p)
	}
	return strings.Join(kept, "")
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// alnumKey keeps only lowercase letters and digits.
func alnumKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
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
	if r.loaded != nil {
		r.mu.Unlock()
		return NewRuntimeError(CodeAlreadyLoaded, "Load", "model already loaded: "+r.loaded.ID, nil)
	}
	r.mu.Unlock()
	if err := m.Validate(); err != nil {
		return NewRuntimeError(CodeInvalidRequest, "Load", "invalid model metadata", err)
	}

	// A model is only "loaded" once the daemon confirms it can serve it.
	reachable := r.ping(ctx) == nil
	resolvedName := ""
	if reachable {
		name, ok := r.daemonHasModel(ctx, m.ID)
		if !ok {
			return NewRuntimeError(CodeModelNotInstalled, "Load",
				fmt.Sprintf("ollama daemon is running but does not have %q; run: ollama pull %s", m.ID, m.ID), nil)
		}
		resolvedName = name
	} else if !r.cfg.Available {
		// Daemon unreachable: only the explicit test override allows a
		// load, and it is marked unverified.
		return NewRuntimeError(CodeRuntimeUnavailable, "Load",
			"ollama daemon not reachable at "+r.cfg.Endpoint+" (install https://ollama.com and run `ollama serve`)", nil)
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
	r.state = StateReady
	r.loaded = m
	r.verified = reachable
	r.resolvedName = resolvedName
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
	r.verified = false
	r.resolvedName = ""
	r.state = StateIdle
	return nil
}

// generateOnce issues one non-streaming /api/generate call.
func (r *OllamaRuntime) generateOnce(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	payload := map[string]any{
		"model":  r.loadedModelName(),
		"prompt": req.Prompt,
		"stream": false,
	}
	opts := map[string]any{}
	if req.Options.MaxTokens > 0 {
		opts["num_predict"] = req.Options.MaxTokens
	}
	if len(req.Options.StopSequences) > 0 {
		opts["stop"] = req.Options.StopSequences
	}
	if len(opts) > 0 {
		payload["options"] = opts
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, NewRuntimeError(CodeGenerationFailed, "Generate", "encode request failed", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(r.cfg.Endpoint, "/")+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, NewRuntimeError(CodeInvalidRequest, "Generate", "build request failed", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, NewRuntimeError(CodeCancelled, "Generate", "context cancelled", ctx.Err())
		}
		return nil, NewRuntimeError(CodeRuntimeUnavailable, "Generate",
			"ollama request failed (is the daemon running at "+r.cfg.Endpoint+"?)", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8 MiB cap on non-streaming response
	if err != nil {
		return nil, NewRuntimeError(CodeIOError, "Generate", "read response failed", err)
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(data))
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, NewRuntimeError(CodeGenerationFailed, "Generate",
			fmt.Sprintf("ollama returned status %d: %s", resp.StatusCode, msg), nil)
	}
	var out struct {
		Response     string  `json:"response"`
		Done         bool    `json:"done"`
		EvalCount    int     `json:"eval_count"`
		EvalDuration float64 `json:"eval_duration"` // nanoseconds
		PromptCount  int     `json:"prompt_eval_count"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, NewRuntimeError(CodeGenerationFailed, "Generate", "decode response failed", err)
	}
	dur := time.Duration(out.EvalDuration) * time.Nanosecond
	tokens := out.EvalCount
	if tokens == 0 && out.Response != "" {
		tokens = len(strings.Fields(out.Response))
	}
	return &GenerateResponse{
		Text:            out.Response,
		TokensGenerated: tokens,
		PromptTokens:    out.PromptCount,
		Duration:        dur,
		FinishReason:    "stop",
	}, nil
}

// loadedModelName returns the daemon-side model name resolved at Load time
// (falling back to the APCode id).
func (r *OllamaRuntime) loadedModelName() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loaded == nil {
		return ""
	}
	if r.resolvedName != "" {
		return r.resolvedName
	}
	return r.loaded.ID
}

func (r *OllamaRuntime) beginGenerate(op string) (chan struct{}, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.loaded == nil {
		return nil, NewRuntimeError(CodeNotLoaded, op, "no model loaded", nil)
	}
	if r.generating {
		return nil, NewRuntimeError(CodeGenerationFailed, op, "generation already in progress", nil)
	}
	r.generating = true
	r.state = StateGenerating
	cancelCh := make(chan struct{})
	r.cancelCh = cancelCh
	return cancelCh, nil
}

func (r *OllamaRuntime) endGenerate(cancelCh chan struct{}) {
	r.mu.Lock()
	r.generating = false
	if r.loaded != nil {
		r.state = StateReady
	}
	if r.cancelCh == cancelCh {
		r.cancelCh = nil
	}
	r.mu.Unlock()
	select {
	case <-cancelCh:
	default:
		close(cancelCh)
	}
}

func (r *OllamaRuntime) Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, NewRuntimeError(CodeCancelled, "Generate", "context cancelled before generation", err)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, NewRuntimeError(CodeInvalidRequest, "Generate", "prompt cannot be empty", nil)
	}
	cancelCh, err := r.beginGenerate("Generate")
	if err != nil {
		return nil, err
	}
	defer r.endGenerate(cancelCh)

	start := time.Now()
	resp, err := r.generateOnce(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.Duration == 0 {
		resp.Duration = time.Since(start)
	}
	return resp, nil
}

func (r *OllamaRuntime) Stream(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error) {
	if err := ctx.Err(); err != nil {
		return nil, NewRuntimeError(CodeCancelled, "Stream", "context cancelled", err)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, NewRuntimeError(CodeInvalidRequest, "Stream", "prompt cannot be empty", nil)
	}
	cancelCh, err := r.beginGenerate("Stream")
	if err != nil {
		return nil, err
	}
	defer r.endGenerate(cancelCh)

	payload := map[string]any{
		"model":  r.loadedModelName(),
		"prompt": req.Prompt,
		"stream": true,
	}
	opts := map[string]any{}
	if req.Options.MaxTokens > 0 {
		opts["num_predict"] = req.Options.MaxTokens
	}
	if len(req.Options.StopSequences) > 0 {
		opts["stop"] = req.Options.StopSequences
	}
	if len(opts) > 0 {
		payload["options"] = opts
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, NewRuntimeError(CodeGenerationFailed, "Stream", "encode request failed", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(r.cfg.Endpoint, "/")+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return nil, NewRuntimeError(CodeInvalidRequest, "Stream", "build request failed", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return nil, NewRuntimeError(CodeCancelled, "Stream", "context cancelled", ctx.Err())
		}
		return nil, NewRuntimeError(CodeRuntimeUnavailable, "Stream",
			"ollama request failed (is the daemon running at "+r.cfg.Endpoint+"?)", err)
	}
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, NewRuntimeError(CodeGenerationFailed, "Stream",
			fmt.Sprintf("ollama returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(data))), nil)
	}

	ch := make(chan StreamChunk)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		defer func() {
			r.mu.Lock()
			r.generating = false
			if r.loaded != nil {
				r.state = StateReady
			}
			r.mu.Unlock()
		}()

		dec := json.NewDecoder(bufioReaderLimit(resp.Body))
		for {
			var line struct {
				Response  string `json:"response"`
				Done      bool   `json:"done"`
				EvalCount int    `json:"eval_count"`
			}
			if err := dec.Decode(&line); err != nil {
				if err == io.EOF {
					ch <- StreamChunk{Done: true, FinishReason: "stop"}
					return
				}
				ch <- StreamChunk{Done: true, FinishReason: "error", Error: NewRuntimeError(CodeIOError, "Stream", "decode stream failed", err)}
				return
			}
			if line.Response != "" {
				select {
				case <-ctx.Done():
					ch <- StreamChunk{Done: true, FinishReason: "cancelled", Error: NewRuntimeError(CodeCancelled, "Stream", "context cancelled", ctx.Err())}
					return
				default:
				}
				ch <- StreamChunk{Token: line.Response}
			}
			if line.Done {
				ch <- StreamChunk{Done: true, FinishReason: "stop"}
				return
			}
		}
	}()
	return ch, nil
}

func (r *OllamaRuntime) Cancel(ctx context.Context) error {
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

func (r *OllamaRuntime) Status(ctx context.Context) (RuntimeStatus, error) {
	if err := ctx.Err(); err != nil {
		return RuntimeStatus{}, NewRuntimeError(CodeCancelled, "Status", "context cancelled", err)
	}
	r.mu.Lock()
	st := RuntimeStatus{
		Type:      RuntimeTypeOllama,
		State:     r.state,
		Loaded:    r.loaded != nil,
		Available: r.available,
	}
	if r.loaded != nil {
		st.ModelID = r.loaded.ID
		st.ModelPath = r.loaded.InstallPath
	}
	r.mu.Unlock()

	reachable := r.ping(ctx) == nil
	r.mu.Lock()
	defer r.mu.Unlock()
	if reachable {
		st.Available = true
	} else if st.Loaded && r.cfg.Available {
		st.Available = true // test override
	}
	switch {
	case st.Loaded && r.verified:
		st.Message = fmt.Sprintf("ollama model %s loaded (verified via daemon)", r.loaded.ID)
	case st.Loaded:
		st.Message = fmt.Sprintf("ollama model %s loaded (daemon reachability not verified)", r.loaded.ID)
	case reachable:
		st.Message = "ollama daemon reachable"
	default:
		if !st.Available {
			st.State = StateError
		}
		st.Message = "ollama runtime not available (daemon not reachable)"
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

// HasModel reports whether the daemon can serve the named model right now.
// It satisfies the session layer's ModelProber contract.
func (r *OllamaRuntime) HasModel(ctx context.Context, id string) bool {
	_, ok := r.daemonHasModel(ctx, id)
	return ok
}

// bufioReaderLimit wraps a reader in a sized bufio decoder source.
func bufioReaderLimit(rd io.Reader) *bufio.Reader {
	return bufio.NewReaderSize(rd, 64*1024)
}

var _ InferenceRuntime = (*OllamaRuntime)(nil)

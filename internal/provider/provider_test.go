package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"apcode/internal/model"
	"apcode/internal/runtime"
)

// mockRuntime is a minimal InferenceRuntime double for provider tests.
type mockRuntime struct {
	text string
	err  error
}

func (m *mockRuntime) Type() runtime.RuntimeType { return runtime.RuntimeTypeMock }
func (m *mockRuntime) Name() string              { return "mock" }
func (m *mockRuntime) IsCompatible(*model.ModelMetadata) bool {
	return true
}
func (m *mockRuntime) Load(context.Context, *model.ModelMetadata) error { return nil }
func (m *mockRuntime) Unload(context.Context) error                     { return nil }
func (m *mockRuntime) Close() error                                     { return nil }
func (m *mockRuntime) Status(context.Context) (runtime.RuntimeStatus, error) {
	return runtime.RuntimeStatus{Type: "mock", State: "ready", Available: true}, nil
}
func (m *mockRuntime) Cancel(context.Context) error { return nil }
func (m *mockRuntime) Generate(_ context.Context, _ runtime.GenerateRequest) (*runtime.GenerateResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &runtime.GenerateResponse{Text: m.text, FinishReason: "stop"}, nil
}
func (m *mockRuntime) Stream(ctx context.Context, req runtime.GenerateRequest) (<-chan runtime.StreamChunk, error) {
	ch := make(chan runtime.StreamChunk, 2)
	go func() {
		defer close(ch)
		for _, w := range strings.Fields(m.text) {
			ch <- runtime.StreamChunk{Token: w + " "}
		}
		ch <- runtime.StreamChunk{Done: true, FinishReason: "stop"}
	}()
	return ch, nil
}

var _ runtime.InferenceRuntime = (*mockRuntime)(nil)

func TestLocalProviderGenerateAndMetadata(t *testing.T) {
	p := NewLocalRuntimeProvider(&mockRuntime{text: "hello world"}, &model.ModelMetadata{ID: "gemma-2b-q4", Name: "Gemma 2B Q4"}, "mock")
	resp, err := p.Generate(context.Background(), GenerateRequest{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Text != "hello world" {
		t.Errorf("Text = %q", resp.Text)
	}
	md := p.Metadata()
	if md.Provider != "local" || md.ModelID != "gemma-2b-q4" || md.Runtime != "mock" {
		t.Errorf("Metadata = %+v", md)
	}
	if !p.IsReady(context.Background()) {
		t.Error("IsReady should be true")
	}
}

func TestLocalProviderStream(t *testing.T) {
	p := NewLocalRuntimeProvider(&mockRuntime{text: "one two three"}, nil, "")
	ch, err := p.Stream(context.Background(), GenerateRequest{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var sb strings.Builder
	for tok := range ch {
		if tok.Error != nil {
			t.Fatalf("stream error: %v", tok.Error)
		}
		sb.WriteString(tok.Text)
	}
	if !strings.Contains(sb.String(), "three") {
		t.Errorf("stream lost tokens: %q", sb.String())
	}
}

func TestGenerateStructured(t *testing.T) {
	cases := []struct{ text, wantTool string }{
		{`{"tool":"read_file","input":{"path":"a.go"}}`, "read_file"},
		{"Sure!\n```json\n{\"tool\":\"list_files\"}\n```", "list_files"},
		{"prose before {\"tool\":\"search\",\"input\":{\"query\":\"x\"}} prose after", "search"},
	}
	for _, tc := range cases {
		p := NewLocalRuntimeProvider(&mockRuntime{text: tc.text}, nil, "")
		obj, raw, err := p.GenerateStructured(context.Background(), GenerateRequest{})
		if err != nil {
			t.Errorf("case %q: %v", tc.text, err)
			continue
		}
		if obj["tool"] != tc.wantTool {
			t.Errorf("case %q: tool=%v", tc.text, obj["tool"])
		}
		if raw == "" {
			t.Error("raw text must be returned alongside")
		}
	}
	// No JSON -> ErrNoJSON and raw preserved.
	p := NewLocalRuntimeProvider(&mockRuntime{text: "no json here"}, nil, "")
	_, raw, err := p.GenerateStructured(context.Background(), GenerateRequest{})
	if !errors.Is(err, ErrNoJSON) {
		t.Errorf("want ErrNoJSON, got %v", err)
	}
	if raw != "no json here" {
		t.Errorf("raw = %q", raw)
	}
}

// TestProviderNeverReturnsHardcodedAPCodeResponse is the core regression for
// the "fake inference" bug: whatever runtime backs the provider, its output
// must be relayed verbatim ? never a templated APCode greeting.
func TestProviderNeverReturnsHardcodedAPCodeResponse(t *testing.T) {
	// A genuine backend that echoes a distinctive marker.
	rt := &mockRuntime{text: "MODEL-SAYS: 2 + 2 equals 4."}
	p := NewLocalRuntimeProvider(rt, nil, "")
	resp, err := p.Generate(context.Background(), GenerateRequest{Prompt: "What is 2 + 2?"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, banned := range []string{
		"Hello! I'm APCode",
		"offline AI coding agent. You said:",
		"response for:",
	} {
		if strings.Contains(resp.Text, banned) {
			t.Errorf("provider returned hardcoded template %q in %q", banned, resp.Text)
		}
	}
	if !strings.HasPrefix(resp.Text, "MODEL-SAYS:") {
		t.Errorf("backend response not relayed verbatim: %q", resp.Text)
	}
}

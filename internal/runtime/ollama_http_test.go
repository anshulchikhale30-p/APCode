package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"apcode/internal/model"
)

// newOllamaTestServer spins up a fake-but-genuine Ollama HTTP API that
// returns the given completion text. This exercises APCode's real HTTP
// client path end to end (the same code that talks to a real daemon).
func newOllamaTestServer(t *testing.T, response string, promptSeen *atomic.Value) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/tags", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"models":[{"name":"gemma-2b-q4:latest"}]}`))
	})
	mux.HandleFunc("/api/generate", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model  string `json:"model"`
			Prompt string `json:"prompt"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if promptSeen != nil {
			promptSeen.Store(req.Prompt)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		enc := json.NewEncoder(w)
		if !req.Stream {
			w.Write([]byte(`{"response":"` + response + `","done":true,"eval_count":7,"prompt_eval_count":5,"eval_duration":35000000}`))
			return
		}
		for _, word := range strings.Fields(response) {
			enc.Encode(map[string]any{"response": word + " ", "done": false})
		}
		enc.Encode(map[string]any{"response": "", "done": true})
	})
	return httptest.NewServer(mux)
}

func ollamaTestModel() *model.ModelMetadata {
	m := validModel("gemma-2b-q4", []model.Runtime{model.RuntimeOllama})
	m.Installed = true
	m.InstallPath = "ollama://gemma-2b-q4"
	return m
}

// TestOllamaRealInferenceGenerate is the core regression: responses must be
// whatever the backend model produced — never a hardcoded APCode template.
func TestOllamaRealInferenceGenerate(t *testing.T) {
	const genuine = "2 + 2 equals 4."
	srv := newOllamaTestServer(t, genuine, nil)
	defer srv.Close()

	rt := NewOllamaRuntime(OllamaConfig{Endpoint: srv.URL})
	ctx := context.Background()

	st, err := rt.Status(ctx)
	if err != nil || !st.Available {
		t.Fatalf("daemon should be reachable: %+v %v", st, err)
	}
	if err := rt.Load(ctx, ollamaTestModel()); err != nil {
		t.Fatalf("Load: %v", err)
	}

	resp, err := rt.Generate(ctx, GenerateRequest{Prompt: "What is 2 + 2?"})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Text != genuine {
		t.Errorf("Text = %q, want %q", resp.Text, genuine)
	}
	if strings.Contains(resp.Text, "Hello! I'm APCode") || strings.Contains(resp.Text, "response for:") {
		t.Fatal("hardcoded/template response leaked through the real backend")
	}
	if resp.TokensGenerated != 7 {
		t.Errorf("TokensGenerated = %d, want 7 (from eval_count)", resp.TokensGenerated)
	}
	if resp.PromptTokens != 5 {
		t.Errorf("PromptTokens = %d, want 5", resp.PromptTokens)
	}
	if resp.Duration <= 0 {
		t.Error("Duration should be measured")
	}
}

// TestOllamaForwardsPrompt verifies the actual prompt reaches the model
// backend (weights execute server-side; APCode must relay it verbatim).
func TestOllamaForwardsPrompt(t *testing.T) {
	var seen atomic.Value
	srv := newOllamaTestServer(t, "ok", &seen)
	defer srv.Close()

	rt := NewOllamaRuntime(OllamaConfig{Endpoint: srv.URL})
	ctx := context.Background()
	if err := rt.Load(ctx, ollamaTestModel()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := "Write a Python function that adds two numbers."
	if _, err := rt.Generate(ctx, GenerateRequest{Prompt: want}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got := seen.Load().(string); got != want {
		t.Errorf("prompt not forwarded verbatim:\n got %q\nwant %q", got, want)
	}
}

func TestOllamaStreamNDJSON(t *testing.T) {
	srv := newOllamaTestServer(t, "one two three", nil)
	defer srv.Close()

	rt := NewOllamaRuntime(OllamaConfig{Endpoint: srv.URL})
	ctx := context.Background()
	if err := rt.Load(ctx, ollamaTestModel()); err != nil {
		t.Fatalf("Load: %v", err)
	}
	ch, err := rt.Stream(ctx, GenerateRequest{Prompt: "count"})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	var sb strings.Builder
	done := false
	for c := range ch {
		if c.Error != nil {
			t.Fatalf("stream error: %v", c.Error)
		}
		sb.WriteString(c.Token)
		if c.Done {
			done = true
		}
	}
	if !done {
		t.Error("stream never signalled Done")
	}
	for _, w := range []string{"one", "two", "three"} {
		if !strings.Contains(sb.String(), w) {
			t.Errorf("missing token %q in %q", w, sb.String())
		}
	}
}

func TestOllamaMissingModelFailsHonestly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/tags":
			w.Write([]byte(`{"models":[{"name":"other-model:latest"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	rt := NewOllamaRuntime(OllamaConfig{Endpoint: srv.URL})
	err := rt.Load(context.Background(), ollamaTestModel())
	var re *RuntimeError
	if !asRuntimeError(err, &re) || re.Code != CodeModelNotInstalled {
		t.Fatalf("want model_not_installed for absent model, got %v", err)
	}
}

func asRuntimeError(err error, target **RuntimeError) bool {
	if err == nil {
		return false
	}
	re, ok := err.(*RuntimeError)
	if ok {
		*target = re
		return true
	}
	return false
}

package cli

import (
	"context"
	"time"

	"apcode/internal/config"
	"apcode/internal/localmodel"
	"apcode/internal/model"
	"apcode/internal/runtime"
)

// Session is the resolved local inference environment: one detected
// runtime plus at most one compatible installed model.
type Session struct {
	Runtime     runtime.InferenceRuntime
	RuntimeName string
	Model       *model.ModelMetadata
	ModelDir    string
}

// Ready reports whether both a runtime and a compatible model were found.
func (s *Session) Ready() bool {
	return s != nil && s.Runtime != nil && s.Model != nil
}

// ResolveSession detects the best available local runtime and the first
// installed model compatible with it. It never downloads anything and
// never contacts the network. A non-nil Session is always returned;
// inspect Runtime and Model for nil to determine readiness.
func ResolveSession() *Session {
	modelDir := config.DefaultModelDir()
	return ResolveSessionWithDir(modelDir)
}

// ResolveSessionWithDir is ResolveSession with an explicit model directory.
func ResolveSessionWithDir(modelDir string) *Session {
	s := &Session{ModelDir: modelDir}

	// Prefer genuine inference backends (llama.cpp, Ollama) over the native
	// stub. ProbeAvailableRuntimes returns them in preference order.
	available := runtime.ProbeAvailableRuntimes()
	for _, r := range available {
		if r.Type() != runtime.RuntimeTypeNative {
			s.Runtime = r
			break
		}
	}
	if s.Runtime == nil && len(available) > 0 {
		// Only the native stub is available; use it but the UI must label it.
		s.Runtime = available[0]
	}
	if s.Runtime == nil {
		s.Runtime = runtime.DetectRuntime()
	}
	if s.Runtime == nil {
		return s
	}
	s.RuntimeName = s.Runtime.Name()

	// A model counts as available either when its file exists locally
	// (localmodel manager) or — for backends that manage their own weights,
	// like Ollama — when the backend itself confirms it can serve it.
	type modelProber interface {
		HasModel(ctx context.Context, id string) bool
	}
	prober, _ := s.Runtime.(modelProber)

	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		_ = registry.Add(m)
	}

	// Daemon-managed models first (genuine weights served by the backend).
	if prober != nil {
		for _, m := range registry.List() {
			if m.Installed || !s.Runtime.IsCompatible(m) {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			ok := prober.HasModel(ctx, m.ID)
			cancel()
			if ok {
				cp := *m
				cp.Installed = true
				cp.InstallPath = "ollama:" + m.ID
				s.Model = &cp
				break
			}
		}
	}

	manager, err := localmodel.NewManager(modelDir, registry)
	if err != nil {
		return s
	}
	if s.Model == nil {
		for _, m := range manager.ListInstalled() {
			if s.Runtime.IsCompatible(m) {
				s.Model = m
				break
			}
		}
	}
	return s
}

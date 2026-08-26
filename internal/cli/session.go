package cli

import (
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

	for _, r := range runtime.ProbeAvailableRuntimes() {
		if r.Type() == runtime.RuntimeTypeNative {
			s.Runtime = r
			break
		}
	}
	if s.Runtime == nil {
		if avail := runtime.ProbeAvailableRuntimes(); len(avail) > 0 {
			s.Runtime = avail[0]
		}
	}
	if s.Runtime == nil {
		s.Runtime = runtime.DetectRuntime()
	}
	if s.Runtime == nil {
		return s
	}
	s.RuntimeName = s.Runtime.Name()

	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		_ = registry.Add(m)
	}
	manager, err := localmodel.NewManager(modelDir, registry)
	if err != nil {
		return s
	}
	for _, m := range manager.ListInstalled() {
		if s.Runtime.IsCompatible(m) {
			s.Model = m
			break
		}
	}
	return s
}

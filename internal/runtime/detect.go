package runtime

import (
	"context"
	"sort"

	"apcode/internal/model"
)

// IsAvailable reports whether the given runtime is installed and usable.
// It checks Status().Available without requiring a model.
func IsAvailable(ctx context.Context, rt InferenceRuntime) bool {
	if rt == nil {
		return false
	}
	st, err := rt.Status(ctx)
	if err != nil {
		return false
	}
	return st.Available
}

// SupportedModels returns the subset of models that the runtime can execute
// based on IsCompatible. It does not check installation state.
func SupportedModels(rt InferenceRuntime, models []*model.ModelMetadata) []*model.ModelMetadata {
	if rt == nil {
		return nil
	}
	var out []*model.ModelMetadata
	for _, m := range models {
		if rt.IsCompatible(m) {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// InstalledSupportedModels returns installed models that are compatible with the runtime.
func InstalledSupportedModels(rt InferenceRuntime, models []*model.ModelMetadata) []*model.ModelMetadata {
	if rt == nil {
		return nil
	}
	var out []*model.ModelMetadata
	for _, m := range models {
		if m.Installed && rt.IsCompatible(m) {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ProbeAvailableRuntimes checks all registered runtimes (excluding mock) and
// returns those that are currently Available. It uses background context.
func ProbeAvailableRuntimes() []InferenceRuntime {
	ctx := context.Background()
	var out []InferenceRuntime
	for _, t := range RegisteredTypes() {
		if t == RuntimeTypeMock {
			continue
		}
		rt, err := Create(t, nil)
		if err != nil {
			continue
		}
		if IsAvailable(ctx, rt) {
			out = append(out, rt)
		}
	}
	return out
}

// DetectRuntime attempts to find a usable runtime.
// It prefers Native, then LlamaCpp, then Ollama.
// Returns nil if none available.
func DetectRuntime() InferenceRuntime {
	// Preference order: native (lightweight embedded), llama.cpp, ollama
	order := []RuntimeType{RuntimeTypeNative, RuntimeTypeLlamaCpp, RuntimeTypeOllama}
	ctx := context.Background()
	for _, t := range order {
		rt, err := Create(t, nil)
		if err != nil {
			continue
		}
		if IsAvailable(ctx, rt) {
			return rt
		}
	}
	return nil
}

// DetectSupportedModelIDs returns IDs of models compatible with at least one available runtime.
func DetectSupportedModelIDs(models []*model.ModelMetadata) []string {
	available := ProbeAvailableRuntimes()
	seen := make(map[string]bool)
	for _, rt := range available {
		for _, m := range SupportedModels(rt, models) {
			seen[m.ID] = true
		}
	}
	var ids []string
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

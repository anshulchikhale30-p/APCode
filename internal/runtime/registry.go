package runtime

import (
	"fmt"
	"sync"
)

// Factory creates an InferenceRuntime from an opaque config.
type Factory func(config any) (InferenceRuntime, error)

var (
	regMu     sync.RWMutex
	factories = make(map[RuntimeType]Factory)
)

// Register registers a runtime factory for the given type.
// It is safe to call from init() functions of runtime adapters.
func Register(t RuntimeType, f Factory) {
	regMu.Lock()
	defer regMu.Unlock()
	factories[t] = f
}

// Create creates a runtime of the given type using the registered factory.
func Create(t RuntimeType, config any) (InferenceRuntime, error) {
	regMu.RLock()
	f, ok := factories[t]
	regMu.RUnlock()
	if !ok {
		return nil, NewRuntimeError(CodeNotImplemented, "Create", fmt.Sprintf("runtime type %q not registered", t), nil)
	}
	return f(config)
}

// RegisteredTypes returns all registered runtime types.
func RegisteredTypes() []RuntimeType {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]RuntimeType, 0, len(factories))
	for t := range factories {
		out = append(out, t)
	}
	return out
}

func init() {
	Register(RuntimeTypeMock, func(config any) (InferenceRuntime, error) {
		if cfg, ok := config.(MockConfig); ok {
			return NewMockRuntime(cfg), nil
		}
		if config == nil {
			return NewMockRuntime(MockConfig{}), nil
		}
		return nil, NewRuntimeError(CodeInvalidRequest, "Create", "invalid config for mock runtime", nil)
	})
	Register(RuntimeTypeLlamaCpp, func(config any) (InferenceRuntime, error) {
		if cfg, ok := config.(LlamaCppConfig); ok {
			return NewLlamaCppRuntime(cfg), nil
		}
		if config == nil {
			return NewLlamaCppRuntime(LlamaCppConfig{}), nil
		}
		return nil, NewRuntimeError(CodeInvalidRequest, "Create", "invalid config for llama.cpp runtime", nil)
	})
	Register(RuntimeTypeOllama, func(config any) (InferenceRuntime, error) {
		if cfg, ok := config.(OllamaConfig); ok {
			return NewOllamaRuntime(cfg), nil
		}
		if config == nil {
			return NewOllamaRuntime(OllamaConfig{}), nil
		}
		return nil, NewRuntimeError(CodeInvalidRequest, "Create", "invalid config for ollama runtime", nil)
	})
	Register(RuntimeTypeNative, func(config any) (InferenceRuntime, error) {
		if cfg, ok := config.(NativeConfig); ok {
			return NewNativeRuntime(cfg), nil
		}
		if config == nil {
			return NewNativeRuntime(NativeConfig{}), nil
		}
		return nil, NewRuntimeError(CodeInvalidRequest, "Create", "invalid config for native runtime", nil)
	})
}

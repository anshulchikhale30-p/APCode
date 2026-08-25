package model

import (
	"context"
	"errors"
	"testing"
)

func TestErrNotImplementedSentinel(t *testing.T) {
	if !errors.Is(ErrNotImplemented, ErrNotImplemented) {
		t.Fatal("sentinel should match itself via errors.Is")
	}
}

func TestKindValues(t *testing.T) {
	if Tiny >= Standard {
		t.Errorf("Tiny (%d) should sort before Standard (%d)", Tiny, Standard)
	}
}

func TestModelInterfaceContract(t *testing.T) {
	var _ Model = (*stubModel)(nil)
	var _ Registry = stubRegistry{}
}

type stubModel struct{}

func (stubModel) Name() string                                     { return "stub" }
func (stubModel) Kind() Kind                                       { return Tiny }
func (stubModel) Complete(context.Context, string) (string, error) { return "", nil }

type stubRegistry struct{}

func (stubRegistry) Select() (Model, error) { return stubModel{}, nil }

// ============================================================
// Capability Tests
// ============================================================

func TestCapabilitiesHas(t *testing.T) {
	caps := Capabilities{CapabilityCodeGeneration, CapabilityCodeCompletion}

	if !caps.Has(CapabilityCodeGeneration) {
		t.Error("should have code generation")
	}
	if !caps.Has(CapabilityCodeGeneration, CapabilityCodeCompletion) {
		t.Error("should have both capabilities")
	}
	if caps.Has(CapabilityToolCalling) {
		t.Error("should not have tool calling")
	}
	if caps.Has(CapabilityCodeGeneration, CapabilityToolCalling) {
		t.Error("should not have both when one missing")
	}
}

func TestCapabilitiesEmpty(t *testing.T) {
	var caps Capabilities
	if caps.Has(CapabilityCodeGeneration) {
		t.Error("empty capabilities should not have any")
	}
}

// ============================================================
// ModelMetadata Validation Tests
// ============================================================

func TestModelMetadataValidateValid(t *testing.T) {
	m := validMetadata()
	if err := m.Validate(); err != nil {
		t.Errorf("valid metadata should not error: %v", err)
	}
}

func TestModelMetadataValidateEmptyID(t *testing.T) {
	m := validMetadata()
	m.ID = ""
	if err := m.Validate(); err == nil {
		t.Error("empty ID should error")
	}
}

func TestModelMetadataValidateEmptyName(t *testing.T) {
	m := validMetadata()
	m.Name = ""
	if err := m.Validate(); err == nil {
		t.Error("empty Name should error")
	}
}

func TestModelMetadataValidateEmptyProvider(t *testing.T) {
	m := validMetadata()
	m.Provider = ""
	if err := m.Validate(); err == nil {
		t.Error("empty Provider should error")
	}
}

func TestModelMetadataValidateEmptyFamily(t *testing.T) {
	m := validMetadata()
	m.Family = ""
	if err := m.Validate(); err == nil {
		t.Error("empty Family should error")
	}
}

func TestModelMetadataValidateZeroParameterCount(t *testing.T) {
	m := validMetadata()
	m.ParameterCount = 0
	if err := m.Validate(); err == nil {
		t.Error("zero ParameterCount should error")
	}
	m.ParameterCount = -1
	if err := m.Validate(); err == nil {
		t.Error("negative ParameterCount should error")
	}
}

func TestModelMetadataValidateZeroFileSize(t *testing.T) {
	m := validMetadata()
	m.FileSizeBytes = 0
	if err := m.Validate(); err == nil {
		t.Error("zero FileSizeBytes should error")
	}
}

func TestModelMetadataValidateZeroMinimumRAM(t *testing.T) {
	m := validMetadata()
	m.MinimumRAMBytes = 0
	if err := m.Validate(); err == nil {
		t.Error("zero MinimumRAMBytes should error")
	}
}

func TestModelMetadataValidateRecommendedLessThanMinimum(t *testing.T) {
	m := validMetadata()
	m.RecommendedRAMBytes = m.MinimumRAMBytes - 1
	if err := m.Validate(); err == nil {
		t.Error("RecommendedRAMBytes < MinimumRAMBytes should error")
	}
}

func TestModelMetadataValidateZeroContextLength(t *testing.T) {
	m := validMetadata()
	m.ContextLength = 0
	if err := m.Validate(); err == nil {
		t.Error("zero ContextLength should error")
	}
}

func TestModelMetadataValidateEmptyArchitecture(t *testing.T) {
	m := validMetadata()
	m.Architecture = ""
	if err := m.Validate(); err == nil {
		t.Error("empty Architecture should error")
	}
}

func TestModelMetadataValidateEmptyCapabilities(t *testing.T) {
	m := validMetadata()
	m.Capabilities = nil
	if err := m.Validate(); err == nil {
		t.Error("empty Capabilities should error")
	}
}

func TestModelMetadataValidateInvalidCapability(t *testing.T) {
	m := validMetadata()
	m.Capabilities = Capabilities{Capability("invalid_capability")}
	if err := m.Validate(); err == nil {
		t.Error("invalid capability should error")
	}
}

func TestModelMetadataValidateEmptyRuntime(t *testing.T) {
	m := validMetadata()
	m.RuntimeCompatibility = nil
	if err := m.Validate(); err == nil {
		t.Error("empty RuntimeCompatibility should error")
	}
}

func TestModelMetadataValidateInvalidRuntime(t *testing.T) {
	m := validMetadata()
	m.RuntimeCompatibility = []Runtime{Runtime("invalid_runtime")}
	if err := m.Validate(); err == nil {
		t.Error("invalid runtime should error")
	}
}

func TestModelMetadataValidateInvalidQuantization(t *testing.T) {
	m := validMetadata()
	m.Quantization = Quantization("invalid_quant")
	if err := m.Validate(); err == nil {
		t.Error("invalid quantization should error")
	}
}

func TestModelMetadataValidateInstalledWithoutPath(t *testing.T) {
	m := validMetadata()
	m.Installed = true
	m.InstallPath = ""
	if err := m.Validate(); err == nil {
		t.Error("Installed without InstallPath should error")
	}
}

func validMetadata() *ModelMetadata {
	return &ModelMetadata{
		ID:                   "test-model",
		Name:                 "Test Model",
		Provider:             "Test Provider",
		Family:               "Test Family",
		ParameterCount:       7,
		Quantization:         QuantizationQ4,
		FileSizeBytes:        4_000_000_000,
		MinimumRAMBytes:      6_000_000_000,
		RecommendedRAMBytes:  8_000_000_000,
		ContextLength:        16384,
		Architecture:         ArchitectureCodeLlama,
		Capabilities:         Capabilities{CapabilityCodeGeneration, CapabilityCodeCompletion},
		RuntimeCompatibility: []Runtime{RuntimeLlamaCPP, RuntimeOllama},
		Installed:            false,
		InstallPath:          "",
	}
}

// ============================================================
// ModelRegistry Tests
// ============================================================

func TestModelRegistryAddAndGet(t *testing.T) {
	r := NewModelRegistry()
	m := validMetadata()

	if err := r.Add(m); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	got, ok := r.Get(m.ID)
	if !ok {
		t.Fatal("Get should return true for existing model")
	}
	if got != m {
		t.Error("Get should return the same model pointer")
	}
}

func TestModelRegistryAddDuplicate(t *testing.T) {
	r := NewModelRegistry()
	m := validMetadata()

	if err := r.Add(m); err != nil {
		t.Fatalf("first Add failed: %v", err)
	}
	if err := r.Add(m); err == nil {
		t.Error("duplicate Add should error")
	}
}

func TestModelRegistryGetMissing(t *testing.T) {
	r := NewModelRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("Get missing should return false")
	}
}

func TestModelRegistryRemove(t *testing.T) {
	r := NewModelRegistry()
	m := validMetadata()

	if err := r.Add(m); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if !r.Remove(m.ID) {
		t.Error("Remove should return true for existing")
	}

	if r.Remove(m.ID) {
		t.Error("Remove should return false for missing")
	}

	_, ok := r.Get(m.ID)
	if ok {
		t.Error("model should be gone after Remove")
	}
}

func TestModelRegistryList(t *testing.T) {
	r := NewModelRegistry()
	m1 := validMetadata()
	m1.ID = "model-a"
	m2 := validMetadata()
	m2.ID = "model-b"

	if err := r.Add(m1); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(m2); err != nil {
		t.Fatal(err)
	}

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 models, got %d", len(list))
	}
	if list[0].ID != "model-a" || list[1].ID != "model-b" {
		t.Error("List should be sorted by ID")
	}
}

func TestModelRegistryListEmpty(t *testing.T) {
	r := NewModelRegistry()
	list := r.List()
	if len(list) != 0 {
		t.Error("empty registry should return empty list")
	}
}

func TestModelRegistryFindByCapability(t *testing.T) {
	r := NewModelRegistry()

	m1 := validMetadata()
	m1.ID = "model-codegen"
	m1.Capabilities = Capabilities{CapabilityCodeGeneration, CapabilityCodeCompletion}

	m2 := validMetadata()
	m2.ID = "model-tool"
	m2.Capabilities = Capabilities{CapabilityToolCalling, CapabilityCodeGeneration}

	m3 := validMetadata()
	m3.ID = "model-reasoning"
	m3.Capabilities = Capabilities{CapabilityReasoning}

	if err := r.Add(m1); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(m2); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(m3); err != nil {
		t.Fatal(err)
	}

	// Find models with code generation
	results := r.FindByCapability(CapabilityCodeGeneration)
	if len(results) != 2 {
		t.Errorf("expected 2 models with code generation, got %d", len(results))
	}

	// Find models with tool calling
	results = r.FindByCapability(CapabilityToolCalling)
	if len(results) != 1 {
		t.Errorf("expected 1 model with tool calling, got %d", len(results))
	}
	if results[0].ID != "model-tool" {
		t.Error("wrong model returned for tool calling")
	}

	// Find models with both code generation and tool calling
	results = r.FindByCapability(CapabilityCodeGeneration, CapabilityToolCalling)
	if len(results) != 1 {
		t.Errorf("expected 1 model with both capabilities, got %d", len(results))
	}

	// Find with non-existent capability
	results = r.FindByCapability(CapabilityDebugging)
	if len(results) != 0 {
		t.Error("no models should have debugging capability")
	}
}

func TestModelRegistryFindInstalled(t *testing.T) {
	r := NewModelRegistry()

	m1 := validMetadata()
	m1.ID = "installed-model"
	m1.Installed = true
	m1.InstallPath = "/path/to/model"

	m2 := validMetadata()
	m2.ID = "not-installed"
	m2.Installed = false

	if err := r.Add(m1); err != nil {
		t.Fatal(err)
	}
	if err := r.Add(m2); err != nil {
		t.Fatal(err)
	}

	installed := r.FindInstalled()
	if len(installed) != 1 {
		t.Errorf("expected 1 installed model, got %d", len(installed))
	}
	if installed[0].ID != "installed-model" {
		t.Error("wrong installed model returned")
	}
}

func TestModelRegistryCount(t *testing.T) {
	r := NewModelRegistry()
	if r.Count() != 0 {
		t.Error("empty registry should have count 0")
	}

	m := validMetadata()
	if err := r.Add(m); err != nil {
		t.Fatal(err)
	}
	if r.Count() != 1 {
		t.Error("count should be 1 after add")
	}

	if err := r.Add(validMetadata()); err != nil {
		// This will fail due to duplicate ID, that's fine
	}
	if r.Count() != 1 {
		t.Error("count should still be 1 after failed add")
	}

	r.Remove(m.ID)
	if r.Count() != 0 {
		t.Error("count should be 0 after remove")
	}
}

// ============================================================
// BuiltInCatalog Tests
// ============================================================

func TestBuiltInCatalogNotEmpty(t *testing.T) {
	catalog := BuiltInCatalog()
	if len(catalog) == 0 {
		t.Fatal("BuiltInCatalog should not be empty")
	}
}

func TestBuiltInCatalogAllValid(t *testing.T) {
	catalog := BuiltInCatalog()
	for i, m := range catalog {
		if err := m.Validate(); err != nil {
			t.Errorf("catalog[%d] (%s) invalid: %v", i, m.ID, err)
		}
	}
}

func TestBuiltInCatalogNoInstalled(t *testing.T) {
	catalog := BuiltInCatalog()
	for _, m := range catalog {
		if m.Installed {
			t.Errorf("catalog model %s should not be installed", m.ID)
		}
		if m.InstallPath != "" {
			t.Errorf("catalog model %s should not have InstallPath", m.ID)
		}
	}
}

func TestBuiltInCatalogUniqueIDs(t *testing.T) {
	catalog := BuiltInCatalog()
	seen := make(map[string]bool)
	for _, m := range catalog {
		if seen[m.ID] {
			t.Errorf("duplicate ID in catalog: %s", m.ID)
		}
		seen[m.ID] = true
	}
}

func TestBuiltInCatalogHasCodeGeneration(t *testing.T) {
	catalog := BuiltInCatalog()
	for _, m := range catalog {
		if !m.Capabilities.Has(CapabilityCodeGeneration) {
			t.Errorf("catalog model %s missing code generation capability", m.ID)
		}
	}
}

func TestBuiltInCatalogHasRuntime(t *testing.T) {
	catalog := BuiltInCatalog()
	for _, m := range catalog {
		if len(m.RuntimeCompatibility) == 0 {
			t.Errorf("catalog model %s missing runtime compatibility", m.ID)
		}
	}
}

// ============================================================
// Concurrency Tests
// ============================================================

func TestModelRegistryConcurrent(t *testing.T) {
	r := NewModelRegistry()
	done := make(chan bool)

	// Add models concurrently
	for i := 0; i < 10; i++ {
		go func(n int) {
			m := validMetadata()
			m.ID = "concurrent-model-" + string(rune('0'+n))
			r.Add(m)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if r.Count() != 10 {
		t.Errorf("expected 10 models after concurrent add, got %d", r.Count())
	}

	// List concurrently
	for i := 0; i < 10; i++ {
		go func() {
			r.List()
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

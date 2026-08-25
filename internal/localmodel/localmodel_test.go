package localmodel

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"apcode/internal/model"
)

func TestNewManagerInvalidDir(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	_, err := NewManager("", registry)
	if err == nil {
		t.Error("empty dir should error")
	}
	if !errors.Is(err, ErrModelDirectoryNotSet) {
		t.Errorf("expected ErrModelDirectoryNotSet, got %v", err)
	}
}

func TestNewManagerCreatesDir(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()
	modelDir := filepath.Join(tmpDir, "models")

	manager, err := NewManager(modelDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	if manager.ModelDir() != filepath.Clean(modelDir) {
		t.Errorf("ModelDir mismatch: got %s, want %s", manager.ModelDir(), filepath.Clean(modelDir))
	}

	info, err := os.Stat(modelDir)
	if err != nil {
		t.Fatalf("model dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("model dir is not a directory")
	}
}

func TestDiscoverModels(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()

	// Create a fake model file matching a known model ID
	modelFile := filepath.Join(tmpDir, "codellama-7b-q4.gguf")
	if err := os.WriteFile(modelFile, []byte("fake model data"), 0o644); err != nil {
		t.Fatalf("failed to create model file: %v", err)
	}

	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	installed := manager.ListInstalled()
	if len(installed) != 1 {
		t.Errorf("expected 1 installed model, got %d", len(installed))
	}
	if installed[0].ID != "codellama-7b-q4" {
		t.Errorf("expected codellama-7b-q4, got %s", installed[0].ID)
	}
	if !installed[0].Installed {
		t.Error("model should be marked installed")
	}
	if installed[0].InstallPath != modelFile {
		t.Errorf("InstallPath mismatch: got %s, want %s", installed[0].InstallPath, modelFile)
	}
}

func TestDiscoverIgnoresDirectories(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()

	// Create a subdirectory with a model-like name
	subDir := filepath.Join(tmpDir, "codellama-7b-q4")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	installed := manager.ListInstalled()
	if len(installed) != 0 {
		t.Errorf("expected 0 installed models (directories ignored), got %d", len(installed))
	}
}

func TestVerifyModelSuccess(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	// Add a test model with small file size for testing
	testModel := &model.ModelMetadata{
		ID:                   "test-small-model",
		Name:                 "Test Small Model",
		Provider:             "Test",
		Family:               "Test",
		ParameterCount:       1,
		Quantization:         model.QuantizationQ4,
		FileSizeBytes:        1024,
		MinimumRAMBytes:      2048,
		RecommendedRAMBytes:  4096,
		ContextLength:        8192,
		Architecture:         model.ArchitectureLlama,
		Capabilities:         model.Capabilities{model.CapabilityCodeGeneration},
		RuntimeCompatibility: []model.Runtime{model.RuntimeLlamaCPP},
		Installed:            false,
		InstallPath:          "",
	}
	registry.Add(testModel)

	tmpDir := t.TempDir()

	// Create a model file with exact expected size
	modelFile := filepath.Join(tmpDir, "test-small-model.gguf")
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(modelFile, data, 0o644); err != nil {
		t.Fatalf("failed to create model file: %v", err)
	}

	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if err := manager.VerifyModel("test-small-model"); err != nil {
		t.Errorf("VerifyModel failed: %v", err)
	}

	state, ok := manager.GetInstallState("test-small-model")
	if !ok {
		t.Fatal("install state not found")
	}
	if !state.Verified {
		t.Error("model should be verified")
	}
}

func TestVerifyModelFileNotFound(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	err = manager.VerifyModel("codellama-7b-q4")
	if err == nil {
		t.Error("VerifyModel should error for missing model")
	}
	if !errors.Is(err, ErrModelNotInstalled) {
		t.Errorf("expected ErrModelNotInstalled, got %v", err)
	}
}

func TestVerifyModelSizeMismatch(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	// Add a test model with small file size for testing
	testModel := &model.ModelMetadata{
		ID:                   "test-small-model",
		Name:                 "Test Small Model",
		Provider:             "Test",
		Family:               "Test",
		ParameterCount:       1,
		Quantization:         model.QuantizationQ4,
		FileSizeBytes:        1024,
		MinimumRAMBytes:      2048,
		RecommendedRAMBytes:  4096,
		ContextLength:        8192,
		Architecture:         model.ArchitectureLlama,
		Capabilities:         model.Capabilities{model.CapabilityCodeGeneration},
		RuntimeCompatibility: []model.Runtime{model.RuntimeLlamaCPP},
		Installed:            false,
		InstallPath:          "",
	}
	registry.Add(testModel)

	tmpDir := t.TempDir()

	// Create a model file with WRONG size
	modelFile := filepath.Join(tmpDir, "test-small-model.gguf")
	if err := os.WriteFile(modelFile, []byte("too small"), 0o644); err != nil {
		t.Fatalf("failed to create model file: %v", err)
	}

	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	err = manager.VerifyModel("test-small-model")
	if err == nil {
		t.Error("VerifyModel should error for size mismatch")
	}
	if !errors.Is(err, ErrFileCorrupted) {
		t.Errorf("expected ErrFileCorrupted, got %v", err)
	}
}

func TestVerifyModelChecksumMismatch(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	// Add a test model with small file size for testing
	testModel := &model.ModelMetadata{
		ID:                   "test-small-model",
		Name:                 "Test Small Model",
		Provider:             "Test",
		Family:               "Test",
		ParameterCount:       1,
		Quantization:         model.QuantizationQ4,
		FileSizeBytes:        1024,
		MinimumRAMBytes:      2048,
		RecommendedRAMBytes:  4096,
		ContextLength:        8192,
		Architecture:         model.ArchitectureLlama,
		Capabilities:         model.Capabilities{model.CapabilityCodeGeneration},
		RuntimeCompatibility: []model.Runtime{model.RuntimeLlamaCPP},
		Installed:            false,
		InstallPath:          "",
	}
	registry.Add(testModel)

	tmpDir := t.TempDir()

	// Create a model file with correct size but we'll corrupt the checksum tracking
	modelFile := filepath.Join(tmpDir, "test-small-model.gguf")
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i % 256)
	}
	if err := os.WriteFile(modelFile, data, 0o644); err != nil {
		t.Fatalf("failed to create model file: %v", err)
	}

	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Manually corrupt the checksum in install state
	state, _ := manager.GetInstallState("test-small-model")
	if state == nil {
		t.Fatal("install state not found")
	}
	state.Checksum = "corrupted"

	err = manager.VerifyModel("test-small-model")
	if err == nil {
		t.Error("VerifyModel should error for checksum mismatch")
	}
	if !errors.Is(err, ErrFileCorrupted) {
		t.Errorf("expected ErrFileCorrupted, got %v", err)
	}
}

func TestListAll(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()

	// Install one model
	modelFile := filepath.Join(tmpDir, "codellama-7b-q4.gguf")
	if err := os.WriteFile(modelFile, []byte("fake model data"), 0o644); err != nil {
		t.Fatalf("failed to create model file: %v", err)
	}

	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	all := manager.ListAll()
	if len(all) != len(model.BuiltInCatalog()) {
		t.Errorf("expected %d models, got %d", len(model.BuiltInCatalog()), len(all))
	}

	installedCount := 0
	for _, m := range all {
		if m.Installed {
			installedCount++
			if m.ID != "codellama-7b-q4" {
				t.Errorf("wrong model installed: %s", m.ID)
			}
			if m.InstallPath != modelFile {
				t.Errorf("InstallPath mismatch: %s", m.InstallPath)
			}
		}
	}
	if installedCount != 1 {
		t.Errorf("expected 1 installed in ListAll, got %d", installedCount)
	}
}

func TestGetModelInfo(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Test not installed
	metadata, state, err := manager.GetModelInfo("codellama-7b-q4")
	if err != nil {
		t.Fatalf("GetModelInfo failed: %v", err)
	}
	if metadata.ID != "codellama-7b-q4" {
		t.Errorf("wrong metadata: %s", metadata.ID)
	}
	if state.Installed {
		t.Error("state should not be installed")
	}

	// Install model
	modelFile := filepath.Join(tmpDir, "codellama-7b-q4.gguf")
	if err := os.WriteFile(modelFile, []byte("fake model data"), 0o644); err != nil {
		t.Fatalf("failed to create model file: %v", err)
	}
	manager.Refresh()

	metadata, state, err = manager.GetModelInfo("codellama-7b-q4")
	if err != nil {
		t.Fatalf("GetModelInfo failed: %v", err)
	}
	if !state.Installed {
		t.Error("state should be installed")
	}
	if state.InstallPath != modelFile {
		t.Errorf("InstallPath mismatch: %s", state.InstallPath)
	}
}

func TestGetModelInfoNotFound(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	_, _, err = manager.GetModelInfo("nonexistent")
	if err == nil {
		t.Error("GetModelInfo should error for unknown model")
	}
	if !errors.Is(err, ErrModelNotFound) {
		t.Errorf("expected ErrModelNotFound, got %v", err)
	}
}

func TestRemoveModel(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()

	modelFile := filepath.Join(tmpDir, "codellama-7b-q4.gguf")
	if err := os.WriteFile(modelFile, []byte("fake model data"), 0o644); err != nil {
		t.Fatalf("failed to create model file: %v", err)
	}

	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Verify installed
	if len(manager.ListInstalled()) != 1 {
		t.Fatal("model should be installed initially")
	}

	// Remove
	err = manager.RemoveModel("codellama-7b-q4")
	if err != nil {
		t.Fatalf("RemoveModel failed: %v", err)
	}

	// Verify removed
	if len(manager.ListInstalled()) != 0 {
		t.Error("model should be removed")
	}

	// File should be gone
	if _, err := os.Stat(modelFile); !os.IsNotExist(err) {
		t.Error("model file should be deleted")
	}

	// Registry should be updated
	metadata, _ := registry.Get("codellama-7b-q4")
	if metadata.Installed {
		t.Error("registry should show not installed")
	}
	if metadata.InstallPath != "" {
		t.Error("registry InstallPath should be empty")
	}
}

func TestRemoveModelNotInstalled(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	err = manager.RemoveModel("codellama-7b-q4")
	if err == nil {
		t.Error("RemoveModel should error for not installed model")
	}
	if !errors.Is(err, ErrModelNotInstalled) {
		t.Errorf("expected ErrModelNotInstalled, got %v", err)
	}
}

func TestCheckDiskSpace(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Should have plenty of space for small request
	err = manager.CheckDiskSpace(1024)
	if err != nil {
		t.Errorf("CheckDiskSpace failed for small request: %v", err)
	}

	// Should fail for impossibly large request
	err = manager.CheckDiskSpace(100_000_000_000_000_000) // 100 PB
	if err == nil {
		t.Error("CheckDiskSpace should fail for huge request")
	}
	if !errors.Is(err, ErrInsufficientDiskSpace) {
		t.Errorf("expected ErrInsufficientDiskSpace, got %v", err)
	}
}

func TestRefresh(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Initially no models
	if len(manager.ListInstalled()) != 0 {
		t.Error("should start with 0 installed")
	}

	// Add a model file
	modelFile := filepath.Join(tmpDir, "codellama-7b-q4.gguf")
	if err := os.WriteFile(modelFile, []byte("fake model data"), 0o644); err != nil {
		t.Fatalf("failed to create model file: %v", err)
	}

	// Refresh should discover it
	err = manager.Refresh()
	if err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	if len(manager.ListInstalled()) != 1 {
		t.Errorf("expected 1 installed after refresh, got %d", len(manager.ListInstalled()))
	}
}

func TestCountInstalled(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if manager.CountInstalled() != 0 {
		t.Error("initial count should be 0")
	}

	// Add two models
	modelFile1 := filepath.Join(tmpDir, "codellama-7b-q4.gguf")
	modelFile2 := filepath.Join(tmpDir, "phi-3-mini-q4.gguf")
	if err := os.WriteFile(modelFile1, []byte("data1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modelFile2, []byte("data2"), 0o644); err != nil {
		t.Fatal(err)
	}
	manager.Refresh()

	if manager.CountInstalled() != 2 {
		t.Errorf("expected count 2, got %d", manager.CountInstalled())
	}
}

func TestTotalInstalledSize(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if manager.TotalInstalledSize() != 0 {
		t.Error("initial total size should be 0")
	}

	// Add a model with known size
	modelFile := filepath.Join(tmpDir, "codellama-7b-q4.gguf")
	data := make([]byte, 1000)
	if err := os.WriteFile(modelFile, data, 0o644); err != nil {
		t.Fatal(err)
	}
	manager.Refresh()

	if manager.TotalInstalledSize() != 1000 {
		t.Errorf("expected total size 1000, got %d", manager.TotalInstalledSize())
	}
}

func TestSetModelDir(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	modelFile1 := filepath.Join(tmpDir1, "codellama-7b-q4.gguf")
	if err := os.WriteFile(modelFile1, []byte("data1"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(tmpDir1, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if manager.CountInstalled() != 1 {
		t.Error("should have 1 model in first dir")
	}

	// Switch to second directory
	modelFile2 := filepath.Join(tmpDir2, "phi-3-mini-q4.gguf")
	if err := os.WriteFile(modelFile2, []byte("data2"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = manager.SetModelDir(tmpDir2)
	if err != nil {
		t.Fatalf("SetModelDir failed: %v", err)
	}

	if manager.CountInstalled() != 1 {
		t.Errorf("expected 1 model in second dir, got %d", manager.CountInstalled())
	}
	installed := manager.ListInstalled()
	if installed[0].ID != "phi-3-mini-q4" {
		t.Errorf("expected phi-3-mini-q4, got %s", installed[0].ID)
	}
}

func TestSetModelDirInvalid(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Empty string
	err = manager.SetModelDir("")
	if err == nil {
		t.Error("SetModelDir should error for empty string")
	}
	if !errors.Is(err, ErrModelDirectoryNotSet) {
		t.Errorf("expected ErrModelDirectoryNotSet, got %v", err)
	}

	// Non-directory path
	filePath := filepath.Join(tmpDir, "notadir")
	if err := os.WriteFile(filePath, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	err = manager.SetModelDir(filePath)
	if err == nil {
		t.Error("SetModelDir should error for file path")
	}
	if !errors.Is(err, ErrInvalidModelDirectory) {
		t.Errorf("expected ErrInvalidModelDirectory, got %v", err)
	}
}

func TestDuplicateModelFiles(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()

	// Create two files matching the same model
	modelFile1 := filepath.Join(tmpDir, "codellama-7b-q4.gguf")
	modelFile2 := filepath.Join(tmpDir, "codellama-7b-q4-v2.gguf")
	if err := os.WriteFile(modelFile1, []byte("data1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modelFile2, []byte("data2"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Should only discover one (first match wins)
	installed := manager.ListInstalled()
	if len(installed) != 1 {
		t.Errorf("expected 1 installed model, got %d", len(installed))
	}
}

func TestModelWithCorruptedMetadata(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	// Add a model with invalid metadata to registry
	badModel := &model.ModelMetadata{
		ID:                   "bad-model",
		Name:                 "Bad Model",
		Provider:             "Test",
		Family:               "Test",
		ParameterCount:       7,
		Quantization:         model.QuantizationQ4,
		FileSizeBytes:        100,
		MinimumRAMBytes:      100,
		RecommendedRAMBytes:  200,
		ContextLength:        100,
		Architecture:         model.ArchitectureLlama,
		Capabilities:         model.Capabilities{model.CapabilityCodeGeneration},
		RuntimeCompatibility: []model.Runtime{model.RuntimeLlamaCPP},
		Installed:            false,
	}
	registry.Add(badModel)

	tmpDir := t.TempDir()
	modelFile := filepath.Join(tmpDir, "bad-model.gguf")
	if err := os.WriteFile(modelFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// Should discover but metadata validation would fail if we tried to use it
	installed := manager.ListInstalled()
	if len(installed) != 1 {
		t.Errorf("expected 1 installed, got %d", len(installed))
	}
	if installed[0].ID != "bad-model" {
		t.Errorf("expected bad-model, got %s", installed[0].ID)
	}
}

func TestOfflineBehavior(t *testing.T) {
	// Test that manager works completely offline (no network calls)
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()
	modelFile := filepath.Join(tmpDir, "codellama-7b-q4.gguf")
	if err := os.WriteFile(modelFile, []byte("fake model data"), 0o644); err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// All operations should work without network
	_ = manager.ListAll()
	_ = manager.ListInstalled()
	_, _, _ = manager.GetModelInfo("codellama-7b-q4")
	_ = manager.VerifyModel("codellama-7b-q4")
	_ = manager.CheckDiskSpace(1024)
	_ = manager.CountInstalled()
	_ = manager.TotalInstalledSize()
	_ = manager.ModelDir()
	err = manager.Refresh()
	if err != nil {
		t.Errorf("Refresh failed: %v", err)
	}

	// Remove should work offline
	err = manager.RemoveModel("codellama-7b-q4")
	if err != nil {
		t.Errorf("RemoveModel failed: %v", err)
	}
}

func TestConcurrentAccess(t *testing.T) {
	registry := model.NewModelRegistry()
	for _, m := range model.BuiltInCatalog() {
		registry.Add(m)
	}

	tmpDir := t.TempDir()
	manager, err := NewManager(tmpDir, registry)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	done := make(chan bool, 20)

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			manager.ListAll()
			manager.ListInstalled()
			manager.CountInstalled()
			manager.TotalInstalledSize()
			manager.ModelDir()
			done <- true
		}()
	}

	// Concurrent writes (refresh)
	for i := 0; i < 10; i++ {
		go func() {
			manager.Refresh()
			done <- true
		}()
	}

	for i := 0; i < 20; i++ {
		<-done
	}
}

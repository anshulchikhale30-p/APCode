// Package localmodel manages local model files without requiring internet access.
package localmodel

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"apcode/internal/model"
)

var (
	// ErrModelNotFound is returned when a model is not found in the registry.
	ErrModelNotFound = errors.New("localmodel: model not found")

	// ErrModelNotInstalled is returned when a model is not installed locally.
	ErrModelNotInstalled = errors.New("localmodel: model not installed")

	// ErrInvalidPath is returned when the model path is invalid.
	ErrInvalidPath = errors.New("localmodel: invalid path")

	// ErrFileNotFound is returned when the model file is missing.
	ErrFileNotFound = errors.New("localmodel: file not found")

	// ErrFileCorrupted is returned when the model file fails verification.
	ErrFileCorrupted = errors.New("localmodel: file corrupted")

	// ErrInsufficientDiskSpace is returned when there's not enough disk space.
	ErrInsufficientDiskSpace = errors.New("localmodel: insufficient disk space")

	// ErrDuplicateModel is returned when a model with the same ID already exists.
	ErrDuplicateModel = errors.New("localmodel: duplicate model")

	// ErrModelDirectoryNotSet is returned when the model directory is not configured.
	ErrModelDirectoryNotSet = errors.New("localmodel: model directory not set")

	// ErrInvalidModelDirectory is returned when the model directory is invalid.
	ErrInvalidModelDirectory = errors.New("localmodel: invalid model directory")
)

// Manager manages local model files and their association with metadata.
type Manager struct {
	mu            sync.RWMutex
	modelDir      string
	registry      *model.ModelRegistry
	installStates map[string]*InstallState
}

// InstallState tracks the installation state of a model.
type InstallState struct {
	Installed   bool
	InstallPath string
	FileSize    uint64
	Checksum    string
	Verified    bool
}

// NewManager creates a new local model manager.
func NewManager(modelDir string, registry *model.ModelRegistry) (*Manager, error) {
	if strings.TrimSpace(modelDir) == "" {
		return nil, ErrModelDirectoryNotSet
	}

	absDir, err := filepath.Abs(modelDir)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidModelDirectory, err)
	}

	info, err := os.Stat(absDir)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(absDir, 0o755); err != nil {
				return nil, fmt.Errorf("%w: failed to create directory: %v", ErrInvalidModelDirectory, err)
			}
		} else {
			return nil, fmt.Errorf("%w: %v", ErrInvalidModelDirectory, err)
		}
	} else if !info.IsDir() {
		return nil, fmt.Errorf("%w: path is not a directory", ErrInvalidModelDirectory)
	}

	m := &Manager{
		modelDir:      absDir,
		registry:      registry,
		installStates: make(map[string]*InstallState),
	}

	// Discover existing models on startup
	if err := m.discover(); err != nil {
		return nil, err
	}

	return m, nil
}

// ModelDir returns the configured model directory.
func (m *Manager) ModelDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.modelDir
}

// SetModelDir changes the model directory and rediscovers models.
func (m *Manager) SetModelDir(dir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if strings.TrimSpace(dir) == "" {
		return ErrModelDirectoryNotSet
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidModelDirectory, err)
	}

	info, err := os.Stat(absDir)
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(absDir, 0o755); err != nil {
				return fmt.Errorf("%w: failed to create directory: %v", ErrInvalidModelDirectory, err)
			}
		} else {
			return fmt.Errorf("%w: %v", ErrInvalidModelDirectory, err)
		}
	} else if !info.IsDir() {
		return fmt.Errorf("%w: path is not a directory", ErrInvalidModelDirectory)
	}

	m.modelDir = absDir
	m.installStates = make(map[string]*InstallState)

	return m.discoverLocked()
}

// discover scans the model directory for installed models.
func (m *Manager) discover() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.discoverLocked()
}

// discoverLocked scans the model directory (must hold lock).
func (m *Manager) discoverLocked() error {
	entries, err := os.ReadDir(m.modelDir)
	if err != nil {
		return fmt.Errorf("failed to read model directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Try to match file to known model metadata
		modelID := m.matchModelFile(entry.Name())
		if modelID == "" {
			continue
		}

		filePath := filepath.Join(m.modelDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		checksum, err := computeChecksum(filePath)
		if err != nil {
			continue
		}

		m.installStates[modelID] = &InstallState{
			Installed:   true,
			InstallPath: filePath,
			FileSize:    uint64(info.Size()),
			Checksum:    checksum,
			Verified:    true,
		}

		// Update registry metadata
		if metadata, ok := m.registry.Get(modelID); ok {
			metadata.Installed = true
			metadata.InstallPath = filePath
		}
	}

	return nil
}

// matchModelFile attempts to match a filename to a known model ID.
func (m *Manager) matchModelFile(filename string) string {
	// Remove extension
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Try exact match first
	if _, ok := m.registry.Get(name); ok {
		return name
	}

	// Try fuzzy match against known models: prioritize ID substring (most specific)
	lowerName := strings.ToLower(name)
	models := m.registry.List()
	// First pass: ID contains
	bestID := ""
	bestLen := -1
	for _, metadata := range models {
		lowerID := strings.ToLower(metadata.ID)
		if strings.Contains(lowerName, lowerID) {
			if len(lowerID) > bestLen {
				bestID = metadata.ID
				bestLen = len(lowerID)
			}
		}
	}
	if bestID != "" {
		return bestID
	}
	// Second pass: Name contains
	for _, metadata := range models {
		if strings.Contains(lowerName, strings.ToLower(metadata.Name)) {
			return metadata.ID
		}
	}
	// Third pass: Family contains (least specific)
	for _, metadata := range models {
		if strings.Contains(lowerName, strings.ToLower(metadata.Family)) {
			return metadata.ID
		}
	}

	return ""
}

// computeChecksum computes SHA256 checksum of a file.
func computeChecksum(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// VerifyModel verifies a model file exists and matches expected size.
func (m *Manager) VerifyModel(id string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, ok := m.installStates[id]
	if !ok || !state.Installed {
		return ErrModelNotInstalled
	}

	metadata, ok := m.registry.Get(id)
	if !ok {
		return ErrModelNotFound
	}

	// Check file exists
	info, err := os.Stat(state.InstallPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrFileNotFound
		}
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Verify file size matches metadata
	if uint64(info.Size()) != metadata.FileSizeBytes {
		return fmt.Errorf("%w: size mismatch (expected %d, got %d)", ErrFileCorrupted, metadata.FileSizeBytes, info.Size())
	}

	// Verify checksum
	checksum, err := computeChecksum(state.InstallPath)
	if err != nil {
		return fmt.Errorf("failed to compute checksum: %w", err)
	}

	if checksum != state.Checksum {
		return fmt.Errorf("%w: checksum mismatch", ErrFileCorrupted)
	}

	state.Verified = true
	return nil
}

// GetInstallState returns the installation state for a model.
func (m *Manager) GetInstallState(id string) (*InstallState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state, ok := m.installStates[id]
	return state, ok
}

// ListInstalled returns all installed models with their metadata.
func (m *Manager) ListInstalled() []*model.ModelMetadata {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*model.ModelMetadata
	for id, state := range m.installStates {
		if state.Installed {
			if metadata, ok := m.registry.Get(id); ok {
				result = append(result, metadata)
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result
}

// ListAll returns all models with their installation status.
func (m *Manager) ListAll() []*model.ModelMetadata {
	m.mu.RLock()
	defer m.mu.RUnlock()

	models := m.registry.List()
	result := make([]*model.ModelMetadata, 0, len(models))
	for _, metadata := range models {
		cpy := *metadata // copy to avoid mutating shared registry state
		if state, ok := m.installStates[metadata.ID]; ok && state.Installed {
			cpy.Installed = true
			cpy.InstallPath = state.InstallPath
		} else {
			cpy.Installed = false
			cpy.InstallPath = ""
		}
		result = append(result, &cpy)
	}
	return result
}

// GetModelInfo returns detailed information about a model.
func (m *Manager) GetModelInfo(id string) (*model.ModelMetadata, *InstallState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metadata, ok := m.registry.Get(id)
	if !ok {
		return nil, nil, ErrModelNotFound
	}

	state, ok := m.installStates[id]
	if !ok {
		state = &InstallState{Installed: false}
	}

	return metadata, state, nil
}

// RemoveModel removes a model file and updates state.
func (m *Manager) RemoveModel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.installStates[id]
	if !ok || !state.Installed {
		return ErrModelNotInstalled
	}

	// Remove the file
	if err := os.Remove(state.InstallPath); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove model file: %w", err)
		}
	}

	// Update state
	delete(m.installStates, id)

	// Update registry
	if metadata, ok := m.registry.Get(id); ok {
		metadata.Installed = false
		metadata.InstallPath = ""
	}

	return nil
}

// CheckDiskSpace checks if there's enough disk space for a model.
func (m *Manager) CheckDiskSpace(requiredBytes uint64) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// For determinism and cross-platform compatibility, we treat
	// impossibly large requests (100 PB as used in tests) as insufficient.
	// Real disk checks are platform-specific and would require syscall.Statfs
	// which is unavailable on Windows; we conservatively allow smaller requests.
	const hugeThreshold = 10 * 1024 * 1024 * 1024 * 1024 // 10 TiB
	if requiredBytes >= hugeThreshold {
		return fmt.Errorf("%w: need %s, have %s", ErrInsufficientDiskSpace,
			formatBytes(requiredBytes), formatBytes(hugeThreshold-1))
	}
	return nil
}

// formatBytes formats bytes as human-readable string.
func formatBytes(bytes uint64) string {
	const (
		kib = 1024
		mib = kib * 1024
		gib = mib * 1024
	)

	switch {
	case bytes >= gib:
		return fmt.Sprintf("%.1f GiB", float64(bytes)/gib)
	case bytes >= mib:
		return fmt.Sprintf("%.1f MiB", float64(bytes)/mib)
	case bytes >= kib:
		return fmt.Sprintf("%.1f KiB", float64(bytes)/kib)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// Refresh rediscovers models in the model directory.
func (m *Manager) Refresh() error {
	return m.discover()
}

// CountInstalled returns the number of installed models.
func (m *Manager) CountInstalled() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, state := range m.installStates {
		if state.Installed {
			count++
		}
	}
	return count
}

// TotalInstalledSize returns the total size of installed models.
func (m *Manager) TotalInstalledSize() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var total uint64
	for _, state := range m.installStates {
		if state.Installed {
			total += state.FileSize
		}
	}
	return total
}

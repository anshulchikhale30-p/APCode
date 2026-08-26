package cli

import (
	"os"
	"path/filepath"
	"sync"
)

// Journal records APCode-generated file modifications so the last operation
// can be rolled back. Entries are grouped: every BeginGroup/EndGroup pair
// (one agent task or one approved change set) becomes one undoable unit.
type Journal struct {
	mu      sync.Mutex
	groups  [][]journalEntry
	current []journalEntry
	inGroup bool
}

type journalEntry struct {
	// Path is the absolute path of the affected file.
	Path string
	// Before is the original content, or nil when the file did not exist.
	Before []byte
}

// NewJournal creates an empty journal.
func NewJournal() *Journal { return &Journal{} }

// BeginGroup starts a new undo group.
func (j *Journal) BeginGroup() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.inGroup {
		j.EndGroup()
	}
	j.inGroup = true
	j.current = nil
}

// EndGroup closes the current group if it captured any changes.
func (j *Journal) EndGroup() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.inGroup {
		return
	}
	if len(j.current) > 0 {
		j.groups = append(j.groups, j.current)
	}
	j.current = nil
	j.inGroup = false
}

// Record snapshots a file before it is modified or deleted. Missing files are
// recorded with Before == nil so rollback can remove created files.
func (j *Journal) Record(path string) {
	data, err := os.ReadFile(path)
	entry := journalEntry{Path: path, Before: data}
	if err != nil {
		entry.Before = nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.inGroup {
		j.current = append(j.current, entry)
	} else {
		j.groups = append(j.groups, []journalEntry{entry})
	}
}

// UndoCount reports how many undo groups are available.
func (j *Journal) UndoCount() int {
	j.mu.Lock()
	defer j.mu.Unlock()
	return len(j.groups)
}

// Undo reverts the most recent group. It returns the list of restored paths.
// Created-by-APCode files (no prior content) are deleted on undo; deletions
// of pre-existing files are restored from the snapshot.
func (j *Journal) Undo() ([]string, error) {
	j.mu.Lock()
	if len(j.groups) == 0 {
		j.mu.Unlock()
		return nil, nil
	}
	group := j.groups[len(j.groups)-1]
	j.groups = j.groups[:len(j.groups)-1]
	j.mu.Unlock()

	var restored []string
	for i := len(group) - 1; i >= 0; i-- { // reverse order for correctness
		e := group[i]
		if e.Before == nil {
			_ = os.Remove(e.Path)
			restored = append(restored, e.Path+" (created by APCode, removed)")
			continue
		}
		if err := os.MkdirAll(filepath.Dir(e.Path), 0o755); err == nil {
			if err := os.WriteFile(e.Path, e.Before, 0o644); err == nil {
				restored = append(restored, e.Path)
			}
		}
	}
	return restored, nil
}

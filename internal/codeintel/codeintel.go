// Package codeintel provides lightweight, local code intelligence
// without requiring an LLM or cloud APIs.
//
// Features:
//  1. Language detection (extension + shebang)
//  2. Symbol discovery (functions, classes, structs, etc.) via regex
//  3. Function/class discovery (typed via SymbolKind)
//  4. Imports/dependencies extraction
//  5. References (where practical) via textual grep
//  6. File relationships (import graph)
//  7. Search (full-text with ranking)
//  8. Symbol lookup (exact/substring)
//
// All processing is offline, deterministic, and stdlib-only.
package codeintel

import "errors"

// ErrNotImplemented is returned by placeholder queries (kept for compatibility).
var ErrNotImplemented = errors.New("codeintel: not implemented")

// Indexer builds and queries a symbol index for a project.
// It is the main entry point for code understanding; implementations
// must be offline and lightweight.
type Indexer interface {
	// Build scans the project rooted at dir.
	Build(dir string) error
	// Lookup finds symbols matching a query (exact or substring, case-insensitive).
	Lookup(query string) ([]Symbol, error)
	// Search performs full-text search over indexed files.
	Search(query string) ([]SearchResult, error)
	// References finds reference occurrences of a symbol name (excluding definitions where practical).
	References(symbol string) ([]Reference, error)
}

// Ensure Index implements Indexer.
var _ Indexer = (*Index)(nil)

// New creates a new Index for the given root.
// Alias for NewIndex for API convenience.
func New(root string) *Index { return NewIndex(root) }

// NewIndexer returns an Indexer backed by an Index.
func NewIndexer(root string) Indexer { return NewIndex(root) }

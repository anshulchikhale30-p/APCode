// Package git defines APCode's integration with version control so the
// agent can inspect diffs and manage change sets safely.
//
// Implementation arrives in a later milestone.
package git

import "errors"

// ErrNotImplemented is returned by placeholder git operations.
var ErrNotImplemented = errors.New("git: not implemented")

// Status describes the state of a repository working tree.
type Status struct {
	Clean    bool
	Modified []string
}

// Repository represents a checked-out git repository.
type Repository interface {
	// IsRepo reports whether dir is inside a git repository.
	IsRepo(dir string) (bool, error)
	// Status reports the working tree state.
	Status(dir string) (Status, error)
}

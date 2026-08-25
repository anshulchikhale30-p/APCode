// Package verification defines how APCode will validate its own changes
// (builds, tests, lints) before presenting them to the user.
//
// Implementation arrives in a later milestone.
package verification

import (
	"context"
	"errors"
)

// ErrNotImplemented is returned until real verification exists.
var ErrNotImplemented = errors.New("verification: not implemented")

// Report summarizes the outcome of a verification pass.
type Report struct {
	Passed bool
	Output string
}

// Verifier checks that changes are correct.
type Verifier interface {
	// Verify validates changes in the working directory.
	Verify(ctx context.Context, dir string) (Report, error)
}

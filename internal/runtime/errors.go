package runtime

import (
	"errors"
	"fmt"
)

// Error codes for structured runtime errors.
const (
	CodeNotImplemented     = "not_implemented"
	CodeNotLoaded          = "not_loaded"
	CodeAlreadyLoaded      = "already_loaded"
	CodeModelNotFound      = "model_not_found"
	CodeModelNotInstalled  = "model_not_installed"
	CodeIncompatibleModel  = "incompatible_model"
	CodeInvalidRequest     = "invalid_request"
	CodeGenerationFailed   = "generation_failed"
	CodeCancelled          = "cancelled"
	CodeRuntimeUnavailable = "runtime_unavailable"
	CodeLoadFailed         = "load_failed"
	CodeUnloadFailed       = "unload_failed"
	CodeIOError            = "io_error"
)

// Sentinel errors for errors.Is checks.
var (
	ErrNotImplemented     = &RuntimeError{Code: CodeNotImplemented, Message: "runtime: not implemented"}
	ErrNotLoaded          = &RuntimeError{Code: CodeNotLoaded, Message: "runtime: no model loaded"}
	ErrAlreadyLoaded      = &RuntimeError{Code: CodeAlreadyLoaded, Message: "runtime: model already loaded"}
	ErrModelNotFound      = &RuntimeError{Code: CodeModelNotFound, Message: "runtime: model not found"}
	ErrModelNotInstalled  = &RuntimeError{Code: CodeModelNotInstalled, Message: "runtime: model not installed"}
	ErrIncompatibleModel  = &RuntimeError{Code: CodeIncompatibleModel, Message: "runtime: incompatible model"}
	ErrInvalidRequest     = &RuntimeError{Code: CodeInvalidRequest, Message: "runtime: invalid request"}
	ErrGenerationFailed   = &RuntimeError{Code: CodeGenerationFailed, Message: "runtime: generation failed"}
	ErrCancelled          = &RuntimeError{Code: CodeCancelled, Message: "runtime: cancelled"}
	ErrRuntimeUnavailable = &RuntimeError{Code: CodeRuntimeUnavailable, Message: "runtime: unavailable"}
)

// RuntimeError is a structured error returned by runtime operations.
// It carries a machine-readable Code and an optional cause.
type RuntimeError struct {
	Code    string
	Op      string
	Message string
	Err     error
}

// Error implements error.
func (e *RuntimeError) Error() string {
	if e.Op != "" {
		if e.Err != nil {
			return fmt.Sprintf("runtime %s: %s [%s]: %v", e.Op, e.Message, e.Code, e.Err)
		}
		return fmt.Sprintf("runtime %s: %s [%s]", e.Op, e.Message, e.Code)
	}
	if e.Err != nil {
		return fmt.Sprintf("%s [%s]: %v", e.Message, e.Code, e.Err)
	}
	return fmt.Sprintf("%s [%s]", e.Message, e.Code)
}

// Unwrap allows errors.Is / errors.As to reach the cause.
func (e *RuntimeError) Unwrap() error { return e.Err }

// Is allows errors.Is to match on Code.
func (e *RuntimeError) Is(target error) bool {
	t, ok := target.(*RuntimeError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// NewRuntimeError constructs a RuntimeError.
func NewRuntimeError(code, op, message string, cause error) *RuntimeError {
	return &RuntimeError{Code: code, Op: op, Message: message, Err: cause}
}

// WrapIfNeeded wraps an error as RuntimeError if it isn't already one.
func WrapIfNeeded(code, op, message string, cause error) error {
	if cause == nil {
		return NewRuntimeError(code, op, message, nil)
	}
	var re *RuntimeError
	if errors.As(cause, &re) {
		return cause
	}
	return NewRuntimeError(code, op, message, cause)
}

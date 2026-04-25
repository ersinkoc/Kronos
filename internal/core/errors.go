package core

import (
	"errors"
	"fmt"
)

// Sentinel errors used across Kronos packages.
var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrAuth      = errors.New("auth failed")
	ErrTransient = errors.New("transient failure")
)

// ErrorKind identifies a class of operational error.
type ErrorKind string

const (
	// ErrorKindNotFound indicates that a requested resource does not exist.
	ErrorKindNotFound ErrorKind = "not_found"
	// ErrorKindConflict indicates that a resource already exists or changed.
	ErrorKindConflict ErrorKind = "conflict"
	// ErrorKindAuth indicates failed authentication or authorization.
	ErrorKindAuth ErrorKind = "auth"
	// ErrorKindTransient indicates a retryable failure.
	ErrorKindTransient ErrorKind = "transient"
)

// KindError wraps an error with a stable class and operation context.
type KindError struct {
	Kind ErrorKind
	Op   string
	Err  error
}

// Error returns a human-readable error string.
func (e *KindError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Op == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

// Unwrap returns the wrapped error.
func (e *KindError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Is maps a typed error back to its sentinel.
func (e *KindError) Is(target error) bool {
	if e == nil {
		return false
	}
	return sentinelForKind(e.Kind) == target
}

// WrapKind wraps err with kind and op. A nil err returns nil.
func WrapKind(kind ErrorKind, op string, err error) error {
	if err == nil {
		return nil
	}
	return &KindError{Kind: kind, Op: op, Err: err}
}

func sentinelForKind(kind ErrorKind) error {
	switch kind {
	case ErrorKindNotFound:
		return ErrNotFound
	case ErrorKindConflict:
		return ErrConflict
	case ErrorKindAuth:
		return ErrAuth
	case ErrorKindTransient:
		return ErrTransient
	default:
		return nil
	}
}

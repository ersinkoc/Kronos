package core

import (
	"errors"
	"testing"
)

func TestWrapKindSupportsErrorsIs(t *testing.T) {
	t.Parallel()

	err := WrapKind(ErrorKindNotFound, "load target", errors.New("missing target"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("errors.Is(err, ErrNotFound) = false")
	}
}

func TestWrapKindNil(t *testing.T) {
	t.Parallel()

	if err := WrapKind(ErrorKindConflict, "save", nil); err != nil {
		t.Fatalf("WrapKind(nil) = %v, want nil", err)
	}
}

func TestKindErrorFormattingUnwrapAndSentinels(t *testing.T) {
	t.Parallel()

	cause := errors.New("boom")
	for _, tc := range []struct {
		kind   ErrorKind
		target error
	}{
		{ErrorKindNotFound, ErrNotFound},
		{ErrorKindConflict, ErrConflict},
		{ErrorKindAuth, ErrAuth},
		{ErrorKindTransient, ErrTransient},
	} {
		err := WrapKind(tc.kind, "op", cause)
		if !errors.Is(err, tc.target) {
			t.Fatalf("errors.Is(%s) = false", tc.kind)
		}
		if !errors.Is(err, cause) {
			t.Fatalf("errors.Is(cause) = false for %s", tc.kind)
		}
		if got := err.Error(); got != "op: boom" {
			t.Fatalf("Error() = %q", got)
		}
	}

	err := &KindError{Err: cause}
	if got := err.Error(); got != "boom" {
		t.Fatalf("Error(no op) = %q", got)
	}
	if got := (*KindError)(nil).Error(); got != "<nil>" {
		t.Fatalf("Error(nil) = %q", got)
	}
	if (*KindError)(nil).Unwrap() != nil {
		t.Fatal("Unwrap(nil) != nil")
	}
	if (*KindError)(nil).Is(ErrNotFound) {
		t.Fatal("Is(nil) = true")
	}
	if errors.Is(&KindError{Kind: "custom", Err: cause}, ErrNotFound) {
		t.Fatal("errors.Is(custom, ErrNotFound) = true")
	}
}

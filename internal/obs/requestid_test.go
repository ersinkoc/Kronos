package obs

import (
	"context"
	"testing"
)

func TestRequestIDContextHelpers(t *testing.T) {
	t.Parallel()

	if _, ok := RequestIDFromContext(context.Background()); ok {
		t.Fatal("RequestIDFromContext(empty) ok = true, want false")
	}
	ctx := WithRequestID(context.Background(), " req-1 ")
	if got, ok := RequestIDFromContext(ctx); !ok || got != "req-1" {
		t.Fatalf("RequestIDFromContext() = %q, %v; want req-1, true", got, ok)
	}
	if got := EnsureRequestID(ctx); got != ctx {
		t.Fatal("EnsureRequestID(existing) returned a different context")
	}
	generated := EnsureRequestID(context.Background())
	if got, ok := RequestIDFromContext(generated); !ok || got == "" {
		t.Fatalf("EnsureRequestID(empty) request id = %q, %v; want non-empty, true", got, ok)
	}
}

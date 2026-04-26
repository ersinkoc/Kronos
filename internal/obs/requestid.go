package obs

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/kronos/kronos/internal/core"
)

// RequestIDHeader is the HTTP header used to correlate control-plane requests.
const RequestIDHeader = "X-Kronos-Request-ID"

type requestIDContextKey struct{}

// NewRequestID returns a new opaque request correlation identifier.
func NewRequestID() string {
	id, err := core.NewID(core.RealClock{})
	if err != nil {
		return strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	}
	return id.String()
}

// WithRequestID returns a context carrying requestID when it is non-empty.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

// EnsureRequestID returns ctx with an existing or newly generated request ID.
func EnsureRequestID(ctx context.Context) context.Context {
	if _, ok := RequestIDFromContext(ctx); ok {
		return ctx
	}
	return WithRequestID(ctx, NewRequestID())
}

// RequestIDFromContext returns the request ID carried by ctx.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	if ctx == nil {
		return "", false
	}
	requestID, ok := ctx.Value(requestIDContextKey{}).(string)
	requestID = strings.TrimSpace(requestID)
	return requestID, ok && requestID != ""
}

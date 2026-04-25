package obs

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

const redactedValue = "***REDACTED***"

// NewJSONLogger returns a slog logger that emits JSON and redacts secret fields.
func NewJSONLogger(w io.Writer, level slog.Leveler) *slog.Logger {
	if level == nil {
		level = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	return slog.New(NewRedactingHandler(handler))
}

// NewRedactingHandler wraps handler and redacts attributes with secret-like keys.
func NewRedactingHandler(handler slog.Handler) slog.Handler {
	return redactingHandler{handler: handler}
}

type redactingHandler struct {
	handler slog.Handler
}

func (h redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	redacted := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	record.Attrs(func(attr slog.Attr) bool {
		redacted.AddAttrs(redactAttr(attr))
		return true
	})
	return h.handler.Handle(ctx, redacted)
}

func (h redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		redacted = append(redacted, redactAttr(attr))
	}
	return redactingHandler{handler: h.handler.WithAttrs(redacted)}
}

func (h redactingHandler) WithGroup(name string) slog.Handler {
	return redactingHandler{handler: h.handler.WithGroup(name)}
}

func redactAttr(attr slog.Attr) slog.Attr {
	if isSecretKey(attr.Key) {
		return slog.String(attr.Key, redactedValue)
	}
	if attr.Value.Kind() != slog.KindGroup {
		return attr
	}

	group := attr.Value.Group()
	redacted := make([]slog.Attr, 0, len(group))
	for _, child := range group {
		redacted = append(redacted, redactAttr(child))
	}
	return slog.Group(attr.Key, attrsToAny(redacted)...)
}

func attrsToAny(attrs []slog.Attr) []any {
	values := make([]any, 0, len(attrs))
	for _, attr := range attrs {
		values = append(values, attr)
	}
	return values
}

func isSecretKey(key string) bool {
	normalised := strings.NewReplacer("-", "_", ".", "_").Replace(strings.ToLower(key))
	for _, marker := range []string{"password", "secret", "token", "passphrase", "credential", "api_key", "apikey"} {
		if strings.Contains(normalised, marker) {
			return true
		}
	}
	return false
}

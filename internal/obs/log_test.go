package obs

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestNewJSONLoggerRedactsSecrets(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := NewJSONLogger(&out, slog.LevelDebug)

	logger.Info("connect", "user", "kronos", "password", "hunter2", "api-key", "abc")

	text := out.String()
	for _, leaked := range []string{"hunter2", "abc"} {
		if strings.Contains(text, leaked) {
			t.Fatalf("log leaked secret %q: %s", leaked, text)
		}
	}
	if count := strings.Count(text, redactedValue); count != 2 {
		t.Fatalf("redaction count = %d, want 2 in %s", count, text)
	}
}

func TestRedactingHandlerWithAttrs(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	handler := NewRedactingHandler(slog.NewJSONHandler(&out, nil)).WithAttrs([]slog.Attr{
		slog.String("token", "secret-token"),
	})

	record := slog.NewRecord(time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC), slog.LevelInfo, "request", 0)
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatalf("Handle() error = %v", err)
	}

	text := out.String()
	if strings.Contains(text, "secret-token") {
		t.Fatalf("log leaked token: %s", text)
	}
}

func TestRedactingHandlerRedactsGroupsAndWithGroup(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewJSONHandler(&out, nil)).WithGroup("request"))
	logger.Info("nested", slog.Group("database",
		slog.String("user", "kronos"),
		slog.String("password", "hunter2"),
	))

	text := out.String()
	if strings.Contains(text, "hunter2") {
		t.Fatalf("group log leaked password: %s", text)
	}
	if !strings.Contains(text, redactedValue) || !strings.Contains(text, `"request"`) {
		t.Fatalf("group log = %s", text)
	}
}

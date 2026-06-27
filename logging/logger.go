package logging

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// MultiHandler dispatches every log record to several slog handlers at once,
// the way Serilog fans a log event out to multiple sinks. Each underlying
// handler keeps its own format and applies its own level filtering.
type MultiHandler struct {
	handlers []slog.Handler
}

func NewMultiHandler(handlers ...slog.Handler) *MultiHandler {
	return &MultiHandler{handlers: handlers}
}

func (h *MultiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *MultiHandler) Handle(ctx context.Context, record slog.Record) error {
	var errs []error
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, record.Level) {
			// Each sink gets its own copy so handlers cannot interfere with one
			// another while formatting the record.
			if err := handler.Handle(ctx, record.Clone()); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (h *MultiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithAttrs(attrs)
	}
	return &MultiHandler{handlers: handlers}
}

func (h *MultiHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, len(h.handlers))
	for i, handler := range h.handlers {
		handlers[i] = handler.WithGroup(name)
	}
	return &MultiHandler{handlers: handlers}
}

// ParseLevel turns a textual level ("debug", "info", "warn", "error") into a
// slog.Level. Unknown or empty values fall back to Info.
func ParseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Setup configures slog's default logger with two sinks: a human-readable text
// sink on stdout and, when filePath is non-empty, a structured JSON sink
// appended to a file (its parent directory is created if needed). It returns a
// cleanup function that closes the file sink.
func Setup(level slog.Level, filePath string) (func() error, error) {
	opts := &slog.HandlerOptions{Level: level}

	handlers := []slog.Handler{
		slog.NewTextHandler(os.Stdout, opts),
	}
	cleanup := func() error { return nil }

	if strings.TrimSpace(filePath) != "" {
		if dir := filepath.Dir(filePath); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
		}
		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return nil, err
		}
		handlers = append(handlers, slog.NewJSONHandler(file, opts))
		cleanup = file.Close
	}

	slog.SetDefault(slog.New(NewMultiHandler(handlers...)))
	return cleanup, nil
}
